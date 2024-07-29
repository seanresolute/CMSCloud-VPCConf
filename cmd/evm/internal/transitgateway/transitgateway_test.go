package transitgateway

import (
	"fmt"
	"log"
	"reflect"
	"regexp"
	"runtime"
	"testing"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cmd/evm/internal/conf"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/vpcconfapi"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/ram"
	"github.com/aws/aws-sdk-go/service/ram/ramiface"
)

var (
	ID                 string         = "tgw-abcdef0123456789"
	OwnerID            string         = "012345678901234"
	ARN                string         = fmt.Sprintf("arn:aws:ec2:us-east-1:%s:transit-gateway/%s", OwnerID, ID)
	ShareARN           string         = fmt.Sprintf("arn:aws:ram:us-east-1:%s:resource-share/abcdef01-b012-c345-d678-901234abcdef", OwnerID)
	ShareInvitationARN string         = fmt.Sprintf("arn:aws:ram:us-east-1:%s:resource-share-invitation/01abcdef-1abc-2def-3abc-456789abcdef", OwnerID)
	TemplateName       string         = "InterVPC-East-Prod"
	ResourceShareName  string         = "InterVPC-East-Prod"
	mockAccountID      string         = "98765432109876"
	callNameRegex      *regexp.Regexp = regexp.MustCompile(`^.*\.(.*)`)
)

type CallList []string

func (cs *CallList) add() {
	name := "--unknown--"
	pc, _, _, ok := runtime.Caller(1)
	if ok {
		fn := runtime.FuncForPC(pc)
		matches := callNameRegex.FindStringSubmatch(fn.Name())
		if len(matches) == 2 {
			name = matches[1]
		}
	}
	if name == "--unknown--" {
		log.Fatal("Unable to parse method call name")
	}

	*cs = append(*cs, name)
}

func (cs *CallList) reset() {
	callList = &CallList{}
}

var callList = &CallList{}

type Mock struct {
	*TransitGateway
}

func (mock *Mock) EC2() ec2iface.EC2API {
	return mock.ec2Svc
}

func (mock *Mock) RAM() ramiface.RAMAPI {
	return mock.ramSvc
}

type MockEC2 struct {
	ec2iface.EC2API
	subnetIDs              []string
	expectedInputSubnetIDs []string
}

func (mockEC2 *MockEC2) DescribeTransitGateways(input *ec2.DescribeTransitGatewaysInput) (*ec2.DescribeTransitGatewaysOutput, error) {
	callList.add()
	expectedInput := &ec2.DescribeTransitGatewaysInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("transit-gateway-id"),
				Values: []*string{aws.String(ID)},
			},
		},
	}

	if !reflect.DeepEqual(expectedInput, input) {
		return nil, fmt.Errorf("DescribeTransitGatewaysInput does not match expected")
	}

	return &ec2.DescribeTransitGatewaysOutput{
		TransitGateways: []*ec2.TransitGateway{
			{
				OwnerId:           aws.String(OwnerID),
				TransitGatewayArn: aws.String(ARN),
			},
		},
	}, nil
}

func (mockEC2 *MockEC2) DescribeSubnets(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	callList.add()

	output := &ec2.DescribeSubnetsOutput{}

	for _, subnetID := range mockEC2.subnetIDs {
		output.Subnets = append(output.Subnets, &ec2.Subnet{SubnetId: aws.String(subnetID)})
	}

	return output, nil
}

func (mockEC2 *MockEC2) DescribeTransitGatewayVpcAttachments(input *ec2.DescribeTransitGatewayVpcAttachmentsInput) (*ec2.DescribeTransitGatewayVpcAttachmentsOutput, error) {
	callList.add()
	return &ec2.DescribeTransitGatewayVpcAttachmentsOutput{
		TransitGatewayVpcAttachments: []*ec2.TransitGatewayVpcAttachment{
			{
				SubnetIds: aws.StringSlice(mockEC2.subnetIDs),
				State:     aws.String("available"),
			},
		},
	}, nil
}

