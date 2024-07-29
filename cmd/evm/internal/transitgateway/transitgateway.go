package transitgateway

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cmd/evm/internal/cloudtamer"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cmd/evm/internal/conf"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/vpcconfapi"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/waiter"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/ram"
	"github.com/aws/aws-sdk-go/service/ram/ramiface"
)

var ErroNoAssociations = errors.New("No matching associations")

// TransitGateway core
type TransitGateway struct {
	AccountID  string
	Conf       *conf.Conf
	Template   *vpcconfapi.TGWTemplate
	ARN        string
	ShareARN   string
	ec2Svc     ec2iface.EC2API
	ramSvc     ramiface.RAMAPI
	session    *session.Session
	CloudTamer *cloudtamer.CloudTamer
}

// TGWAPI interface
type TGWAPI interface {
	EC2() ec2iface.EC2API
	RAM() ramiface.RAMAPI
	Session() *session.Session
	PopulateARNs() error
	ResourceShareARN() (string, error)
	ShareResource(accountID string, invitee TGWAPI) error
	GetAssociationStatus(accountID string) (string, error)
	GetTransitiveSubnets(invitee TGWAPI, vpcID string) ([]string, error)
	AttachSubnetsToTGW(invitee TGWAPI, accountID string, vpcID string, subnets []string) ([]string, error)
	WaitForAssociation(accountID string) error
}

// EC2 returns an active instance for the session
func (tgw *TransitGateway) EC2() ec2iface.EC2API {
	if tgw.ec2Svc == nil {
		tgw.ec2Svc = ec2.New(tgw.Session())
	}

	return tgw.ec2Svc
}

// RAM returns an active instance for the session
func (tgw *TransitGateway) RAM() ramiface.RAMAPI {
	if tgw.ramSvc == nil {
		tgw.ramSvc = ram.New(tgw.Session())
	}

	return tgw.ramSvc
}

// Session fetches, stores, and returns an active AWS session or exits on failure
func (tgw *TransitGateway) Session() *session.Session {
	if tgw.session == nil {
		var err error
		tgw.session, err = tgw.CloudTamer.GetAWSSessionForAccountID(tgw.AccountID)
		if err != nil {
			log.Fatalf("Cannot continue, unable to get AWS Session for Transit Gateway account: %s", tgw.AccountID)
		}
	}

	return tgw.session
}

// PopulateARNs fetches the ARNs
func (tgw *TransitGateway) PopulateARNs() error {
	if tgw.ARN == "" {
		input := &ec2.DescribeTransitGatewaysInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("transit-gateway-id"),
					Values: []*string{aws.String(tgw.Template.TransitGatewayID)},
				},
			},
		}

		result, err := tgw.EC2().DescribeTransitGateways(input)
		if err != nil {
			return fmt.Errorf("Failed to get transit gateway ARN: %s", err)
		}
		if len(result.TransitGateways) == 1 {
			tgw.AccountID = aws.StringValue(result.TransitGateways[0].OwnerId) // remove this after vpc-conf update?
			tgw.ARN = aws.StringValue(result.TransitGateways[0].TransitGatewayArn)
		} else {
			return fmt.Errorf("AWS returned multiple transit gateway descriptions (%d) but the filter requested only: '%s'", len(result.TransitGateways), tgw.Template.TransitGatewayID)
		}
	}
	_, err := tgw.ResourceShareARN()

	return err
}

// ResourceShareARN fetches, stores, and returns the ResourceShareARN for the Transit Gateway
func (tgw *TransitGateway) ResourceShareARN() (string, error) {
	if tgw.ShareARN == "" {
		input := ram.GetResourceSharesInput{
			Name:          aws.String(tgw.Conf.TGWResourceShareName),
			ResourceOwner: aws.String("SELF"),
		}

		result, err := tgw.RAM().GetResourceShares(&input)
		if err != nil {
			return "", fmt.Errorf("Failed to get resource share ARN: %s", err)
		}

		if len(result.ResourceShares) == 1 {
			tgw.ShareARN = aws.StringValue(result.ResourceShares[0].ResourceShareArn)
		} else {
			return "", fmt.Errorf("AWS returned multiple resource shares (%d) but the filter requested only: '%s'", len(result.ResourceShares), tgw.Template.Name)
		}
	}

	return tgw.ShareARN, nil
}

