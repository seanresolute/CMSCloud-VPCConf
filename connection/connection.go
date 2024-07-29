package connection

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"strings"
	"time"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/awscreds"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/iam"
)

type NetworkConnectionTest struct {
	Credentials awscreds.AWSCreds
	Logger      Logger
	Context     context.Context
	rollbacks   []rollbackFunction
}

// You must specify AccountID, Region, and one of:
// - NetworkInterfaceID
// - SubnetID & SecurityGroupIDs
// - SubnetID & CreateEgressSecurityGroup=true
type NetworkInterfaceSpec struct {
	AccountID   string
	Region      string
	UsePublicIP bool

	NetworkInterfaceID *string

	VPCID    string
	SubnetID string
	// If true, then any test performed will use a new security group
	// with a rule allowing egress to all IPs/ports instead of any
	// existing security group.
	CreateEgressSecurityGroup bool
	SecurityGroupIDs          []string
	egressSecurityGroupID     string

	session *session.Session
}

type Logger interface {
	Log(msg string, args ...interface{})
}

type DefaultLogger struct{}

func (*DefaultLogger) Log(msg string, args ...interface{}) {
	log.Printf(msg, args...)
}

type rollbackFunction func() (string, error)

func (t *NetworkConnectionTest) Log(msg string, args ...interface{}) {
	if t.Logger != nil {
		t.Logger.Log(msg, args...)
	}
}

func (t *NetworkConnectionTest) rollback() error {
	if len(t.rollbacks) > 0 {
		t.Log("Deprovisioning everything")
	}
	failed := []string{}
	for idx := len(t.rollbacks) - 1; idx >= 0; idx-- {
		rollback := t.rollbacks[idx]
		if resource, err := rollback(); err != nil {
			t.Log("Error deprovisioning %q: %s", resource, err)
			if resource != "" {
				failed = append(failed, resource)
			}
		}
	}
	t.rollbacks = nil
	if len(failed) > 0 {
		return &RollbackError{
			ResourcesNotRolledBack: failed,
		}
	}
	return nil
}

type IPType int

const (
	IPTypeInternet IPType = iota
	IPTypeSharedService
	IPTypeVPC
)

type IPAddress struct {
	IPType IPType
	IP     net.IP
}

// Either NetworkInterfaceSpec or IPAddress must be specified
type Endpoint struct {
	IPAddress            *IPAddress
	NetworkInterfaceSpec *NetworkInterfaceSpec
}

type ConnectionSpec struct {
	Source      *Endpoint
	Destination *Endpoint
	Port        int64
	// If true, Source.NetworkInterfaceSpec must be specified
	PerformTest bool
}

// TODO: make these names unique for each run
const (
	ecsClusterName     = "debug-test-network-connection"
	serverTaskName     = "debug-test-network-connection-server"
	clientTaskName     = "debug-test-network-connection-client"
	ecsTaskRoleName    = "debug-test-network-connection-util"
	dynamodbTableName  = ecsClusterName
	dynamodbPrimaryKey = "task-arn"
	lambdaFunctionName = "debug-test-network-connection"
)

var egressToAllDestinations = []*ec2.IpPermission{
	{
		FromPort:   aws.Int64(0),
		ToPort:     aws.Int64(65535),
		IpProtocol: aws.String("tcp"),
		IpRanges: []*ec2.IpRange{
			{
				CidrIp: aws.String("0.0.0.0/0"),
			},
		},
	},
}

func (t *NetworkConnectionTest) fillInDetails(endpoint *Endpoint) error {
	if (endpoint.NetworkInterfaceSpec == nil) == (endpoint.IPAddress == nil) {
		return fmt.Errorf("You must specify exactly one of NetworkInterfaceSpec or IPAddress")
	}
	if endpoint.NetworkInterfaceSpec != nil {
		ec2svc := newCancellableEC2(t, endpoint.NetworkInterfaceSpec.session)
		endpoint.IPAddress = &IPAddress{}
		if endpoint.NetworkInterfaceSpec.NetworkInterfaceID != nil {
			if endpoint.NetworkInterfaceSpec.VPCID != "" || endpoint.NetworkInterfaceSpec.SubnetID != "" || endpoint.NetworkInterfaceSpec.SecurityGroupIDs != nil {
				return fmt.Errorf("Must not specify VPC ID, Subnet, or Security Groups if Network Interface is specified")
			}
			out, err := ec2svc.DescribeNetworkInterfaces(&ec2.DescribeNetworkInterfacesInput{
				NetworkInterfaceIds: []*string{endpoint.NetworkInterfaceSpec.NetworkInterfaceID},
			})
			if err != nil {
				return fmt.Errorf("Error describing network interfaces: %w", err)
			}
			if len(out.NetworkInterfaces) != 1 {
				return fmt.Errorf("Got %d network interfaces with id %q", len(out.NetworkInterfaces), *endpoint.NetworkInterfaceSpec.NetworkInterfaceID)
			}
			iface := out.NetworkInterfaces[0]
			for _, group := range iface.Groups {
				endpoint.NetworkInterfaceSpec.SecurityGroupIDs = append(endpoint.NetworkInterfaceSpec.SecurityGroupIDs, aws.StringValue(group.GroupId))
			}
			if endpoint.NetworkInterfaceSpec.UsePublicIP {
				if iface.Association == nil || iface.Association.PublicIp == nil {
					return fmt.Errorf("%s has no public IP", *endpoint.NetworkInterfaceSpec.NetworkInterfaceID)
				}
				ip := aws.StringValue(iface.Association.PublicIp)
				endpoint.IPAddress.IP = net.ParseIP(ip)
				endpoint.IPAddress.IPType = IPTypeInternet
			} else {
				ip := aws.StringValue(iface.PrivateIpAddress)
				endpoint.IPAddress.IP = net.ParseIP(ip)
				endpoint.IPAddress.IPType = IPTypeVPC
			}
			if endpoint.IPAddress.IP == nil {
				return fmt.Errorf("Failed to locate IP of network interface %q", *endpoint.NetworkInterfaceSpec.NetworkInterfaceID)
			}
			endpoint.NetworkInterfaceSpec.VPCID = aws.StringValue(iface.VpcId)
			endpoint.NetworkInterfaceSpec.SubnetID = aws.StringValue(iface.SubnetId)
			t.Log("Network Interface %q is in VPC %s / Subnet %s and has IP %s", *endpoint.NetworkInterfaceSpec.NetworkInterfaceID, endpoint.NetworkInterfaceSpec.VPCID, endpoint.NetworkInterfaceSpec.SubnetID, endpoint.IPAddress.IP)
		} else {
			if endpoint.NetworkInterfaceSpec.SubnetID == "" {
				return fmt.Errorf("Must specify Subnet ID if Network Interface is not specified")
			}
			if endpoint.NetworkInterfaceSpec.SecurityGroupIDs == nil != endpoint.NetworkInterfaceSpec.CreateEgressSecurityGroup {
				return fmt.Errorf("Must either specify security group IDs or ask for an egress security group to be created, not both or neither")
			}
			out, err := ec2svc.DescribeSubnets(&ec2.DescribeSubnetsInput{
				SubnetIds: []*string{&endpoint.NetworkInterfaceSpec.SubnetID},
			})
			if err != nil {
				return fmt.Errorf("Error loading subnet info: %w", err)
			}
			if len(out.Subnets) != 1 {
				return fmt.Errorf("Got %d subnets for ID %q", len(out.Subnets), endpoint.NetworkInterfaceSpec.SubnetID)
			}
			endpoint.NetworkInterfaceSpec.VPCID = aws.StringValue(out.Subnets[0].VpcId)
			endpoint.IPAddress.IPType = IPTypeVPC
			endpoint.IPAddress.IP, _, err = net.ParseCIDR(aws.StringValue(out.Subnets[0].CidrBlock))
			if err != nil {
				return fmt.Errorf("Error parsing subnet CIDR: %w", err)
			}
		}
		return nil
	} else {
		return nil
	}
}