func (mockEC2 *MockEC2) CreateTransitGatewayVpcAttachment(input *ec2.CreateTransitGatewayVpcAttachmentInput) (*ec2.CreateTransitGatewayVpcAttachmentOutput, error) {
	callList.add()

	expectedInput := &ec2.CreateTransitGatewayVpcAttachmentInput{
		SubnetIds:        aws.StringSlice(mockEC2.expectedInputSubnetIDs),
		TransitGatewayId: aws.String("tgw-abcdef0123456789"),
		VpcId:            aws.String("vpc-abcdef0123456789"),
		DryRun:           aws.Bool(false),
	}

	if !reflect.DeepEqual(expectedInput, input) {
		return nil, fmt.Errorf("CreateTransitGatewayVpcAttachmentInput does not match expected")
	}

	return &ec2.CreateTransitGatewayVpcAttachmentOutput{
		TransitGatewayVpcAttachment: &ec2.TransitGatewayVpcAttachment{
			State:                      aws.String("available"),
			SubnetIds:                  input.SubnetIds,
			TransitGatewayId:           input.TransitGatewayId,
			TransitGatewayAttachmentId: aws.String("tgw-attachment-id-abcdef0123456789"),
			VpcId:                      input.VpcId,
		},
	}, nil
}

type MockRAM struct {
	ramiface.RAMAPI
	associationStatus string
	invitationStatus  string
}

func (mockRAM *MockRAM) GetResourceShares(input *ram.GetResourceSharesInput) (*ram.GetResourceSharesOutput, error) {
	callList.add()
	expectedInput := &ram.GetResourceSharesInput{
		Name:          aws.String(TemplateName),
		ResourceOwner: aws.String("SELF"),
	}

	if !reflect.DeepEqual(expectedInput, input) {
		return nil, fmt.Errorf("GetResourceSharesInput does not match expected")
	}

	return &ram.GetResourceSharesOutput{
		ResourceShares: []*ram.ResourceShare{
			{
				ResourceShareArn: aws.String(ShareARN),
			},
		},
	}, nil
}

func (mockRAM *MockRAM) GetResourceShareAssociations(input *ram.GetResourceShareAssociationsInput) (*ram.GetResourceShareAssociationsOutput, error) {
	callList.add()
	expectedInput := &ram.GetResourceShareAssociationsInput{
		AssociationType:   aws.String("PRINCIPAL"),
		Principal:         aws.String(mockAccountID),
		ResourceShareArns: []*string{aws.String(ShareARN)},
	}

	if !reflect.DeepEqual(expectedInput, input) {
		return nil, fmt.Errorf("GetResourceShareAssociationsInput does not match expected")
	}

	return &ram.GetResourceShareAssociationsOutput{
		ResourceShareAssociations: []*ram.ResourceShareAssociation{
			{
				Status: aws.String(mockRAM.associationStatus),
			},
		},
	}, nil
}

func (mockRAM *MockRAM) GetResourceShareInvitations(input *ram.GetResourceShareInvitationsInput) (*ram.GetResourceShareInvitationsOutput, error) {
	callList.add()
	expectedInput := &ram.GetResourceShareInvitationsInput{
		ResourceShareArns: []*string{aws.String(ShareARN)},
	}

	// some calls have a NextToken present and some do not, so only the ResourceShareArns are checked
	if !reflect.DeepEqual(expectedInput.ResourceShareArns, input.ResourceShareArns) {
		return nil, fmt.Errorf("GetResourceShareInvitationsInput does not match expected")
	}

	output := &ram.GetResourceShareInvitationsOutput{
		ResourceShareInvitations: []*ram.ResourceShareInvitation{
			{
				ResourceShareInvitationArn: aws.String(ShareInvitationARN),
			},
		},
	}

	if mockRAM.invitationStatus != "" {
		output.ResourceShareInvitations[0].ReceiverAccountId = aws.String(mockAccountID)
		output.ResourceShareInvitations[0].Status = aws.String(mockRAM.invitationStatus)
	}

	return output, nil
}

func (mockRAM *MockRAM) AssociateResourceShare(input *ram.AssociateResourceShareInput) (*ram.AssociateResourceShareOutput, error) {
	callList.add()
	expectedInput := &ram.AssociateResourceShareInput{
		ResourceShareArn: aws.String(ShareARN),
		Principals:       []*string{aws.String(mockAccountID)},
	}

	if !reflect.DeepEqual(expectedInput, input) {
		return nil, fmt.Errorf("GetResourceShareInvitationsInput does not match expected")
	}

	mockRAM.invitationStatus = "PENDING"

	return nil, nil
}

