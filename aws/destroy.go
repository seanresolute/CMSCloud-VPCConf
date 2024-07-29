package aws

import (
	"fmt"
	"strings"
	"time"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/waiter"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/networkfirewall"
	"github.com/aws/aws-sdk-go/service/route53resolver"
)

func (ctx *Context) DeleteAllSecurityGroups() error {
	out, err := ctx.EC2().DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{&ctx.VPCID},
			},
		},
	})
	if err != nil {
		return err
	}

	for _, sg := range out.SecurityGroups {
		// Delete any security group whose name does not match "default"
		// https://aws.amazon.com/premiumsupport/knowledge-center/troubleshoot-delete-vpc-sg/
		if aws.StringValue(sg.GroupName) == "default" {
			ctx.Logger.Log("Not deleting default security group %s", *sg.GroupId)
		} else {
			err := ctx.DeleteSecurityGroup(*sg.GroupId)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (ctx *Context) DeleteAllRouteTables() error {
	out, err := ctx.EC2().DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{&ctx.VPCID},
			},
		},
	})
	if err != nil {
		return err
	}

	for _, rt := range out.RouteTables {
		isMain := false
		for _, assoc := range rt.Associations {
			if *assoc.Main {
				isMain = true
			}
		}
		if isMain {
			ctx.Logger.Log("Not deleting main route table %s", *rt.RouteTableId)
			continue
		}
		for _, assoc := range rt.Associations {
			err := ctx.DisassociateRouteTable(*assoc.RouteTableAssociationId)
			if err != nil {
				return err
			}
		}
		err := ctx.DeleteRouteTable(*rt.RouteTableId)
		if err != nil {
			return err
		}
	}

	return nil
}

func (ctx *Context) DeleteAllNATGateways() error {
	out, err := ctx.EC2().DescribeNatGateways(&ec2.DescribeNatGatewaysInput{
		Filter: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{&ctx.VPCID},
			},
			{
				Name:   aws.String("state"),
				Values: []*string{aws.String("pending"), aws.String("available")},
			},
		},
	})
	if err != nil {
		return err
	}

	for _, ng := range out.NatGateways {
		if *ng.State != "deleted" && *ng.State != "deleting" {
			err := ctx.DeleteNATGateway(*ng.NatGatewayId)
			if err != nil {
				return err
			}
		}
		for _, addr := range ng.NatGatewayAddresses {
			if addr.AllocationId != nil {
				err := ctx.ReleaseEIP(*addr.AllocationId)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (ctx *Context) DeleteAllInternetGateways() error {
	out, err := ctx.EC2().DescribeInternetGateways(&ec2.DescribeInternetGatewaysInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("attachment.vpc-id"),
				Values: []*string{&ctx.VPCID},
			},
		},
	})
	if err != nil {
		return err
	}

	for _, ng := range out.InternetGateways {
		err := ctx.DetachAndDeleteInternetGateway(*ng.InternetGatewayId)
		if err != nil {
			return err
		}
	}

	return nil
}

func (ctx *Context) DeleteAllTransitGatewayVPCAttachments() error {
	out, err := ctx.EC2().DescribeTransitGatewayVpcAttachments(&ec2.DescribeTransitGatewayVpcAttachmentsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{&ctx.VPCID},
			},
		},
	})
	if err != nil {
		return err
	}

	for _, tgva := range out.TransitGatewayVpcAttachments {
		err := ctx.DeleteTransitGatewayVPCAttachment(*tgva.TransitGatewayAttachmentId)
		if err != nil {
			return err
		}
	}

	return nil
}