type RollbackError struct {
	ResourcesNotRolledBack []string
}

func (e *RollbackError) Error() string {
	return fmt.Sprintf("Resources not rolled back: %s", strings.Join(e.ResourcesNotRolledBack, ","))
}

// If rollBackErr is non-nil, it will be a *RollbackError
func (t *NetworkConnectionTest) Verify(s *ConnectionSpec) (verifyErr error, rollbackErr error) {
	defer func() {
		if x := recover(); x != nil {
			verifyErr = fmt.Errorf("run-time panic: %v", x)
		}
		if verifyErr != nil {
			t.Log("Error encountered: %s", verifyErr)
		}
		rollbackErr = t.rollback()
	}()
	verifyErr = t.verify(s)
	return
}

func (t *NetworkConnectionTest) verify(s *ConnectionSpec) error {
	// Deep copy by serializing to and from JSON
	buf, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("Error copying spec: %w", err)
	}
	spec := &ConnectionSpec{}
	err = json.Unmarshal(buf, spec)
	if err != nil {
		return fmt.Errorf("Error copying spec: %w", err)
	}
	if spec.Source.NetworkInterfaceSpec != nil {
		t.Log("Getting credentials for account %s", spec.Source.NetworkInterfaceSpec.AccountID)
		sourceCreds, err := t.Credentials.GetCredentialsForAccount(spec.Source.NetworkInterfaceSpec.AccountID)
		if err != nil {
			return fmt.Errorf("Failed to get credentials for account %q: %w", spec.Source.NetworkInterfaceSpec.AccountID, err)
		}
		spec.Source.NetworkInterfaceSpec.session = session.Must(session.NewSession(&aws.Config{
			Region:      aws.String(spec.Source.NetworkInterfaceSpec.Region),
			Credentials: sourceCreds,
		}))
	}
	if spec.Destination.NetworkInterfaceSpec != nil {
		if spec.Source.NetworkInterfaceSpec != nil && spec.Source.NetworkInterfaceSpec.AccountID == spec.Destination.NetworkInterfaceSpec.AccountID {
			spec.Destination.NetworkInterfaceSpec.session = spec.Source.NetworkInterfaceSpec.session
		} else {
			t.Log("Getting credentials for account %s", spec.Destination.NetworkInterfaceSpec.AccountID)
			destinationCreds, err := t.Credentials.GetCredentialsForAccount(spec.Destination.NetworkInterfaceSpec.AccountID)
			if err != nil {
				return fmt.Errorf("Failed to get credentials for account %q: %w", spec.Destination.NetworkInterfaceSpec.AccountID, err)
			}
			spec.Destination.NetworkInterfaceSpec.session = session.Must(session.NewSession(&aws.Config{
				Region:      aws.String(spec.Destination.NetworkInterfaceSpec.Region),
				Credentials: destinationCreds,
			}))
		}
	}

	err = t.fillInDetails(spec.Source)
	if err != nil {
		return err
	}
	err = t.fillInDetails(spec.Destination)
	if err != nil {
		return err
	}

	err = t.updateSourceIPForInternetEgress(spec.Source, spec.Destination)
	if err != nil {
		return fmt.Errorf("Error determining public IP for source: %w", err)
	}

	if spec.Source.NetworkInterfaceSpec != nil {
		err := t.checkConfiguration(spec.Source, spec.Destination, spec.Port, true)
		if err != nil {
			return err
		}
	}
	if spec.Destination.NetworkInterfaceSpec != nil {
		err := t.checkConfiguration(spec.Destination, spec.Source, spec.Port, false)
		if err != nil {
			return err
		}
	}

	if spec.PerformTest {
		if spec.Source.NetworkInterfaceSpec == nil {
			return fmt.Errorf("Source must be in a VPC to perform a network connection test")
		}

		err := t.createPrerequisites(spec.Source, nil)
		if err != nil {
			return fmt.Errorf("Error creating test prerequisites: %w", err)
		}

		if spec.Destination.NetworkInterfaceSpec != nil {
			err = t.createPrerequisites(spec.Destination, spec.Source)
			if err != nil {
				return fmt.Errorf("Error creating test prerequisites: %w", err)
			}
			ip, err := t.startWebServer(spec.Destination, spec.Port)
			if err != nil {
				return fmt.Errorf("Error creating ECS web server: %w", err)
			}
			t.Log("New destination IP is %s", ip)
			spec.Destination.IPAddress.IP = ip
		}

		task, err := t.startECSConnectionTest(spec.Source, spec.Destination.IPAddress.IP, spec.Port)
		if err != nil {
			return fmt.Errorf("Error setting up connection test: %w", err)
		}
		spec.Source.IPAddress.IP, err = t.getTaskIP(spec.Source.NetworkInterfaceSpec.session, task, spec.Source.NetworkInterfaceSpec.UsePublicIP)
		if err != nil {
			return fmt.Errorf("Error getting client task public IP: %w", err)
		}
		t.Log("New source IP is %s", spec.Source.IPAddress.IP)

		// If the destination is a NetworkInterfaceSpec then we have spun up containers on both ends,
		// which will have different IPs than the ones specified as the input to `verify`, so we need
		// to recheck the network configuration to make sure the connection will still be allowed.
		if spec.Destination.NetworkInterfaceSpec != nil {
			err = t.updateSourceIPForInternetEgress(spec.Source, spec.Destination)
			if err != nil {
				return fmt.Errorf("Error determining public IP for source: %w", err)
			}
			err := t.checkConfiguration(spec.Source, spec.Destination, spec.Port, true)
			if err != nil {
				return fmt.Errorf("Error rechecking connectivity after setting up for test: %w", err)
			}
			err = t.checkConfiguration(spec.Destination, spec.Source, spec.Port, false)
			if err != nil {
				return fmt.Errorf("Error rechecking connectivity after setting up for test: %w", err)
			}
		}

		t.Log("Checking the results of opening a network connection to %s:%d", spec.Destination.IPAddress.IP, spec.Port)
		sourceAddr, err := t.getECSConnectionTestResult(spec.Source, task)
		if err != nil {
			return fmt.Errorf("Error testing connection: %w", err)
		}
		if spec.Source.NetworkInterfaceSpec.UsePublicIP || (spec.Destination.NetworkInterfaceSpec != nil && spec.Destination.NetworkInterfaceSpec.UsePublicIP) {
			// Connections over the internet are NATed so the task will report its private IP.
			// Override with the public IP we determined earlier.
			sourceAddr = spec.Source.IPAddress.IP.String()
		}

		if spec.Destination.NetworkInterfaceSpec != nil {
			t.Log(`Test successful! We were able to establish the following connection:
Source:
  %s
  VPC: %s
  Subnet: %s
  Security Groups: %v
Destination:
  %s:%d
  VPC: %s
  Subnet: %s
  Security Groups: %v`,
				sourceAddr,
				spec.Source.NetworkInterfaceSpec.VPCID,
				spec.Source.NetworkInterfaceSpec.SubnetID,
				spec.Source.NetworkInterfaceSpec.SecurityGroupIDs,
				spec.Destination.IPAddress.IP,
				spec.Port,
				spec.Destination.NetworkInterfaceSpec.VPCID,
				spec.Destination.NetworkInterfaceSpec.SubnetID,
				spec.Destination.NetworkInterfaceSpec.SecurityGroupIDs)
		} else {
			t.Log(`Test successful! We were able to establish the following connection:
Source:
  %s
  VPC: %s
  Subnet: %s
  Security Groups: %v
Destination:
  %s:%d`,
				sourceAddr,
				spec.Source.NetworkInterfaceSpec.VPCID,
				spec.Source.NetworkInterfaceSpec.SubnetID,
				spec.Source.NetworkInterfaceSpec.SecurityGroupIDs,
				spec.Destination.IPAddress.IP,
				spec.Port)
		}
	}

	return nil
}