func (mockRAM *MockRAM) AcceptResourceShareInvitation(input *ram.AcceptResourceShareInvitationInput) (*ram.AcceptResourceShareInvitationOutput, error) {
	callList.add()
	expectedInput := &ram.AcceptResourceShareInvitationInput{
		ResourceShareInvitationArn: aws.String(ShareInvitationARN),
	}

	if !reflect.DeepEqual(expectedInput, input) {
		return nil, fmt.Errorf("AcceptResourceShareInvitationInput does not match expected")
	}

	mockRAM.associationStatus = "ASSOCIATED"

	return &ram.AcceptResourceShareInvitationOutput{}, nil
}

func initMock() *Mock {
	mock := &Mock{&TransitGateway{}}
	mock.ec2Svc = &MockEC2{}
	mock.ramSvc = &MockRAM{}
	mock.Conf = &conf.Conf{DryRun: false, TGWResourceShareName: ResourceShareName}
	mock.Template = &vpcconfapi.TGWTemplate{Name: TemplateName, TransitGatewayID: ID}
	return mock
}

func TestPopulateARNs(t *testing.T) {
	callList.reset()
	mock := initMock()
	err := mock.PopulateARNs()
	if err != nil {
		t.Error(err)
	}

	if mock.ARN != ARN {
		t.Errorf("ARN expected to be %s, but got %s", ARN, mock.ARN)
	}
	if mock.ShareARN != ShareARN {
		t.Errorf("ShareARN expected to be %s, but got %s", ShareARN, mock.ShareARN)
	}
}

func shareResourceValidation(associationStatus string, expectedCalls []string) string {
	if !reflect.DeepEqual([]string(*callList), expectedCalls) {
		return fmt.Sprintf("Expected status '%s' to make API calls %#v, but got %#v", associationStatus, expectedCalls, callList)
	}

	return ""
}

func TestShareResourceNoAssociationNoInvitation(t *testing.T) {
	callList.reset()
	mock := initMock()
	mock.PopulateARNs()
	invitee := initMock()
	invitee.ramSvc = &MockRAM{invitationStatus: "PENDING"}

	err := mock.ShareResource(mockAccountID, invitee)
	if err != nil {
		t.Errorf("ShareResource should have returned with no error, but got %s", err)
	}

	mock.ramSvc = &MockRAM{associationStatus: "ASSOCIATED"}
	err = mock.WaitForAssociation(mockAccountID)
	if err != nil {
		t.Errorf("WaitForAssociation should have returned with no error, but got %s", err)
	}

	expectedCalls := []string{"DescribeTransitGateways", "GetResourceShares", "GetResourceShareInvitations",
		"AssociateResourceShare", "GetResourceShareInvitations", "AcceptResourceShareInvitation",
		"GetResourceShareAssociations"}

	errString := shareResourceValidation("[None]", expectedCalls)
	if errString != "" {
		t.Error(errString)
	}
}

func TestShareResourceNoAssociationPendingInvitation(t *testing.T) {
	callList.reset()
	mock := initMock()
	mock.PopulateARNs()
	invitee := initMock()
	invitee.ramSvc = &MockRAM{invitationStatus: "PENDING"}

	err := mock.ShareResource(mockAccountID, invitee)
	if err != nil {
		t.Errorf("ShareResource with status '%s' should have returned with no error, but got %s", "PENDING", err)
	}

	mock.ramSvc = &MockRAM{associationStatus: "ASSOCIATED"}
	err = mock.WaitForAssociation(mockAccountID)
	if err != nil {
		t.Errorf("WaitForAssociation should have returned with no error, but got %s", err)
	}

	expectedCalls := []string{"DescribeTransitGateways", "GetResourceShares", "GetResourceShareInvitations",
		"AssociateResourceShare", "GetResourceShareInvitations", "AcceptResourceShareInvitation",
		"GetResourceShareAssociations"}

	errString := shareResourceValidation("PENDING", expectedCalls)
	if errString != "" {
		t.Error(errString)
	}
}

type transitiveSubnetTest struct {
	name      string
	subnetIDs []string
}