func (ctx *Context) DeleteAllPeeringConnections() error {
	out, err := ctx.EC2().DescribeVpcPeeringConnections(&ec2.DescribeVpcPeeringConnectionsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("requester-vpc-info.vpc-id"),
				Values: []*string{&ctx.VPCID},
			},
		},
	})
	if err != nil {
		return err
	}
	out2, err := ctx.EC2().DescribeVpcPeeringConnections(&ec2.DescribeVpcPeeringConnectionsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("accepter-vpc-info.vpc-id"),
				Values: []*string{&ctx.VPCID},
			},
		},
	})
	if err != nil {
		return err
	}

	for _, pc := range append(out.VpcPeeringConnections, out2.VpcPeeringConnections...) {
		ctx.Log("Deleting peering connection %s", aws.StringValue(pc.VpcPeeringConnectionId))
		_, err := ctx.EC2().DeleteVpcPeeringConnection(&ec2.DeleteVpcPeeringConnectionInput{
			VpcPeeringConnectionId: pc.VpcPeeringConnectionId,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (ctx *Context) DeleteAllResolverRuleQueryLogs() error {
	out, err := ctx.R53R().ListResolverQueryLogConfigAssociations(&route53resolver.ListResolverQueryLogConfigAssociationsInput{
		Filters: []*route53resolver.Filter{
			{
				Name:   aws.String("ResourceId"),
				Values: []*string{&ctx.VPCID},
			},
		},
	})
	if err != nil {
		return err
	}

	// Delete associations with this VPC, regardless of origin
	configs := make([]string, 0)
	for _, qlca := range out.ResolverQueryLogConfigAssociations {
		ctx.Log("Deleting query log config association %s", aws.StringValue(qlca.Id))
		configs = append(configs, aws.StringValue(qlca.ResolverQueryLogConfigId))
		_, err := ctx.R53R().DisassociateResolverQueryLogConfig(&route53resolver.DisassociateResolverQueryLogConfigInput{
			ResourceId:               &ctx.VPCID,
			ResolverQueryLogConfigId: qlca.ResolverQueryLogConfigId,
		})
		if err != nil {
			return err
		}
		// Use a bare string for the "CREATED" state, as the actual value returned from the API is "CREATED",
		// not "ACTIVE" as route53resolver.ResolverQueryLogConfigAssociationStatusActive exposes
		err = ctx.WaitForQueryLogConfigAssociationStatus(aws.StringValue(qlca.Id), database.WaitForMissingAWSStatus, "CREATED", route53resolver.ResolverQueryLogConfigAssociationStatusDeleting)
		if err != nil {
			return err
		}
	}

	for _, config := range configs {
		out, err := ctx.R53R().ListResolverQueryLogConfigAssociations(&route53resolver.ListResolverQueryLogConfigAssociationsInput{
			Filters: []*route53resolver.Filter{
				{
					Name:   aws.String("ResolverQueryLogConfigId"),
					Values: []*string{&config},
				},
			},
		})
		if err != nil {
			return err
		}
		associationsStillInUse := 0
		for _, a := range out.ResolverQueryLogConfigAssociations {
			// Use "CREATED" bare due to lack of correct const in route53resolver
			if stringInSlice(aws.StringValue(a.Status), []string{"CREATED", route53resolver.ResolverQueryLogConfigAssociationStatusCreating}) {
				associationsStillInUse = associationsStillInUse + 1
			}
		}
		if associationsStillInUse > 0 {
			ctx.Log("Query Log config %s still associated with other resources (%d), won't delete", config, associationsStillInUse)
		} else {
			out, err := ctx.R53R().GetResolverQueryLogConfig(&route53resolver.GetResolverQueryLogConfigInput{
				ResolverQueryLogConfigId: aws.String(config),
			})
			if err != nil {
				return err
			}
			if aws.StringValue(out.ResolverQueryLogConfig.Name) == ctx.QueryLogConfigName() {
				tagOut, err := ctx.R53R().ListTagsForResource(&route53resolver.ListTagsForResourceInput{
					ResourceArn: out.ResolverQueryLogConfig.Arn,
				})
				if err != nil {
					return err
				}
				isAutomated := false
				for _, tag := range tagOut.Tags {
					if aws.StringValue(tag.Key) == "Automated" && strings.ToLower(aws.StringValue(tag.Value)) == "true" {
						isAutomated = true
					}
				}
				if isAutomated {
					ctx.Log("Deleting query log configuration %s", config)
					_, err = ctx.R53R().DeleteResolverQueryLogConfig(&route53resolver.DeleteResolverQueryLogConfigInput{
						ResolverQueryLogConfigId: &config,
					})
					if err != nil {
						return err
					}
				} else {
					ctx.Log("Found automatable query log configuration, but it is not automated; preserving")
				}
			} else {
				ctx.Log("Leaving (potentially unmanaged) query log configuration %s", config)
			}
		}
	}
	return nil
}

// delete firewall and default policy
func (ctx *Context) DeleteFirewallResources() error {
	w := &waiter.Waiter{
		SleepDuration:  time.Second * 5,
		StatusInterval: time.Second * 60,
		Timeout:        time.Minute * 10,
		Logger:         ctx.Logger,
	}

	// firewall
	// AWS won't delete a firewall with an active logging configuration
	_, err := ctx.NetworkFirewall().UpdateLoggingConfiguration(&networkfirewall.UpdateLoggingConfigurationInput{
		FirewallName: aws.String(ctx.FirewallName()),
		LoggingConfiguration: &networkfirewall.LoggingConfiguration{
			LogDestinationConfigs: []*networkfirewall.LogDestinationConfig{},
		},
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == networkfirewall.ErrCodeResourceNotFoundException {
				// no-op, firewall was already deleted
			} else if aerr.Code() == networkfirewall.ErrCodeInvalidRequestException && aerr.Message() == NoLoggingConfigChanges {
				// no-op, logging config was already cleared
			} else {
				return fmt.Errorf("Error updating network firewall logging configuration: %s", err)
			}
		} else {
			return fmt.Errorf("Error updating network firewall logging configuration: %s", err)
		}
	}

	_, err = ctx.NetworkFirewall().DeleteFirewall(&networkfirewall.DeleteFirewallInput{
		FirewallName: aws.String(ctx.FirewallName()),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == networkfirewall.ErrCodeResourceNotFoundException {
				ctx.Log("Network firewall not found")
			} else {
				return fmt.Errorf("Error deleting network firewall: %s", err)
			}
		} else {
			return fmt.Errorf("Error deleting network firewall: %s", err)
		}
	} else {
		ctx.Log("Deleting network firewall %s", ctx.FirewallName())
	}

	// default firewall policy
	arn, err := ctx.getDefaultFirewallPolicyARN()
	if err != nil {
		return fmt.Errorf("Error checking for existence of default firewall policy: %s", err)
	}
	if arn != nil {
		// we wait here and explicitly check for a dependency exception since AWS doesn't reliably report the deletion of the firewall
		err = w.Wait(func() waiter.Result {
			_, err = ctx.NetworkFirewall().DeleteFirewallPolicy(&networkfirewall.DeleteFirewallPolicyInput{
				FirewallPolicyArn: arn,
			})
			if err != nil {
				if aerr, ok := err.(awserr.Error); ok {
					if aerr.Code() == networkfirewall.ErrCodeResourceNotFoundException {
						// this shouldn't be possible since we just checked if it existed
						return waiter.Done()
					} else if aerr.Code() == networkfirewall.ErrCodeInvalidOperationException {
						// dependency exception: until firewall deletion is complete, it's still using the default policy
						return waiter.Continue("Waiting for deletion of network firewall to be complete")
					} else {
						return waiter.Error(fmt.Errorf("Error deleting default firewall policy: %s", err))
					}
				} else {
					return waiter.Error(fmt.Errorf("Error deleting default firewall policy: %s", err))
				}
			}
			return waiter.DoneWithMessage("Deleting default firewall policy")
		})
		if err != nil {
			return fmt.Errorf("Error waiting to delete default firewall policy: %s", err)
		}
	}

	return nil
}