func (t *NetworkConnectionTest) updateSourceIPForInternetEgress(source, destination *Endpoint) error {
	if destination.IPAddress.IPType == IPTypeInternet && source.NetworkInterfaceSpec != nil && !source.NetworkInterfaceSpec.UsePublicIP {
		// Source needs to switch to the public IP of its NAT gateway
		route, err := t.determineDestination(source.NetworkInterfaceSpec.session, source.NetworkInterfaceSpec.SubnetID, destination.IPAddress.IP)
		if err != nil {
			return fmt.Errorf("Failed to determine route followed for %s: %w", destination.IPAddress.IP, err)
		}
		ec2svc := newCancellableEC2(t, source.NetworkInterfaceSpec.session)
		out, err := ec2svc.DescribeNatGateways(&ec2.DescribeNatGatewaysInput{
			NatGatewayIds: []*string{&route},
		})
		if err != nil {
			return fmt.Errorf("Error checking NAT gateway: %w", err)
		}
		if len(out.NatGateways) != 1 {
			return fmt.Errorf("Got %d NAT Gateways for id %q", len(out.NatGateways), route)
		}
		nat := out.NatGateways[0]
		if len(nat.NatGatewayAddresses) != 1 {
			return fmt.Errorf("%d address for %s", len(nat.NatGatewayAddresses), route)
		}
		addr := nat.NatGatewayAddresses[0]
		source.IPAddress.IP = net.ParseIP(aws.StringValue(addr.PublicIp))
		source.IPAddress.IPType = IPTypeInternet
		t.Log("Source IP changed to NAT Gateway IP %s", source.IPAddress.IP)
	}
	return nil
}

func (t *NetworkConnectionTest) checkConfiguration(localEndpoint *Endpoint, remoteEndpoint *Endpoint, port int64, isEgress bool) error {
	// TODO: NACLs
	route, err := t.determineDestination(localEndpoint.NetworkInterfaceSpec.session, localEndpoint.NetworkInterfaceSpec.SubnetID, remoteEndpoint.IPAddress.IP)
	if err != nil {
		return fmt.Errorf("Failed to determine route followed for %s: %w", remoteEndpoint.IPAddress.IP, err)
	}
	t.Log("%s is routed to %q", remoteEndpoint.IPAddress.IP, route)
	ec2svc := newCancellableEC2(t, localEndpoint.NetworkInterfaceSpec.session)
	if remoteEndpoint.IPAddress.IPType == IPTypeInternet {
		if localEndpoint.NetworkInterfaceSpec.UsePublicIP {
			if !strings.HasPrefix(route, "igw-") {
				return fmt.Errorf("%s is a public IP but internet IP %s is not routed to an internet gateway (routed to %s instead)", localEndpoint.IPAddress.IP, remoteEndpoint.IPAddress.IP, route)
			}
		} else if !strings.HasPrefix(route, "nat-") {
			return fmt.Errorf("%s is a private IP but internet IP %s is not routed to an NAT gateway (routed to %s instead)", localEndpoint.IPAddress.IP, remoteEndpoint.IPAddress.IP, route)
		}
	} else if remoteEndpoint.IPAddress.IPType == IPTypeVPC {
		if remoteEndpoint.NetworkInterfaceSpec == nil {
			return fmt.Errorf("NetworkInterfaceSpec is required for VPC IPs")
		}
		if remoteEndpoint.NetworkInterfaceSpec.VPCID == localEndpoint.NetworkInterfaceSpec.VPCID {
			// Special case: same VPC
			if route != "local" {
				return fmt.Errorf("%s is not routed to \"local\" (routed to %s instead)", remoteEndpoint.IPAddress.IP, route)
			}
		} else {
			if !strings.HasPrefix(route, "pcx-") {
				return fmt.Errorf("%s is not routed to a peering connection (routed to %s instead)", remoteEndpoint.IPAddress.IP, route)
			}
			out, err := ec2svc.DescribeVpcPeeringConnections(&ec2.DescribeVpcPeeringConnectionsInput{
				VpcPeeringConnectionIds: []*string{&route},
			})
			if err != nil {
				return fmt.Errorf("Error checking peering connection: %w", err)
			}
			if len(out.VpcPeeringConnections) != 1 {
				return fmt.Errorf("Got %d peering connections for id %q", len(out.VpcPeeringConnections), route)
			}
			pcx := out.VpcPeeringConnections[0]
			matches := func(endpoint *Endpoint, info *ec2.VpcPeeringConnectionVpcInfo) bool {
				if info == nil {
					return false
				}
				if info.Region == nil || info.VpcId == nil {
					return false
				}
				return *info.Region == endpoint.NetworkInterfaceSpec.Region && *info.VpcId == endpoint.NetworkInterfaceSpec.VPCID
			}
			localIsAccepter := matches(localEndpoint, pcx.AccepterVpcInfo) && matches(remoteEndpoint, pcx.RequesterVpcInfo)
			localIsRequester := matches(localEndpoint, pcx.RequesterVpcInfo) && matches(remoteEndpoint, pcx.AccepterVpcInfo)
			if !(localIsAccepter || localIsRequester) {
				return fmt.Errorf("%s does not connect the right VPCs", route)
			}
			if pcx.Status == nil {
				return fmt.Errorf("Unable to determine status of %s", route)
			}
			status := aws.StringValue(pcx.Status.Code)
			if status != ec2.VpcPeeringConnectionStateReasonCodeActive {
				return fmt.Errorf("%s status is %q", route, status)
			}
		}
	} else if remoteEndpoint.IPAddress.IPType == IPTypeSharedService {
		// TODO: verify other things about the TGW?
		if !strings.HasPrefix(route, "tgw-") {
			return fmt.Errorf("%s is not routed to a transit gateway (routed to %s instead)", remoteEndpoint.IPAddress.IP, route)
		}
	} else {
		return fmt.Errorf("Unknown IP type %d", remoteEndpoint.IPAddress.IPType)
	}

	if isEgress && localEndpoint.NetworkInterfaceSpec.CreateEgressSecurityGroup {
		t.Log("Skipping security group check; will create one allowing egress to all IPs/ports")
	} else {
		ingressGroups, egressGroups, err := t.determineAuthorizingGroups(localEndpoint, remoteEndpoint, port)
		if err != nil {
			return fmt.Errorf("Failed to determine security group that applies to %s: %w", remoteEndpoint.IPAddress.IP, err)
		}
		if isEgress {
			if len(egressGroups) == 0 {
				return fmt.Errorf("No group allows egress to %s on port %d", remoteEndpoint.IPAddress.IP, port)
			}
			for _, egressGroup := range egressGroups {
				t.Log("egress to %s is allowed by security group %q (%q)", remoteEndpoint.IPAddress.IP, aws.StringValue(egressGroup.GroupId), aws.StringValue(egressGroup.GroupName))
			}
		} else {
			if len(ingressGroups) == 0 {
				return fmt.Errorf("No group allows ingress from %s on port %d", remoteEndpoint.IPAddress.IP, port)
			}
			for _, ingressGroup := range ingressGroups {
				t.Log("ingress from %s is allowed by security group %q (%q)", remoteEndpoint.IPAddress.IP, aws.StringValue(ingressGroup.GroupId), aws.StringValue(ingressGroup.GroupName))
			}
		}
	}

	return nil
}