var transitiveSubnetTests = []transitiveSubnetTest{
	{
		name:      "NoTransitiveSubnets",
		subnetIDs: []string{},
	},
	{
		name:      "OneTransitiveSubnet",
		subnetIDs: []string{"subnet-1"},
	},
	{
		name:      "FiveTransitiveSubnets",
		subnetIDs: []string{"subnet-1", "subnet-2", "subnet-3", "subnet-4", "subnet-5"},
	},
}

func TestGetTransitiveSubnets(t *testing.T) {
	for _, test := range transitiveSubnetTests {
		t.Run(test.name, func(t *testing.T) {
			callList.reset()
			mock := initMock()
			invitee := initMock()
			invitee.ec2Svc = &MockEC2{subnetIDs: test.subnetIDs}

			subnets, err := mock.GetTransitiveSubnets(invitee, "vpc-abcdef0123456789")
			if err != nil {
				t.Errorf("Did not expect an error, but got %s", err)
			}

			if len(subnets) != len(test.subnetIDs) {
				t.Errorf("Expected %d subnet(s) but got %d", len(test.subnetIDs), len(subnets))
			}

			expectedCalls := []string{"DescribeSubnets"}

			errString := shareResourceValidation(test.name, expectedCalls)
			if errString != "" {
				t.Error(errString)
			}
		})
	}
}

type attachSubnetTest struct {
	name                 string
	vpcSubnetIDs         []string
	tgwAttachedSubnetIDs []string
	expectedSubnetIDs    []string
	expectedCalls        []string
}

var attachSubnetTests = []attachSubnetTest{
	{
		name:                 "AllSubnetsAttached",
		vpcSubnetIDs:         []string{"subnet-1", "subnet-2", "subnet-3", "subnet-4", "subnet-5"},
		tgwAttachedSubnetIDs: []string{"subnet-1", "subnet-2", "subnet-3", "subnet-4", "subnet-5"},
		expectedSubnetIDs:    []string{},
		expectedCalls:        []string{"DescribeTransitGatewayVpcAttachments"},
	},
	{
		name:                 "SomeSubnetsAttached",
		vpcSubnetIDs:         []string{"subnet-1", "subnet-2", "subnet-3", "subnet-4", "subnet-5"},
		tgwAttachedSubnetIDs: []string{"subnet-1", "subnet-2", "subnet-3"},
		expectedSubnetIDs:    []string{"subnet-4", "subnet-5"},
		expectedCalls:        []string{"DescribeTransitGatewayVpcAttachments", "CreateTransitGatewayVpcAttachment", "DescribeTransitGatewayVpcAttachments"},
	},
	{
		name:                 "NoSubnetsAttached",
		vpcSubnetIDs:         []string{"subnet-1", "subnet-2", "subnet-3", "subnet-4", "subnet-5"},
		tgwAttachedSubnetIDs: []string{},
		expectedSubnetIDs:    []string{"subnet-1", "subnet-2", "subnet-3", "subnet-4", "subnet-5"},
		expectedCalls:        []string{"DescribeTransitGatewayVpcAttachments", "CreateTransitGatewayVpcAttachment", "DescribeTransitGatewayVpcAttachments"},
	},
}

func TestAttachSubnetsToTGW(t *testing.T) {
	for _, test := range attachSubnetTests {
		t.Run(test.name, func(t *testing.T) {
			callList.reset()
			mock := initMock()
			invitee := initMock()
			invitee.ec2Svc = &MockEC2{subnetIDs: test.tgwAttachedSubnetIDs, expectedInputSubnetIDs: test.expectedSubnetIDs}

			subnetsAttached, err := mock.AttachSubnetsToTGW(invitee, "9876543210987", "vpc-abcdef0123456789", test.vpcSubnetIDs)
			if err != nil {
				t.Errorf("Didn't expect an error but got %s", err)
			}

			errString := shareResourceValidation("TestAttachSubnetsToTGW", test.expectedCalls)
			if errString != "" {
				t.Error(errString)
			}

			if !reflect.DeepEqual(subnetsAttached, test.expectedSubnetIDs) {
				t.Errorf("Expected subnets %q to be newly attached, but got %q", test.expectedSubnetIDs, subnetsAttached)
			}
		})
	}
}