// ShareResource shares the TransitGateway with the given account ID
func (tgw *TransitGateway) ShareResource(accountID string, invitee TGWAPI) error {
	invitationARN, err := tgw.getInvitationARN(accountID)
	if err != nil {
		return err
	}

	resourceShareARN, err := tgw.ResourceShareARN()
	if err != nil {
		return fmt.Errorf("Failed to get resource share ARN: %s", err)
	}

	if tgw.Conf.DryRun { // there is no dry run flag for the share invite
		log.Printf("DRY RUN abort on ShareResource for accountID: %s", accountID)
		return nil
	}

	if invitationARN == "" { // only send an invite if there isn't one pending
		_, err = tgw.RAM().AssociateResourceShare(&ram.AssociateResourceShareInput{
			ResourceShareArn: aws.String(resourceShareARN),
			Principals:       []*string{&accountID},
		})
		if err != nil {
			return fmt.Errorf("Error associating principal: %s", err)
		}
	}

	w := waiter.NewDefaultWaiter()

	err = w.Wait(func() waiter.Result {
		inviteOut, err := invitee.RAM().GetResourceShareInvitations(&ram.GetResourceShareInvitationsInput{
			ResourceShareArns: []*string{&resourceShareARN},
		})
		if err != nil {
			return waiter.Error(fmt.Errorf("Error fetching share invitation %q: %s", resourceShareARN, err))
		}

		for _, invite := range inviteOut.ResourceShareInvitations {
			if aws.StringValue(invite.Status) != "PENDING" {
				continue
			}
			log.Printf("Accepting invitation %s", *invite.ResourceShareInvitationArn)
			_, err = invitee.RAM().AcceptResourceShareInvitation(&ram.AcceptResourceShareInvitationInput{
				ResourceShareInvitationArn: invite.ResourceShareInvitationArn,
			})
			if err != nil {
				return waiter.Error(fmt.Errorf("Error accepting resource share %q: %s", aws.StringValue(invite.ResourceShareInvitationArn), err))
			}
			return waiter.Done()
		}

		return waiter.Continue(fmt.Sprintf("Waiting to accept invitation for %s", resourceShareARN))
	})

	return err
}

// WaitForAssociation waits up tp 5 minutes
func (tgw *TransitGateway) WaitForAssociation(accountID string) error {
	w := &waiter.Waiter{SleepDuration: time.Second * 1, StatusInterval: time.Second * 10, Timeout: time.Minute * 5}

	return w.Wait(func() waiter.Result {
		status, err := tgw.GetAssociationStatus(accountID)
		if err != nil {
			return waiter.Error(err)
		}
		if status == "ASSOCIATED" {
			return waiter.Done()
		}

		return waiter.Continue(fmt.Sprintf("Waiting on association - status: %s", status))
	})
}

// GetAssociationStatus fetches the association status
func (tgw *TransitGateway) GetAssociationStatus(accountID string) (string, error) {
	shareARN, err := tgw.ResourceShareARN()
	if err != nil {
		return "", fmt.Errorf("Failed to get resource share ARN: %s", err)
	}

	result, err := tgw.RAM().GetResourceShareAssociations(&ram.GetResourceShareAssociationsInput{
		AssociationType:   aws.String("PRINCIPAL"),
		Principal:         aws.String(accountID),
		ResourceShareArns: []*string{aws.String(shareARN)},
	})

	if err != nil {
		return "", fmt.Errorf("Unexpected response from AWS for GetResourceShareAssociations for %s - %s", shareARN, err)
	} else if len(result.ResourceShareAssociations) == 1 {
		return aws.StringValue(result.ResourceShareAssociations[0].Status), nil
	} else if len(result.ResourceShareAssociations) > 1 {
		return "", fmt.Errorf("GetResourceShareAssociations unexpectedly returned more than one resource")
	}

	return "", ErroNoAssociations
}