func (t *NetworkConnectionTest) revokeEgress(endpoint *Endpoint) error {
	ec2svc := newCancellableEC2(t, endpoint.NetworkInterfaceSpec.session)
	_, err := ec2svc.RevokeSecurityGroupEgress(&ec2.RevokeSecurityGroupEgressInput{
		GroupId:       aws.String(endpoint.NetworkInterfaceSpec.egressSecurityGroupID),
		IpPermissions: egressToAllDestinations,
	})
	return err
}

func (t *NetworkConnectionTest) createPrerequisites(endpoint *Endpoint, alreadyCreatedFor *Endpoint) error {
	if alreadyCreatedFor == nil || alreadyCreatedFor.NetworkInterfaceSpec.AccountID != endpoint.NetworkInterfaceSpec.AccountID {
		ecssvc := newCancellableECS(t, endpoint.NetworkInterfaceSpec.session)
		for {
			out, err := ecssvc.DescribeClusters(&ecs.DescribeClustersInput{
				Clusters: []*string{aws.String(ecsClusterName)},
			})
			if err != nil {
				return fmt.Errorf("Error describing clusters: %w", err)
			}
			status := "FAKE_STATUS_NO_CLUSTERS"
			if len(out.Clusters) > 0 {
				status = aws.StringValue(out.Clusters[0].Status)
			}
			if status == "DEPROVISIONING" {
				t.Log("ECS cluster is still deprovisioning from a previous test")
			} else {
				if status != "ACTIVE" && status != "PROVISIONING" {
					_, err := ecssvc.CreateCluster(&ecs.CreateClusterInput{
						ClusterName:       aws.String(ecsClusterName),
						CapacityProviders: []*string{aws.String("FARGATE")},
					})
					if err != nil {
						return fmt.Errorf("Error creating ECS cluster: %w", err)
					}
					t.rollbacks = append(t.rollbacks, func() (string, error) {
						_, err := ecssvc.ECS.DeleteCluster(&ecs.DeleteClusterInput{
							Cluster: aws.String(ecsClusterName),
						})
						return "ECS Cluster " + ecsClusterName, err
					})
				}
				break
			}
			time.Sleep(time.Second)
		}

		logssvc := newCancellableCloudWatchLogs(t, endpoint.NetworkInterfaceSpec.session)
		logsOut, err := logssvc.DescribeLogGroups(&cloudwatchlogs.DescribeLogGroupsInput{
			LogGroupNamePrefix: aws.String("/ecs/" + ecsClusterName),
		})
		if err != nil {
			return fmt.Errorf("Error listing CloudWatch Logs groups: %w", err)
		}
		found := false
		for _, group := range logsOut.LogGroups {
			if aws.StringValue(group.LogGroupName) == "/ecs/"+ecsClusterName {
				found = true
			}
		}
		if !found {
			_, err = logssvc.CreateLogGroup(&cloudwatchlogs.CreateLogGroupInput{
				LogGroupName: aws.String("/ecs/" + ecsClusterName),
			})
			if err != nil {
				return fmt.Errorf("Error creating CloudWatch Logs group: %w", err)
			}
			_, err = logssvc.PutRetentionPolicy(&cloudwatchlogs.PutRetentionPolicyInput{
				LogGroupName:    aws.String("/ecs/" + ecsClusterName),
				RetentionInDays: aws.Int64(7),
			})
			if err != nil {
				return fmt.Errorf("Error setting CloudWatch Logs group retention policy: %w", err)
			}
		}

		iamsvc := newCancellableIAM(t, endpoint.NetworkInterfaceSpec.session)
		roleOut, err := iamsvc.CreateRole(&iam.CreateRoleInput{
			AssumeRolePolicyDocument: aws.String(`{
			"Version": "2012-10-17",
			"Statement": [
			  {
				"Sid": "",
				"Effect": "Allow",
				"Principal": {
				  "Service": "ecs-tasks.amazonaws.com"
				},
				"Action": "sts:AssumeRole"
			  }
			]
		  }`),
			RoleName: aws.String(ecsTaskRoleName),
		})
		if err != nil {
			return fmt.Errorf("Error creating role for ecs tasks in account %s: %w", endpoint.NetworkInterfaceSpec.AccountID, err)
		}
		t.rollbacks = append(t.rollbacks, func() (string, error) {
			_, err := iamsvc.IAM.DeleteRole(&iam.DeleteRoleInput{
				RoleName: roleOut.Role.RoleName,
			})
			return "IAM Role " + aws.StringValue(roleOut.Role.RoleName), err
		})

		policyName := "debug-test-network-client"
		_, err = iamsvc.PutRolePolicy(&iam.PutRolePolicyInput{
			RoleName:   roleOut.Role.RoleName,
			PolicyName: aws.String(policyName),
			PolicyDocument: aws.String(fmt.Sprintf(`{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Effect": "Allow",
					"Action": [
						"dynamodb:GetItem",
						"dynamodb:PutItem",
						"dynamodb:UpdateItem"
					],
					"Resource": "arn:aws:dynamodb:%s:%s:table/%s"
				},
				{
					"Effect": "Allow",
					"Action": [
						"logs:*"
					],
					"Resource": "arn:aws:logs:%s:%s:log-group:/ecs/%s:*"
				}
			]
		}`, endpoint.NetworkInterfaceSpec.Region, endpoint.NetworkInterfaceSpec.AccountID, dynamodbTableName,
				endpoint.NetworkInterfaceSpec.Region, endpoint.NetworkInterfaceSpec.AccountID, ecsClusterName)),
		})
		if err != nil {
			return fmt.Errorf("Error giving ecs task role permissions: %w", err)
		}
		t.rollbacks = append(t.rollbacks, func() (string, error) {
			_, err := iamsvc.IAM.DeleteRolePolicy(&iam.DeleteRolePolicyInput{
				RoleName:   roleOut.Role.RoleName,
				PolicyName: aws.String(policyName),
			})
			return fmt.Sprintf("Policy %s on IAM Role %s", policyName, aws.StringValue(roleOut.Role.RoleName)), err
		})

		t.Log("waiting 10 seconds for ECS task permissions to settle")
		time.Sleep(10 * time.Second)
	}

	ec2svc := newCancellableEC2(t, endpoint.NetworkInterfaceSpec.session)
	name := fmt.Sprintf("test-debug-all-egress-%d", rand.Int())
	sgOut, err := ec2svc.CreateSecurityGroup(&ec2.CreateSecurityGroupInput{
		GroupName:   aws.String(name),
		Description: aws.String(name),
		VpcId:       &endpoint.NetworkInterfaceSpec.VPCID,
	})
	if err != nil {
		return fmt.Errorf("Error creating egress security group: %w", err)
	}
	endpoint.NetworkInterfaceSpec.egressSecurityGroupID = aws.StringValue(sgOut.GroupId)
	t.rollbacks = append(t.rollbacks, func() (string, error) {
		_, err := ec2svc.EC2.DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
			GroupId: sgOut.GroupId,
		})
		return "Security Group " + aws.StringValue(sgOut.GroupId), err
	})
	_, err = ec2svc.AuthorizeSecurityGroupEgress(&ec2.AuthorizeSecurityGroupEgressInput{
		GroupId:       sgOut.GroupId,
		IpPermissions: egressToAllDestinations,
	})
	if err != nil {
		return fmt.Errorf("Error creating egress security group: %w", err)
	}

	return nil
}

