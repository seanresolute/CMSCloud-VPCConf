package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	evmct "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cmd/evm/internal/cloudtamer"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cmd/evm/internal/conf"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cmd/evm/internal/transitgateway"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/vpcconfapi"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
)

const (
	exitCodeSuccess = iota
	exitCodeConfigurationError
	exitCodeConversionFailure
	exitCodeIOFailure
	exitCodeVPCConfAPI
)

var (
	region = "us-east-1"
)

type exceptionVPC struct {
	vpc             vpcconfapi.VPC
	TGWIsAssociated bool
	routeTables     []routeTable
}

type routeTable struct {
	id           string
	routesNeeded []string
}

func (r *routeTable) indexOf(cidr string) int {
	for i, rn := range r.routesNeeded {
		if rn == cidr {
			return i
		}
	}
	return -1
}

func (r *routeTable) removeAt(i int) error {
	if i < 0 || i > len(r.routesNeeded)-1 {
		return fmt.Errorf("Index out of bounds %d", i)
	}

	r.routesNeeded = append(r.routesNeeded[:i], r.routesNeeded[i+1:]...)

	return nil
}

func isPrefixListID(dest string) bool {
	return strings.HasPrefix(dest, "pl-")
}

func filterVPCsByStack(vpcs []vpcconfapi.VPC, conf *conf.Conf) []vpcconfapi.VPC {
	filtered := []vpcconfapi.VPC{}

	for _, vpc := range vpcs {
		if conf.AllowedStack(strings.ToLower(vpc.Stack)) {
			filtered = append(filtered, vpc)
		}
	}

	return filtered
}

func inspectRouteTables(ec2Svc ec2iface.EC2API, vpcID string, transitGatewayID string, tgwTemplateRoutes []string) ([]routeTable, []error) {
	tables := []routeTable{}
	tableErrors := []error{}

	input := &ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(vpcID)},
			},
		},
	}
	output, err := ec2Svc.DescribeRouteTables(input)
	if err != nil {
		tableErrors = append(tableErrors, fmt.Errorf("Failed to get route tables for %s: %s - skipping", vpcID, err))
		return tables, tableErrors
	}

	for _, rt := range output.RouteTables {
		table := routeTable{id: *rt.RouteTableId}
		table.routesNeeded = make([]string, len(tgwTemplateRoutes))
		copy(table.routesNeeded, tgwTemplateRoutes)
		errs := []error{}

		for _, route := range rt.Routes {
			destination := aws.StringValue(route.DestinationPrefixListId)
			destIdx := table.indexOf(destination)
			// only error if we find a prefix list from the template that has a target other than the new TGW
			if destIdx > -1 {
				if aws.StringValue(route.TransitGatewayId) == transitGatewayID {
					err := table.removeAt(destIdx)
					if err != nil { // cannot continue at this point because the needed routes table state is invalid
						tableErrors = append(tableErrors, fmt.Errorf("Unable to remove entry at index %d - %s", destIdx, err))
						return tables, tableErrors
					}
				} else {
					errs = append(errs, fmt.Errorf("Prefix list %s on route table %s matches the TGW template but has a different target than the new TGW",
						aws.StringValue(route.DestinationPrefixListId), aws.StringValue(rt.RouteTableId)))
				}
			}
		}
		// only modify routes if there are no errors that require manual intervention
		if len(errs) == 0 && len(table.routesNeeded) > 0 {
			tables = append(tables, table)
		} else if len(errs) > 0 {
			tableErrors = append(tableErrors, errs...)
		}
	}

	return tables, tableErrors
}

func createRoutes(ec2Svc ec2iface.EC2API, eVPC exceptionVPC, transitGatewayID string, dryRun bool) error {
	for _, routeTable := range eVPC.routeTables {
		for _, destination := range routeTable.routesNeeded {
			if !isPrefixListID(destination) {
				return fmt.Errorf("A CIDR route %s was passed to CreateRoute when only prefix list routes were expected", destination)
			}

			input := ec2.CreateRouteInput{
				DestinationPrefixListId: &destination,
				DryRun:                  &dryRun,
				TransitGatewayId:        &transitGatewayID,
				RouteTableId:            &routeTable.id,
			}

			log.Printf("Creating route for %s: %s -> %s on %s", eVPC.vpc.ID, destination, transitGatewayID, routeTable.id)

			output, err := ec2Svc.CreateRoute(&input)
			if err != nil {
				return fmt.Errorf("Failed to create route for %s: %s -> %s on %s - %s", eVPC.vpc.ID, destination, transitGatewayID, routeTable.id, err)
			}
			if !aws.BoolValue(output.Return) {
				return fmt.Errorf("CreateRoute didn't return an error but still has false for success for %s: %s -> %s on %s - %s", eVPC.vpc.ID, destination, transitGatewayID, routeTable.id, err)
			}
		}
	}

	return nil
}