func (tgw *TransitGateway) getInvitationARN(accountID string) (string, error) {
	resourceShareARN, err := tgw.ResourceShareARN()
	if err != nil {
		return "", fmt.Errorf("Failed to get resource share ARN: %s", err)
	}

	invitationARN := ""
	var nextToken *string

	for {
		input := &ram.GetResourceShareInvitationsInput{
			ResourceShareArns: []*string{aws.String(resourceShareARN)},
			NextToken:         nextToken,
		}
		output, err := tgw.RAM().GetResourceShareInvitations(input)
		if err != nil {
			return "", fmt.Errorf("Unexpected response from AWS for GetResourceShareInvitations for %s - %s", resourceShareARN, err)
		}

		for _, invite := range output.ResourceShareInvitations {
			if accountID == aws.StringValue(invite.ReceiverAccountId) {
				invitationARN = aws.StringValue(invite.ResourceShareInvitationArn)
				break
			}
		}

		nextToken = output.NextToken
		if aws.StringValue(nextToken) == "" {
			break
		}
	}

	return invitationARN, nil
}

// GetTransitiveSubnets returns any subnets for the invitee that are tagged as transitive
func (tgw *TransitGateway) GetTransitiveSubnets(invitee TGWAPI, vpcID string) ([]string, error) {
	subnets := []string{}

	output, err := invitee.EC2().DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: aws.StringSlice([]string{vpcID}),
			},
			{
				Name:   aws.String("tag:vpc-conf-layer"),
				Values: aws.StringSlice([]string{"transitive"}),
			},
		},
	})
	if err != nil {
		return subnets, err
	}

	for _, subnet := range output.Subnets {
		subnets = append(subnets, aws.StringValue(subnet.SubnetId))
	}

	return subnets, nil
}

func stringInSlice(str string, sl []string) bool {
	for _, s := range sl {
		if s == str {
			return true
		}
	}
	return false
}

func (tgw *TransitGateway) AttachSubnetsToTGW(invitee TGWAPI, accountID string, vpcID string, subnets []string) ([]string, error) {
	if tgw.Conf.DryRun {
		log.Println("DRY RUN ABORT for AttachSubnetsToTGW")
		return []string{}, nil
	}

	vpc, err := invitee.EC2().DescribeTransitGatewayVpcAttachments(&ec2.DescribeTransitGatewayVpcAttachmentsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: aws.StringSlice([]string{vpcID}),
			},
			{
				Name:   aws.String("transit-gateway-id"),
				Values: aws.StringSlice([]string{tgw.Template.TransitGatewayID}),
			},
		},
	})

	if err != nil {
		return []string{}, err
	}

	subnetsToAttach := []string{}

	if len(vpc.TransitGatewayVpcAttachments) > 0 {
		attachedSubnets := aws.StringValueSlice(vpc.TransitGatewayVpcAttachments[0].SubnetIds)

		for _, subnet := range subnets {
			if !stringInSlice(subnet, attachedSubnets) {
				subnetsToAttach = append(subnetsToAttach, subnet)
			}
		}
		if len(subnetsToAttach) == 0 {
			return subnetsToAttach, nil
		}
	} else {
		subnetsToAttach = subnets
	}

	output, err := invitee.EC2().CreateTransitGatewayVpcAttachment(&ec2.CreateTransitGatewayVpcAttachmentInput{
		SubnetIds:        aws.StringSlice(subnetsToAttach),
		TransitGatewayId: aws.String(tgw.Template.TransitGatewayID),
		VpcId:            aws.String(vpcID),
		DryRun:           aws.Bool(tgw.Conf.DryRun),
	})
	if err != nil {
		return []string{}, err
	}

	input := ec2.DescribeTransitGatewayVpcAttachmentsInput{
		TransitGatewayAttachmentIds: []*string{output.TransitGatewayVpcAttachment.TransitGatewayAttachmentId},
	}

	w := &waiter.Waiter{SleepDuration: time.Second * 5, StatusInterval: time.Second * 10, Timeout: time.Minute * 5}

	err = w.Wait(func() waiter.Result {
		output, err := invitee.EC2().DescribeTransitGatewayVpcAttachments(&input)
		if err != nil {
			return waiter.Error(err)
		}
		if len(output.TransitGatewayVpcAttachments) == 1 && aws.StringValue(output.TransitGatewayVpcAttachments[0].State) == "available" {
			return waiter.Done()
		}
		return waiter.Continue("")
	})

	if err != nil {
		return []string{}, err
	}

	return subnetsToAttach, nil
}