func (t *NetworkConnectionTest) startECSConnectionTest(endpoint *Endpoint, ip net.IP, port int64) (*ecs.Task, error) {
	t.Log("Provisioning a test container on ECS in %s", endpoint.NetworkInterfaceSpec.VPCID)

	dynamodbsvc := newCancellableDynamoDB(t, endpoint.NetworkInterfaceSpec.session)
	_, err := dynamodbsvc.CreateTable(&dynamodb.CreateTableInput{
		TableName:   aws.String(dynamodbTableName),
		BillingMode: aws.String(dynamodb.BillingModePayPerRequest),
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			{
				AttributeName: aws.String(dynamodbPrimaryKey),
				AttributeType: aws.String(dynamodb.ScalarAttributeTypeS),
			},
		},
		KeySchema: []*dynamodb.KeySchemaElement{
			{
				AttributeName: aws.String(dynamodbPrimaryKey),
				KeyType:       aws.String(dynamodb.KeyTypeHash),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("Error creating dynamodb table: %w", err)
	}
	t.rollbacks = append(t.rollbacks, func() (string, error) {
		_, err := dynamodbsvc.DynamoDB.DeleteTable(&dynamodb.DeleteTableInput{
			TableName: aws.String(dynamodbTableName),
		})
		return "DynamoDB table " + dynamodbTableName, err
	})
	t.Log("ecs client: waiting for dynamodb table to become available")
	err = t.wait(func() (bool, string, error) {
		out, err := dynamodbsvc.DescribeTable(&dynamodb.DescribeTableInput{
			TableName: aws.String(dynamodbTableName),
		})
		if err != nil {
			return false, "", fmt.Errorf("Error listing cluster: %s", err)
		} else if aws.StringValue(out.Table.TableStatus) == dynamodb.TableStatusCreating {
			return false, fmt.Sprintf("Status is still %s", aws.StringValue(out.Table.TableStatus)), nil
		} else if aws.StringValue(out.Table.TableStatus) != dynamodb.TableStatusActive {
			return false, "", fmt.Errorf("Table status is %s", aws.StringValue(out.Table.TableStatus))
		} else {
			return true, "", nil
		}
	}, 0)
	if err != nil {
		return nil, err
	}

	securityGroupIDs := []*string{aws.String(endpoint.NetworkInterfaceSpec.egressSecurityGroupID)}
	securityGroupIDs = append(securityGroupIDs, aws.StringSlice(endpoint.NetworkInterfaceSpec.SecurityGroupIDs)...)

	ecssvc := newCancellableECS(t, endpoint.NetworkInterfaceSpec.session)
	t.Log("ecs client: waiting for cluster to become available")
	err = t.wait(func() (bool, string, error) {
		out, err := ecssvc.DescribeClusters(&ecs.DescribeClustersInput{
			Clusters: []*string{aws.String(ecsClusterName)},
		})
		if err != nil {
			return false, "", fmt.Errorf("Error listing cluster: %s", err)
		} else if len(out.Clusters) != 1 {
			return false, "", fmt.Errorf("Got %d clusters", len(out.Clusters))
		} else if aws.StringValue(out.Clusters[0].Status) != "ACTIVE" {
			return false, fmt.Sprintf("Status is still %s", aws.StringValue(out.Clusters[0].Status)), nil
		} else {
			return true, "", nil
		}
	}, 0)
	if err != nil {
		return nil, err
	}

	// TODO: get container from ECR
	tdOut, err := ecssvc.RegisterTaskDefinition(&ecs.RegisterTaskDefinitionInput{
		Family: aws.String(clientTaskName),
		ContainerDefinitions: []*ecs.ContainerDefinition{
			{
				Name: aws.String(clientTaskName),
				// https://github.com/corbaltcode/connect-from-ecs
				Image:   aws.String("superbrilliant/connect-from-ecs:latest"),
				Command: []*string{aws.String(fmt.Sprintf("%s:%d", ip, port))},
				Environment: []*ecs.KeyValuePair{
					{
						Name:  aws.String("DYNAMODB_TABLE"),
						Value: aws.String(dynamodbTableName),
					},
				},
				LogConfiguration: &ecs.LogConfiguration{
					LogDriver: aws.String(ecs.LogDriverAwslogs),
					Options: map[string]*string{
						"awslogs-region":        &endpoint.NetworkInterfaceSpec.Region,
						"awslogs-group":         aws.String("/ecs/" + ecsClusterName),
						"awslogs-stream-prefix": aws.String("ecs"),
					},
				},
			},
		},
		Cpu:              aws.String("256"),
		Memory:           aws.String("512"),
		NetworkMode:      aws.String("awsvpc"),
		ExecutionRoleArn: aws.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", endpoint.NetworkInterfaceSpec.AccountID, ecsTaskRoleName)),
		TaskRoleArn:      aws.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", endpoint.NetworkInterfaceSpec.AccountID, ecsTaskRoleName)),
	})
	if err != nil {
		return nil, fmt.Errorf("Error creating task definition: %w", err)
	}
	t.rollbacks = append(t.rollbacks, func() (string, error) {
		_, err := ecssvc.ECS.DeregisterTaskDefinition(&ecs.DeregisterTaskDefinitionInput{
			TaskDefinition: tdOut.TaskDefinition.TaskDefinitionArn,
		})
		return "ECS Task Definition " + aws.StringValue(tdOut.TaskDefinition.TaskDefinitionArn), err
	})

	taskDef := fmt.Sprintf("%s:%d",
		aws.StringValue(tdOut.TaskDefinition.Family),
		aws.Int64Value(tdOut.TaskDefinition.Revision))
	netConfig := &ecs.NetworkConfiguration{
		AwsvpcConfiguration: &ecs.AwsVpcConfiguration{
			Subnets:        []*string{&endpoint.NetworkInterfaceSpec.SubnetID},
			SecurityGroups: securityGroupIDs,
		},
	}
	if endpoint.NetworkInterfaceSpec.UsePublicIP {
		netConfig.AwsvpcConfiguration.AssignPublicIp = aws.String(ecs.AssignPublicIpEnabled)
	}
	runOut, err := ecssvc.RunTask(&ecs.RunTaskInput{
		Cluster:              aws.String(ecsClusterName),
		LaunchType:           aws.String("FARGATE"),
		NetworkConfiguration: netConfig,
		TaskDefinition:       aws.String(taskDef),
	})
	if err != nil {
		return nil, fmt.Errorf("Error creating ECS task: %w", err)
	}

	t.rollbacks = append(t.rollbacks, func() (string, error) {
		t.Log("ecs client: waiting for task to be stopped")
		err = t.wait(func() (bool, string, error) {
			out, err := ecssvc.ECS.DescribeTasks(&ecs.DescribeTasksInput{
				Cluster: aws.String(ecsClusterName),
				Tasks:   []*string{runOut.Tasks[0].TaskArn},
			})
			if err != nil {
				return false, "", fmt.Errorf("Error listing task: %w", err)
			} else if len(out.Tasks) != 1 {
				return false, "", fmt.Errorf("Got %d tasks", len(out.Tasks))
			} else if aws.StringValue(out.Tasks[0].LastStatus) == "STOPPED" {
				return true, "", nil
			} else if aws.StringValue(out.Tasks[0].LastStatus) == "RUNNING" {
				_, err := ecssvc.ECS.StopTask(&ecs.StopTaskInput{
					Cluster: aws.String(ecsClusterName),
					Task:    out.Tasks[0].TaskArn,
				})
				if err != nil {
					return false, "", fmt.Errorf("Error stopping task: %w", err)
				}
			}
			return false, fmt.Sprintf("Task status is still %s", aws.StringValue(out.Tasks[0].LastStatus)), nil
		}, 5*time.Minute)
		return "", err
	})

	t.Log("ecs client: waiting for task to be running")
	var task *ecs.Task
	err = t.wait(func() (bool, string, error) {
		out, err := ecssvc.DescribeTasks(&ecs.DescribeTasksInput{
			Cluster: aws.String(ecsClusterName),
			Tasks:   []*string{runOut.Tasks[0].TaskArn},
		})
		if err != nil {
			return false, "", fmt.Errorf("Error listing task: %w", err)
		} else if len(out.Tasks) != 1 {
			return false, "", fmt.Errorf("Got %d tasks", len(out.Tasks))
		} else if aws.StringValue(out.Tasks[0].LastStatus) == "STOPPED" {
			return false, "", fmt.Errorf("Task failed")
		} else if aws.StringValue(out.Tasks[0].LastStatus) != "RUNNING" {
			return false, fmt.Sprintf("Task status is still %s", aws.StringValue(out.Tasks[0].LastStatus)), nil
		} else {
			task = out.Tasks[0]
			return true, "", nil
		}
	}, 0)
	if err != nil {
		return nil, err
	}

	if !endpoint.NetworkInterfaceSpec.CreateEgressSecurityGroup {
		err = t.revokeEgress(endpoint)
		if err != nil {
			return nil, fmt.Errorf("Error removing egress group: %w", err)
		}
		// Give security group changes a few seconds to take effect
		time.Sleep(5 * time.Second)
	}

	return task, nil
}

func (t *NetworkConnectionTest) getECSConnectionTestResult(endpoint *Endpoint, task *ecs.Task) (string, error) {

	statusKey := "status"
	addressKey := "address"

	dynamodbsvc := newCancellableDynamoDB(t, endpoint.NetworkInterfaceSpec.session)
	_, err := dynamodbsvc.UpdateItem(&dynamodb.UpdateItemInput{
		TableName: aws.String(dynamodbTableName),
		Key: map[string]*dynamodb.AttributeValue{
			dynamodbPrimaryKey: {
				S: task.TaskArn,
			},
		},
		AttributeUpdates: map[string]*dynamodb.AttributeValueUpdate{
			statusKey: {
				Action: aws.String(dynamodb.AttributeActionPut),
				Value: &dynamodb.AttributeValue{
					S: aws.String("ready"),
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("Error writing ready flag: %w", err)
	}

	t.Log("Waiting for task to finish running")

	t.Log("ecs client: waiting for task to be done")
	ecssvc := newCancellableECS(t, endpoint.NetworkInterfaceSpec.session)
	err = t.wait(func() (bool, string, error) {
		out, err := ecssvc.DescribeTasks(&ecs.DescribeTasksInput{
			Cluster: aws.String(ecsClusterName),
			Tasks:   []*string{task.TaskArn},
		})
		if err != nil {
			return false, "", fmt.Errorf("Error listing task: %w", err)
		} else if len(out.Tasks) != 1 {
			return false, "", fmt.Errorf("Got %d tasks", len(out.Tasks))
		} else if aws.StringValue(out.Tasks[0].LastStatus) == "STOPPED" || aws.StringValue(out.Tasks[0].LastStatus) == "DEPROVISIONING" {
			return true, "", nil
		}
		return false, fmt.Sprintf("Task status is still %s", aws.StringValue(out.Tasks[0].LastStatus)), nil
	}, 0)
	if err != nil {
		return "", err
	}
	out, err := dynamodbsvc.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(dynamodbTableName),
		Key: map[string]*dynamodb.AttributeValue{
			dynamodbPrimaryKey: {
				S: task.TaskArn,
			},
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return "", fmt.Errorf("Error getting task result: %w", err)
	}
	if out.Item == nil {
		return "", fmt.Errorf("Error getting task result: task item missing")
	} else if out.Item[statusKey] == nil {
		return "", fmt.Errorf("Error getting task result: status key missing")
	} else if aws.StringValue(out.Item[statusKey].S) == "ready" {
		return "", fmt.Errorf("Error getting task result: status key not updated; program probably crashed?")
	} else {
		address := "?"
		if out.Item[addressKey] != nil {
			address = aws.StringValue(out.Item[addressKey].S)
		}
		status := aws.StringValue(out.Item[statusKey].S)
		if status == "success" {
			return address, nil
		} else {
			return address, fmt.Errorf("Connect failure: %s", status)
		}
	}
}

func (t *NetworkConnectionTest) getTaskIP(sess *session.Session, task *ecs.Task, public bool) (net.IP, error) {

	if len(task.Attachments) != 1 {
		return nil, fmt.Errorf("Task has %d attachments", len(task.Attachments))
	}
	var networkInterfaceID *string
	var ip net.IP
	for _, kvp := range task.Attachments[0].Details {
		key := aws.StringValue(kvp.Name)
		if key == "networkInterfaceId" {
			networkInterfaceID = kvp.Value
		} else if key == "privateIPv4Address" {
			ip = net.ParseIP(aws.StringValue(kvp.Value))
		}
	}
	if networkInterfaceID == nil {
		return nil, fmt.Errorf("Could not find network interface ID for task")
	}
	if ip == nil {
		return nil, fmt.Errorf("Could not find IP Address for task")
	}
	if public {
		// Find public IP for ENI
		ec2svc := newCancellableEC2(t, sess)
		out, err := ec2svc.DescribeNetworkInterfaces(&ec2.DescribeNetworkInterfacesInput{
			NetworkInterfaceIds: []*string{networkInterfaceID},
		})
		if err != nil {
			return nil, fmt.Errorf("error getting server public IP: %w", err)
		}
		if len(out.NetworkInterfaces) != 1 {
			return nil, fmt.Errorf("Got %d network interfaces with id %s for getting ECS task", len(out.NetworkInterfaces), *networkInterfaceID)
		}
		iface := out.NetworkInterfaces[0]
		if iface.Association == nil || iface.Association.PublicIp == nil {
			return nil, fmt.Errorf("No public IP for interface %s for server ECS task", *networkInterfaceID)
		}
		ip = net.ParseIP(aws.StringValue(iface.Association.PublicIp))
	}
	return ip, nil
}

type statusChecker func() (done bool, msg string, err error)

func (t *NetworkConnectionTest) wait(checker statusChecker, limit time.Duration) error {
	lastMessageTime := time.Now()
	startTime := time.Now()
	for done, msg, err := checker(); !done; done, msg, err = checker() {
		if err != nil {
			return err
		}
		if limit > 0 && time.Since(startTime) > limit {
			return fmt.Errorf("Timed out waiting")
		} else if time.Since(lastMessageTime) > 10*time.Second {
			t.Log(msg)
			lastMessageTime = time.Now()
		} else {
			time.Sleep(time.Second)
		}
	}
	return nil
}

func (t *NetworkConnectionTest) startWebServer(endpoint *Endpoint, port int64) (net.IP, error) {
	// Create a security group allowing all egress to pull the docker image
	t.Log("Provisioning a web server on ECS in %s", endpoint.NetworkInterfaceSpec.VPCID)
	securityGroupIDs := []*string{aws.String(endpoint.NetworkInterfaceSpec.egressSecurityGroupID)}
	securityGroupIDs = append(securityGroupIDs, aws.StringSlice(endpoint.NetworkInterfaceSpec.SecurityGroupIDs)...)
	ecssvc := newCancellableECS(t, endpoint.NetworkInterfaceSpec.session)
	t.Log("ecs server: waiting for cluster to become available")
	err := t.wait(func() (bool, string, error) {
		out, err := ecssvc.DescribeClusters(&ecs.DescribeClustersInput{
			Clusters: []*string{aws.String(ecsClusterName)},
		})
		if err != nil {
			return false, "", err
		} else if len(out.Clusters) != 1 {
			return false, "", fmt.Errorf("Got %d clusters", len(out.Clusters))
		} else if aws.StringValue(out.Clusters[0].Status) != "ACTIVE" {
			return false, fmt.Sprintf("Status is still %s", aws.StringValue(out.Clusters[0].Status)), nil
		} else {
			return true, "", nil
		}
	}, 0)
	if err != nil {
		return nil, err
	}

	// TODO: get container from ECR
	tdOut, err := ecssvc.RegisterTaskDefinition(&ecs.RegisterTaskDefinitionInput{
		Family: aws.String(serverTaskName),
		ContainerDefinitions: []*ecs.ContainerDefinition{
			{
				Name:    aws.String(serverTaskName),
				Image:   aws.String("containous/whoami"),
				Command: []*string{aws.String("-port"), aws.String(fmt.Sprintf("%d", port))},
				PortMappings: []*ecs.PortMapping{
					{
						ContainerPort: aws.Int64(80),
					},
				},
				LogConfiguration: &ecs.LogConfiguration{
					LogDriver: aws.String(ecs.LogDriverAwslogs),
					Options: map[string]*string{
						"awslogs-region":        &endpoint.NetworkInterfaceSpec.Region,
						"awslogs-group":         aws.String("/ecs/" + ecsClusterName),
						"awslogs-stream-prefix": aws.String("ecs"),
					},
				},
			},
		},
		Cpu:              aws.String("256"),
		Memory:           aws.String("512"),
		NetworkMode:      aws.String("awsvpc"),
		ExecutionRoleArn: aws.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", endpoint.NetworkInterfaceSpec.AccountID, ecsTaskRoleName)),
	})
	if err != nil {
		return nil, fmt.Errorf("Error creating task definition: %w", err)
	}
	t.rollbacks = append(t.rollbacks, func() (string, error) {
		_, err := ecssvc.ECS.DeregisterTaskDefinition(&ecs.DeregisterTaskDefinitionInput{
			TaskDefinition: tdOut.TaskDefinition.TaskDefinitionArn,
		})
		return "ECS Task Definition " + aws.StringValue(tdOut.TaskDefinition.TaskDefinitionArn), err
	})
	taskDef := fmt.Sprintf("%s:%d",
		aws.StringValue(tdOut.TaskDefinition.Family),
		aws.Int64Value(tdOut.TaskDefinition.Revision))
	netConfig := &ecs.NetworkConfiguration{
		AwsvpcConfiguration: &ecs.AwsVpcConfiguration{
			Subnets:        []*string{&endpoint.NetworkInterfaceSpec.SubnetID},
			SecurityGroups: securityGroupIDs,
		},
	}
	if endpoint.NetworkInterfaceSpec.UsePublicIP {
		netConfig.AwsvpcConfiguration.AssignPublicIp = aws.String(ecs.AssignPublicIpEnabled)
	}
	runOut, err := ecssvc.RunTask(&ecs.RunTaskInput{
		Cluster:              aws.String(ecsClusterName),
		LaunchType:           aws.String("FARGATE"),
		NetworkConfiguration: netConfig,
		TaskDefinition:       aws.String(taskDef),
	})
	if err != nil {
		return nil, fmt.Errorf("Error creating ECS task: %w", err)
	}

	t.rollbacks = append(t.rollbacks, func() (string, error) {
		t.Log("ecs server: waiting for task to be stopped")
		err = t.wait(func() (bool, string, error) {
			out, err := ecssvc.ECS.DescribeTasks(&ecs.DescribeTasksInput{
				Cluster: aws.String(ecsClusterName),
				Tasks:   []*string{runOut.Tasks[0].TaskArn},
			})
			if err != nil {
				return false, "", fmt.Errorf("Error listing task: %w", err)
			} else if len(out.Tasks) != 1 {
				return false, "", fmt.Errorf("Got %d tasks", len(out.Tasks))
			} else if aws.StringValue(out.Tasks[0].LastStatus) == "STOPPED" {
				return true, "", nil
			} else if aws.StringValue(out.Tasks[0].LastStatus) == "RUNNING" {
				_, err := ecssvc.ECS.StopTask(&ecs.StopTaskInput{
					Cluster: aws.String(ecsClusterName),
					Task:    out.Tasks[0].TaskArn,
				})
				if err != nil {
					return false, "", fmt.Errorf("Error stopping task: %w", err)
				}
			}
			return false, fmt.Sprintf("Task status is still %s", aws.StringValue(out.Tasks[0].LastStatus)), nil
		}, 5*time.Minute)
		return "", err
	})

	t.Log("ecs server: waiting for task to be running")
	var task *ecs.Task
	err = t.wait(func() (bool, string, error) {
		out, err := ecssvc.DescribeTasks(&ecs.DescribeTasksInput{
			Cluster: aws.String(ecsClusterName),
			Tasks:   []*string{runOut.Tasks[0].TaskArn},
		})
		if err != nil {
			return false, "", fmt.Errorf("Error listing task: %w", err)
		} else if len(out.Tasks) != 1 {
			return false, "", fmt.Errorf("Got %d tasks", len(out.Tasks))
		} else if aws.StringValue(out.Tasks[0].LastStatus) == "STOPPED" {
			return false, "", fmt.Errorf("Task failed")
		} else if aws.StringValue(out.Tasks[0].LastStatus) != "RUNNING" {
			return false, fmt.Sprintf("Task status is still %s", aws.StringValue(out.Tasks[0].LastStatus)), nil
		} else {
			task = out.Tasks[0]
			return true, "", nil
		}
	}, 0)
	if err != nil {
		return nil, err
	}

	ip, err := t.getTaskIP(endpoint.NetworkInterfaceSpec.session, task, endpoint.NetworkInterfaceSpec.UsePublicIP)
	if err != nil {
		return nil, fmt.Errorf("Error determining server task IP: %w", err)
	}

	err = t.revokeEgress(endpoint)
	if err != nil {
		return nil, fmt.Errorf("Error removing egress group: %w", err)
	}

	return ip, nil
}

func getRouteDestination(route *ec2.Route) string {
	for _, s := range []*string{
		route.GatewayId,
		route.InstanceId,
		route.LocalGatewayId,
		route.NatGatewayId,
		route.NetworkInterfaceId,
		route.TransitGatewayId,
		route.VpcPeeringConnectionId,
	} {
		if s != nil {
			return *s
		}
	}
	return ""
}

func (t *NetworkConnectionTest) determineDestination(sess *session.Session, subnetID string, ip net.IP) (string, error) {
	ec2svc := newCancellableEC2(t, sess)
	out, err := ec2svc.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("association.subnet-id"),
				Values: []*string{&subnetID},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("Error determining route table for %q: %w", subnetID, err)
	}
	if len(out.RouteTables) != 1 {
		return "", fmt.Errorf("Found %d route tables associated with %q", len(out.RouteTables), subnetID)
	}
	routeTable := out.RouteTables[0]
	matchingSize := -1
	matchingDestination := ""
	destinationWithinRoute := func(cidr *string) (bool, error) {
		_, ipNet, err := net.ParseCIDR(*cidr)
		if err != nil {
			return false, fmt.Errorf("Unable to parse route %s: %w", *cidr, err)
		}
		if ipNet.Contains(ip) {
			size, _ := ipNet.Mask.Size()
			if size > matchingSize {
				matchingSize = size
				return true, nil
			}
		}
		return false, nil
	}
	for _, route := range routeTable.Routes {
		if aws.StringValue(route.State) == ec2.RouteStateBlackhole {
			t.Log("Skipping blackhole route %s", route.String())
			continue
		}
		if route.DestinationCidrBlock != nil {
			within, err := destinationWithinRoute(route.DestinationCidrBlock)
			if err != nil {
				return "", err
			}
			if within {
				matchingDestination = getRouteDestination(route)
			}
		} else if route.DestinationPrefixListId != nil {
			plout, err := ec2svc.GetManagedPrefixListEntries(
				&ec2.GetManagedPrefixListEntriesInput{
					PrefixListId: route.DestinationPrefixListId,
				},
			)
			if err != nil {
				return "", fmt.Errorf("Error determining entries for prefix list %q: %w", *route.DestinationPrefixListId, err)
			}
			for _, entry := range plout.Entries {
				within, err := destinationWithinRoute(entry.Cidr)
				if err != nil {
					return "", err
				}
				if within {
					matchingDestination = getRouteDestination(route)
				}
			}
		} else {
			t.Log("Skipping route %s with no ipv4 cidr", route.String())
			continue
		}
	}
	if matchingSize == -1 {
		return "", fmt.Errorf("No route matched %s", ip)
	}
	return matchingDestination, nil
}

func stringInSlice(s string, sl []string) bool {
	for _, t := range sl {
		if s == t {
			return true
		}
	}
	return false
}

func destinationIsAllowed(source *Endpoint, destination *Endpoint, port int64, perms []*ec2.IpPermission) (bool, error) {
	for _, perm := range perms {
		protocol := aws.StringValue(perm.IpProtocol)
		if protocol != "-1" && protocol != "tcp" {
			continue
		}
		if protocol != "-1" {
			fromPort := aws.Int64Value(perm.FromPort)
			toPort := aws.Int64Value(perm.ToPort)
			if fromPort > port || toPort < port {
				continue
			}
		}
		if source.IPAddress.IPType == IPTypeVPC && destination.IPAddress.IPType == IPTypeVPC {
			if destination.NetworkInterfaceSpec != nil && destination.NetworkInterfaceSpec.Region == source.NetworkInterfaceSpec.Region {
				for _, ug := range perm.UserIdGroupPairs {
					if stringInSlice(aws.StringValue(ug.GroupId), destination.NetworkInterfaceSpec.SecurityGroupIDs) {
						return true, nil
					}
				}
			}
		}
		for _, ipRange := range perm.IpRanges {
			cidr := aws.StringValue(ipRange.CidrIp)
			if strings.HasPrefix(cidr, "sg-") {
				if destination.NetworkInterfaceSpec != nil && destination.NetworkInterfaceSpec.Region == source.NetworkInterfaceSpec.Region {
					if stringInSlice(cidr, destination.NetworkInterfaceSpec.SecurityGroupIDs) {
						return true, nil
					}
				}
			} else {
				_, ipNet, err := net.ParseCIDR(cidr)
				if err != nil {
					return false, fmt.Errorf("Unable to parse CIDR %q: %w", cidr, err)
				}
				if ipNet.Contains(destination.IPAddress.IP) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (t *NetworkConnectionTest) determineAuthorizingGroups(source, destination *Endpoint, port int64) ([]*ec2.SecurityGroup, []*ec2.SecurityGroup, error) {
	// TODO: make sure cross-account sg security group references work
	ec2svc := newCancellableEC2(t, source.NetworkInterfaceSpec.session)
	var matchingIngressGroups []*ec2.SecurityGroup
	var matchingEgressGroups []*ec2.SecurityGroup
	groups := source.NetworkInterfaceSpec.SecurityGroupIDs
	if len(groups) > 0 {
		out, err := ec2svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
			GroupIds: aws.StringSlice(groups),
		})
		if err != nil {
			return nil, nil, fmt.Errorf("Error listing groups: %w", err)
		}
		if len(out.SecurityGroups) != len(groups) {
			return nil, nil, fmt.Errorf("Got %d groups for %d IDs", len(out.SecurityGroups), len(groups))
		}
		for _, group := range out.SecurityGroups {
			matches, err := destinationIsAllowed(source, destination, port, group.IpPermissions)
			if err != nil {
				return nil, nil, fmt.Errorf("Error getting ingress permissions for group %q: %w", aws.StringValue(group.GroupId), err)
			}
			if matches {
				matchingIngressGroups = append(matchingIngressGroups, group)
			}
			matches, err = destinationIsAllowed(source, destination, port, group.IpPermissionsEgress)
			if err != nil {
				return nil, nil, fmt.Errorf("Error getting egress permissions for group %q: %w", aws.StringValue(group.GroupId), err)
			}
			if matches {
				matchingEgressGroups = append(matchingEgressGroups, group)
			}
		}
	}
	return matchingIngressGroups, matchingEgressGroups, nil
}