func main() {
	var err error

	start := time.Now()
	log.Println("Exceptional VPC Manager")

	log.Println("Load environment variables...")
	conf := &conf.Conf{}
	if !conf.Load() {
		conf.LogProblems()
		os.Exit(exitCodeConfigurationError)
	}

	vpcConfAPI := &vpcconfapi.VPCConfAPI{Username: conf.Username, Password: conf.Password, BaseURL: conf.VPCConfBaseURL}
	tgwTemplate, err := vpcConfAPI.GetTGWTemplate(conf.VPCConfTemplateName)
	if err != nil {
		log.Println(err)
		os.Exit(exitCodeVPCConfAPI)
	}

	vpcs, err := vpcConfAPI.GetExceptionVPCs()
	if err != nil {
		log.Println(err)
		os.Exit(exitCodeVPCConfAPI)
	}

	if len(vpcs) == 0 {
		log.Printf("No exception VPCs retrieved from VPC Conf.")
		os.Exit(exitCodeVPCConfAPI) // generate an error so it can be investigated
	}

	vpcs = filterVPCsByStack(vpcs, conf)
	if len(vpcs) == 0 {
		log.Printf("No exception VPCs found for stack(s): %s", strings.Join(conf.Stacks, ", "))
		os.Exit(exitCodeSuccess)
	}

	log.Printf("Managing %d VPCs for stacks: %s", len(vpcs), strings.Join(conf.Stacks, ", "))

	log.Printf("Authenticate as %s at %s", conf.Username, conf.CloudTamerBaseURL)
	ct := &evmct.CloudTamer{Config: conf, Region: region}
	ct.ValidateToken()

	log.Printf("Get details for transit gateway from AWS")
	// TODO: update vpc-conf to provide the accountID and region for the TGWs in mtgas.json?
	tgw := &transitgateway.TransitGateway{Conf: conf, Template: tgwTemplate, AccountID: conf.TransitGatewayAccountID, CloudTamer: ct}
	err = tgw.PopulateARNs()
	if err != nil {
		log.Printf("Failed to fetch Transit Gateway details: %s", err)
		os.Exit(exitCodeIOFailure)
	}

	log.Printf("Transit Gateway Target: %s - %s", tgwTemplate.Name, tgwTemplate.TransitGatewayID)
	log.Printf("Transit Gateway ARN: %s", tgw.ARN)
	log.Printf("Transit Gateway Share ARN: %s", tgw.ShareARN)

	log.Printf("Dry Run: %t", conf.DryRun)

	for i, vpc := range vpcs {
		log.Printf("Inspect attachment %02d/%d %s", i+1, len(vpcs), vpc)

		vpcSession, err := ct.GetAWSSessionForAccountID(vpc.AccountID)
		if err != nil {
			log.Printf("Failed to get session for account %s: %s - skipping", vpc.AccountID, err)
			continue
		}
		ec2Svc := ec2.New(vpcSession)

		exceptionVPC := &exceptionVPC{vpc: vpc, TGWIsAssociated: false}

		associationStatus, err := tgw.GetAssociationStatus(vpc.AccountID)
		if err != nil && err != transitgateway.ErroNoAssociations {
			log.Printf("Unable to get the TGW association status for %s - skipping", vpc)
			continue
		}

		if associationStatus == "ASSOCIATED" {
			exceptionVPC.TGWIsAssociated = true
		}

		if !exceptionVPC.TGWIsAssociated {
			log.Printf("Transit gateway is not assocaited with %s", vpc)
		}

		var errs []error

		exceptionVPC.routeTables, errs = inspectRouteTables(ec2Svc, vpc.ID, tgwTemplate.TransitGatewayID, append([]string(nil), tgwTemplate.Routes...))

		if len(errs) > 0 {
			for _, e := range errs {
				log.Println(e)
			}
			continue
		}

		invitee := &transitgateway.TransitGateway{AccountID: vpc.AccountID, Conf: conf, CloudTamer: ct, Template: tgwTemplate}

		if !exceptionVPC.TGWIsAssociated {
			err = tgw.ShareResource(exceptionVPC.vpc.AccountID, invitee)
			if err != nil {
				log.Println(err)
				continue
			}
		}

		subnets, err := tgw.GetTransitiveSubnets(invitee, vpc.ID)
		if err != nil {
			log.Println(err)
			continue
		}
		if len(subnets) == 0 {
			log.Printf("%s/%s has 0 subnets labeled as transitive - unable to continue", vpc.AccountID, vpc.ID)
			continue
		}

		log.Printf("%s has %d transitive subnets", vpc.ID, len(subnets))

		attachedSubnets, err := tgw.AttachSubnetsToTGW(invitee, vpc.AccountID, vpc.ID, subnets)
		if err != nil {
			log.Println(err)
			continue
		}

		log.Printf("%s attached %d additional transitive subnets", vpc.ID, len(attachedSubnets))

		if len(exceptionVPC.routeTables) > 0 {
			err := createRoutes(ec2Svc, *exceptionVPC, tgwTemplate.TransitGatewayID, conf.DryRun)
			if err != nil {
				log.Println(err)
			}
		}
	}

	log.Printf("Total execution time: %v", time.Since(start).Round(time.Second))
}
