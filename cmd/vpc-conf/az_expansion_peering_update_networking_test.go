package main

import (
	"encoding/json"
	"fmt"
	"testing"

	awsp "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/aws"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/testhelpers"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/testmocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/networkfirewall"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type AZExpansionPeeringUpdateNetworking struct {
	TestCaseName string

	VPCName string
	Stack   string
	VPCID   string
	Region  database.Region

	StartState                       database.VPCState
	ExistingContainers               testmocks.ContainerTree
	ExistingPeeringConnections       []*ec2.VpcPeeringConnection
	ExistingSubnetCIDRs              map[string]string
	ExistingVPCCIDRBlocks            []*ec2.VpcCidrBlockAssociation // only used when there are unroutable subnets
	ExistingRouteTables              []*ec2.RouteTable
	ExistingFirewalls                map[string]string // firewall id -> vpc id
	ExistingFirewallSubnetToEndpoint map[string]string // subnet id -> endpoint id
	ExistingFirewallPolicies         []*networkfirewall.FirewallPolicyMetadata

	TaskConfig database.UpdateNetworkingTaskData

	ExpectedTaskStatus database.TaskStatus
	ExpectedEndState   database.VPCState
}

func TestPerformAZExpansionPeeringUpdateNetworking(t *testing.T) {
	ExistingSubnetCIDRs := map[string]string{
		"subnet-04a6fef5eb22e49c1": "10.147.105.64/27",
		"subnet-0fe27ec34f6e076da": "10.147.51.0/27",
		"subnet-0d0dca144386166d7": "10.147.51.32/27",
		"subnet-071c6fba1ab18c7b1": "10.147.51.96/27",
		"subnet-0aa4c424af4e58114": "10.147.51.64/27",
		"subnet-0649e25ce78faf4ad": "10.147.105.96/27",
	}

	startStateOtherJson := `
{
 "VPCType": 0,
 "PublicRouteTableID": "rtb-014640137d63257c4",
 "RouteTables": {
  "rtb-014640137d63257c4": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "",
     "InternetGatewayID": "igw-0f6a57d0903bab058",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Public",
   "EdgeAssociationType": ""
  },
  "rtb-06e915943fb2a50d2": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-092e460968326ed0d",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    },
    {
     "Destination": "10.147.51.0/27",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "pcx-08ce5ffda6a84d6f5",
     "VPCEndpointID": ""
    },
    {
     "Destination": "10.147.51.32/27",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "pcx-08ce5ffda6a84d6f5",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-0ed04415dc476ad65": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-0c9994008931c2e9c",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    },
    {
     "Destination": "10.147.51.0/27",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "pcx-08ce5ffda6a84d6f5",
     "VPCEndpointID": ""
    },
    {
     "Destination": "10.147.51.32/27",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "pcx-08ce5ffda6a84d6f5",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  }
 },
 "InternetGateway": {
  "InternetGatewayID": "igw-0f6a57d0903bab058",
  "IsInternetGatewayAttached": true,
  "RouteTableID": "",
  "RouteTableAssociationID": ""
 },
 "AvailabilityZones": {
  "us-east-1a": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-0a2c1c8540f6b69aa",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-0686bf23401af29ac",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0fdcc4c17447a651b",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-0dff996ca259e6a21",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-06e915943fb2a50d2",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-092e460968326ed0d",
    "EIPID": "eipalloc-09a774c7853896d25"
   }
  },
  "us-east-1b": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-0db05543735fefa17",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-0e2cde3a31ca9fd61",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0c21dbbe5531d0266",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-09668784e1af667e8",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-0ed04415dc476ad65",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-0c9994008931c2e9c",
    "EIPID": "eipalloc-0cd33c69aab772775"
   }
  }
 },
 "TransitGatewayAttachments": null,
 "ResolverRuleAssociations": [],
 "SecurityGroups": null,
 "S3FlowLogID": "fl-084bcb1ec7bfc05d2",
 "CloudWatchLogsFlowLogID": "",
 "ResolverQueryLogConfigurationID": "rqlc-e243fefe7da0480d",
 "ResolverQueryLogAssociationID": "rqlca-d803cd85dfa74946",
 "Firewall": null,
 "FirewallRouteTableID": ""
}
`

	startStateJson := `
{
 "VPCType": 0,
 "PublicRouteTableID": "rtb-07ae3fa56c8be961e",
 "RouteTables": {
  "rtb-02eafba7a6689ce83": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-09e672dd8f7d98b7c",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    },
    {
     "Destination": "10.147.64.0/27",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "pcx-08ce5ffda6a84d6f5",
     "VPCEndpointID": ""
    },
    {
     "Destination": "10.147.64.32/27",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "pcx-08ce5ffda6a84d6f5",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-049cde62c954b5406": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-0c125fea02c8c1077",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    },
    {
     "Destination": "10.147.64.0/27",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "pcx-08ce5ffda6a84d6f5",
     "VPCEndpointID": ""
    },
    {
     "Destination": "10.147.64.32/27",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "pcx-08ce5ffda6a84d6f5",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-07ae3fa56c8be961e": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "",
     "InternetGatewayID": "igw-067235b0a3c2b0096",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Public",
   "EdgeAssociationType": ""
  }
 },
 "InternetGateway": {
  "InternetGatewayID": "igw-067235b0a3c2b0096",
  "IsInternetGatewayAttached": true,
  "RouteTableID": "",
  "RouteTableAssociationID": ""
 },
 "AvailabilityZones": {
  "us-east-1a": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-0fe27ec34f6e076da",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-069201384eec31008",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0aa4c424af4e58114",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-0c1b1f68efce3f0b5",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-02eafba7a6689ce83",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-09e672dd8f7d98b7c",
    "EIPID": "eipalloc-0e62654bbb17823e1"
   }
  },
  "us-east-1b": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-0d0dca144386166d7",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-0c27a9e95cc79ccd2",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-071c6fba1ab18c7b1",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-09d63f6774aba8474",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-049cde62c954b5406",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-0c125fea02c8c1077",
    "EIPID": "eipalloc-0138f85405e964112"
   }
  },
  "us-east-1c": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-04a6fef5eb22e49c1",
      "GroupName": "private",
      "RouteTableAssociationID": "",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0649e25ce78faf4ad",
      "GroupName": "public",
      "RouteTableAssociationID": "",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "",
    "EIPID": ""
   }
  }
 },
 "TransitGatewayAttachments": null,
 "ResolverRuleAssociations": [],
 "SecurityGroups": null,
 "S3FlowLogID": "fl-0840dfba2d76b92ac",
 "CloudWatchLogsFlowLogID": "",
 "ResolverQueryLogConfigurationID": "rqlc-c51bd2e1541e4dd1",
 "ResolverQueryLogAssociationID": "rqlca-dc1c3dc26f564dfc",
 "Firewall": null,
 "FirewallRouteTableID": ""
}
`

	existingContainersJson := `
{
 "Name": "/Global/AWS/V4/Commercial/East",
 "ResourceID": "",
 "Blocks": null,
 "Children": [
  {
   "Name": "/Global/AWS/V4/Commercial/East/Development and Test",
   "ResourceID": "",
   "Blocks": [
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.147.51.224",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.147.66.0",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.147.72.0",
     "BlockType": "Environment CIDR Block",
     "Size": 25,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.147.73.160",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.147.74.160",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.147.105.224",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.147.107.0",
     "BlockType": "Environment CIDR Block",
     "Size": 25,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.147.135.0",
     "BlockType": "Environment CIDR Block",
     "Size": 26,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.202.2.0",
     "BlockType": "Environment CIDR Block",
     "Size": 25,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.202.3.64",
     "BlockType": "Environment CIDR Block",
     "Size": 26,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.202.3.128",
     "BlockType": "Environment CIDR Block",
     "Size": 26,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.202.18.128",
     "BlockType": "Environment CIDR Block",
     "Size": 26,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.202.159.64",
     "BlockType": "Environment CIDR Block",
     "Size": 26,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.202.159.128",
     "BlockType": "Environment CIDR Block",
     "Size": 25,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.203.98.0",
     "BlockType": "Environment CIDR Block",
     "Size": 23,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.203.100.0",
     "BlockType": "Environment CIDR Block",
     "Size": 22,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.203.104.0",
     "BlockType": "Environment CIDR Block",
     "Size": 21,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.203.112.0",
     "BlockType": "Environment CIDR Block",
     "Size": 20,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.205.0.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.240.192.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.242.27.32",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.242.27.128",
     "BlockType": "Environment CIDR Block",
     "Size": 25,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.242.38.0",
     "BlockType": "Environment CIDR Block",
     "Size": 24,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.242.74.224",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.242.120.192",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.147.0.0",
     "BlockType": "Environment CIDR Block",
     "Size": 16,
     "Status": "Aggregate"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.202.0.0",
     "BlockType": "Environment CIDR Block",
     "Size": 16,
     "Status": "Aggregate"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.203.64.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Aggregate"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.205.0.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Aggregate"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.240.192.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Aggregate"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.242.0.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Aggregate"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.242.64.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Aggregate"
    }
   ],
   "Children": [
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test",
     "ResourceID": "vpc-075a5231051b57ac9",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test",
       "Address": "10.147.51.0",
       "BlockType": "VPC CIDR Block",
       "Size": 25,
       "Status": "Aggregate"
      },
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test",
       "Address": "10.147.105.64",
       "BlockType": "VPC CIDR Block",
       "Size": 26,
       "Status": "Aggregate"
      }
     ],
     "Children": [
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/private-a",
       "ResourceID": "subnet-0fe27ec34f6e076da",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/private-a",
         "Address": "10.147.51.0",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/private-b",
       "ResourceID": "subnet-0d0dca144386166d7",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/private-b",
         "Address": "10.147.51.32",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/public-b",
       "ResourceID": "subnet-071c6fba1ab18c7b1",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/public-b",
         "Address": "10.147.51.96",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/public-a",
       "ResourceID": "subnet-0aa4c424af4e58114",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/public-a",
         "Address": "10.147.51.64",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/private-c",
       "ResourceID": "subnet-04a6fef5eb22e49c1",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/private-c",
         "Address": "10.147.105.64",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/public-c",
       "ResourceID": "subnet-0649e25ce78faf4ad",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/public-c",
         "Address": "10.147.105.96",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      }
     ]
    }
   ]
  }
 ]
}
`

	endStateJson := `
{
 "VPCType": 0,
 "PublicRouteTableID": "rtb-07ae3fa56c8be961e",
 "RouteTables": {
  "rtb-02eafba7a6689ce83": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-09e672dd8f7d98b7c",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    },
    {
     "Destination": "10.147.64.0/27",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "pcx-08ce5ffda6a84d6f5",
     "VPCEndpointID": ""
    },
    {
     "Destination": "10.147.64.32/27",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "pcx-08ce5ffda6a84d6f5",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-049cde62c954b5406": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-0c125fea02c8c1077",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    },
    {
     "Destination": "10.147.64.0/27",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "pcx-08ce5ffda6a84d6f5",
     "VPCEndpointID": ""
    },
    {
     "Destination": "10.147.64.32/27",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "pcx-08ce5ffda6a84d6f5",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-076c39eb186205a48": {
   "Routes": [
    {
     "Destination": "10.147.64.0/27",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "pcx-08ce5ffda6a84d6f5",
     "VPCEndpointID": ""
    },
    {
     "Destination": "10.147.64.32/27",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "pcx-08ce5ffda6a84d6f5",
     "VPCEndpointID": ""
    },
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-0b37cb989ca2131d7",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-07ae3fa56c8be961e": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "",
     "InternetGatewayID": "igw-067235b0a3c2b0096",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Public",
   "EdgeAssociationType": ""
  }
 },
 "InternetGateway": {
  "InternetGatewayID": "igw-067235b0a3c2b0096",
  "IsInternetGatewayAttached": true,
  "RouteTableID": "",
  "RouteTableAssociationID": ""
 },
 "AvailabilityZones": {
  "us-east-1a": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-0fe27ec34f6e076da",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-069201384eec31008",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0aa4c424af4e58114",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-0c1b1f68efce3f0b5",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-02eafba7a6689ce83",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-09e672dd8f7d98b7c",
    "EIPID": "eipalloc-0e62654bbb17823e1"
   }
  },
  "us-east-1b": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-0d0dca144386166d7",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-0c27a9e95cc79ccd2",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-071c6fba1ab18c7b1",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-09d63f6774aba8474",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-049cde62c954b5406",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-0c125fea02c8c1077",
    "EIPID": "eipalloc-0138f85405e964112"
   }
  },
  "us-east-1c": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-04a6fef5eb22e49c1",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-00ef860590c1d81c2",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0649e25ce78faf4ad",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-0440705a15cc1f381",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-076c39eb186205a48",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-0b37cb989ca2131d7",
    "EIPID": "eipalloc-0afac73ce6f745cc3"
   }
  }
 },
 "TransitGatewayAttachments": null,
 "ResolverRuleAssociations": [],
 "SecurityGroups": null,
 "S3FlowLogID": "fl-0840dfba2d76b92ac",
 "CloudWatchLogsFlowLogID": "",
 "ResolverQueryLogConfigurationID": "rqlc-c51bd2e1541e4dd1",
 "ResolverQueryLogAssociationID": "rqlca-dc1c3dc26f564dfc",
 "Firewall": null,
 "FirewallRouteTableID": ""
}
`

	endContainersJson := `
{
 "Name": "/Global/AWS/V4/Commercial/East",
 "ResourceID": "",
 "Blocks": null,
 "Children": [
  {
   "Name": "/Global/AWS/V4/Commercial/East/Development and Test",
   "ResourceID": "",
   "Blocks": [
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.147.51.224",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.147.66.0",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.147.72.0",
     "BlockType": "Environment CIDR Block",
     "Size": 25,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.147.73.160",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.147.74.160",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.147.105.224",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.147.107.0",
     "BlockType": "Environment CIDR Block",
     "Size": 25,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.147.135.0",
     "BlockType": "Environment CIDR Block",
     "Size": 26,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.202.2.0",
     "BlockType": "Environment CIDR Block",
     "Size": 25,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.202.3.64",
     "BlockType": "Environment CIDR Block",
     "Size": 26,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.202.3.128",
     "BlockType": "Environment CIDR Block",
     "Size": 26,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.202.18.128",
     "BlockType": "Environment CIDR Block",
     "Size": 26,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.202.159.64",
     "BlockType": "Environment CIDR Block",
     "Size": 26,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.202.159.128",
     "BlockType": "Environment CIDR Block",
     "Size": 25,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.203.98.0",
     "BlockType": "Environment CIDR Block",
     "Size": 23,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.203.100.0",
     "BlockType": "Environment CIDR Block",
     "Size": 22,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.203.104.0",
     "BlockType": "Environment CIDR Block",
     "Size": 21,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.203.112.0",
     "BlockType": "Environment CIDR Block",
     "Size": 20,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.205.0.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.240.192.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.242.27.32",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.242.27.128",
     "BlockType": "Environment CIDR Block",
     "Size": 25,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.242.38.0",
     "BlockType": "Environment CIDR Block",
     "Size": 24,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.242.74.224",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.242.120.192",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.147.0.0",
     "BlockType": "Environment CIDR Block",
     "Size": 16,
     "Status": "Aggregate"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.202.0.0",
     "BlockType": "Environment CIDR Block",
     "Size": 16,
     "Status": "Aggregate"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.203.64.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Aggregate"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.205.0.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Aggregate"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.240.192.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Aggregate"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.242.0.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Aggregate"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test",
     "Address": "10.242.64.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Aggregate"
    }
   ],
   "Children": [
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test",
     "ResourceID": "vpc-075a5231051b57ac9",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test",
       "Address": "10.147.51.0",
       "BlockType": "VPC CIDR Block",
       "Size": 25,
       "Status": "Aggregate"
      },
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test",
       "Address": "10.147.105.64",
       "BlockType": "VPC CIDR Block",
       "Size": 26,
       "Status": "Aggregate"
      }
     ],
     "Children": [
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/private-a",
       "ResourceID": "subnet-0fe27ec34f6e076da",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/private-a",
         "Address": "10.147.51.0",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/private-b",
       "ResourceID": "subnet-0d0dca144386166d7",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/private-b",
         "Address": "10.147.51.32",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/public-b",
       "ResourceID": "subnet-071c6fba1ab18c7b1",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/public-b",
         "Address": "10.147.51.96",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/private-c",
       "ResourceID": "subnet-04a6fef5eb22e49c1",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/private-c",
         "Address": "10.147.105.64",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/public-c",
       "ResourceID": "subnet-0649e25ce78faf4ad",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/public-c",
         "Address": "10.147.105.96",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/public-a",
       "ResourceID": "subnet-0aa4c424af4e58114",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp5-east-test/public-a",
         "Address": "10.147.51.64",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      }
     ]
    }
   ]
  }
 ]
}
`

	endContainers := testmocks.ContainerTree{}
	err := json.Unmarshal([]byte(endContainersJson), &endContainers)
	if err != nil {
		fmt.Println(err)
	}

	preDefinedSubnetIDQueue := map[string][]string{}

	preDefinedNatGatewayIDQueue := []string{
		"nat-0b37cb989ca2131d7"}
	preDefinedRouteTableIDQueue := []string{
		"rtb-076c39eb186205a48"}
	preDefinedRouteTableAssociationIDQueue := []string{
		"rtbassoc-0c1b1f68efce3f0b5", "rtbassoc-09d63f6774aba8474", "rtbassoc-0440705a15cc1f381", "rtbassoc-00ef860590c1d81c2", "rtbassoc-a", "rtbassoc-b"}
	preDefinedEIPQueue := []string{
		"eipalloc-0afac73ce6f745cc3"}

	startState := database.VPCState{}
	err = json.Unmarshal([]byte(startStateJson), &startState)
	if err != nil {
		fmt.Println(err)
	}

	for routeID, route := range startState.RouteTables {
		route.RouteTableID = routeID
	}

	startStateOther := database.VPCState{}
	err = json.Unmarshal([]byte(startStateOtherJson), &startStateOther)
	if err != nil {
		fmt.Println(err)
	}

	endState := database.VPCState{}
	err = json.Unmarshal([]byte(endStateJson), &endState)
	if err != nil {
		fmt.Println(err)
	}

	existingContainers := testmocks.ContainerTree{}
	err = json.Unmarshal([]byte(existingContainersJson), &existingContainers)
	if err != nil {
		fmt.Println(err)
	}

	TaskConfig := database.UpdateNetworkingTaskData{
		VPCID:     "vpc-075a5231051b57ac9",
		AWSRegion: "us-east-1",
		NetworkingConfig: database.NetworkingConfig{
			ConnectPublic:  true,
			ConnectPrivate: true,
			PeeringConnections: []*database.PeeringConnectionConfig{
				{
					IsRequester:                 false,
					OtherVPCID:                  "vpc-076177219e68e5052",
					OtherVPCRegion:              "us-east-1",
					ConnectPrivate:              true,
					ConnectSubnetGroups:         []string{},
					OtherVPCConnectPrivate:      true,
					OtherVPCConnectSubnetGroups: []string{},
				},
			},
		},
	}

	ExistingVPCCIDRBlocks := []*ec2.VpcCidrBlockAssociation{
		{
			AssociationId: aws.String("vpc-cidr-assoc-0461b4cbd930c132a"),
			CidrBlock:     aws.String("10.147.51.0/25"),
			CidrBlockState: &ec2.VpcCidrBlockState{
				State: aws.String("associated"),
			},
		},
		{
			AssociationId: aws.String("vpc-cidr-assoc-0b1c5fbb5c7922726"),
			CidrBlock:     aws.String("10.147.105.64/26"),
			CidrBlockState: &ec2.VpcCidrBlockState{
				State: aws.String("associated"),
			},
		},
	}

	ExistingRouteTables := []*ec2.RouteTable{
		{
			RouteTableId: aws.String("rtb-02eafba7a6689ce83"),
			VpcId:        aws.String("vpc-075a5231051b57ac9"),
		},
		{
			RouteTableId: aws.String("rtb-07ae3fa56c8be961e"),
			VpcId:        aws.String("vpc-075a5231051b57ac9"),
		},
		{
			RouteTableId: aws.String("rtb-049cde62c954b5406"),
			VpcId:        aws.String("vpc-075a5231051b57ac9"),
		},
		{
			RouteTableId: aws.String("rtb-010f6bb01f9e3cbc2"),
			VpcId:        aws.String("vpc-075a5231051b57ac9"),
		},
	}

	ExistingPeeringConnections := []*ec2.VpcPeeringConnection{
		{
			VpcPeeringConnectionId: aws.String("pcx-08ce5ffda6a84d6f5"),
			AccepterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
				VpcId: aws.String("vpc-075a5231051b57ac9"),
				CidrBlockSet: []*ec2.CidrBlock{
					{
						CidrBlock: aws.String("10.147.51.0/27"),
					},
				},
			},
			RequesterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
				VpcId: aws.String("vpc-076177219e68e5052"),
				CidrBlockSet: []*ec2.CidrBlock{
					{
						CidrBlock: aws.String("10.147.64.0/27"),
					},
				},
			},
			Status: &ec2.VpcPeeringConnectionStateReason{Code: aws.String("active")},
		},
	}

	startState.PeeringConnections = []*database.PeeringConnection{
		{
			RequesterVPCID:      "vpc-076177219e68e5052",
			RequesterRegion:     "us-east-1",
			AccepterVPCID:       "vpc-075a5231051b57ac9",
			AccepterRegion:      "us-east-1",
			PeeringConnectionID: "pcx-08ce5ffda6a84d6f5",
			IsAccepted:          true,
		},
	}

	tc := AZExpansionPeeringUpdateNetworking{
		VPCName:                    "alc-az-exp5-east-test",
		VPCID:                      "vpc-075a5231051b57ac9",
		Region:                     "us-east-1",
		Stack:                      "test",
		TaskConfig:                 TaskConfig,
		ExistingSubnetCIDRs:        ExistingSubnetCIDRs,
		ExistingVPCCIDRBlocks:      ExistingVPCCIDRBlocks,
		StartState:                 startState,
		ExistingContainers:         existingContainers,
		ExistingRouteTables:        ExistingRouteTables,
		ExistingPeeringConnections: ExistingPeeringConnections,
		ExpectedTaskStatus:         database.TaskStatusSuccessful,
	}

	taskId := uint64(1235)
	vpcKey := string(tc.Region) + tc.VPCID
	mm := &testmocks.MockModelsManager{
		VPCs: map[string]*database.VPC{
			vpcKey: {
				AccountID: "346570397073",
				ID:        tc.VPCID,
				State:     &tc.StartState,
				Name:      tc.VPCName,
				Stack:     tc.Stack,
				Region:    tc.Region,
			},
			"us-east-1vpc-076177219e68e5052": {
				AccountID: "346570397073",
				ID:        "vpc-076177219e68e5052",
				State:     &startStateOther,
				Name:      "alc-az-exp6-east-test",
				Stack:     "test",
				Region:    "us-east-1",
			},
		},
	}

	ipcontrol := &testmocks.MockIPControl{
		ExistingContainers: tc.ExistingContainers,
		BlocksDeleted:      []string{},
	}

	ec2Mock := &testmocks.MockEC2{
		PeeringConnections:                     &tc.ExistingPeeringConnections,
		PeeringConnectionStatus:                map[string]string{"pcx-08ce5ffda6a84d6f5": "active"},
		PrimaryCIDR:                            aws.String("10.1`47.51.0/25"),
		CIDRBlockAssociationSet:                tc.ExistingVPCCIDRBlocks,
		RouteTables:                            tc.ExistingRouteTables,
		SubnetCIDRs:                            tc.ExistingSubnetCIDRs,
		PreDefinedSubnetIDQueue:                preDefinedSubnetIDQueue,
		PreDefinedNatGatewayIDQueue:            preDefinedNatGatewayIDQueue,
		PreDefinedRouteTableIDQueue:            preDefinedRouteTableIDQueue,
		PreDefinedRouteTableAssociationIDQueue: preDefinedRouteTableAssociationIDQueue,
		PreDefinedEIPQueue:                     preDefinedEIPQueue,
	}

	ec2Other := testmocks.MockEC2{
		PeeringConnections: &[]*ec2.VpcPeeringConnection{
			{
				VpcPeeringConnectionId: aws.String("pcx-08ce5ffda6a84d6f5"),
				AccepterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
					VpcId: aws.String("vpc-075a5231051b57ac9"),
					CidrBlockSet: []*ec2.CidrBlock{
						{
							CidrBlock: aws.String("10.147.51.0/27"),
						},
					},
				},
				RequesterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
					VpcId: aws.String("vpc-076177219e68e5052"),
					CidrBlockSet: []*ec2.CidrBlock{
						{
							CidrBlock: aws.String("10.147.64.0/27"),
						},
					},
				},
				Status: &ec2.VpcPeeringConnectionStateReason{Code: aws.String("active")},
			},
		},

		PeeringConnectionStatus:   map[string]string{"pcx-08ce5ffda6a84d6f5": "active"},
		PeeringConnectionsCreated: &[]*ec2.VpcPeeringConnection{},
		SubnetCIDRs: map[string]string{
			"subnet-0db05543735fefa17": "10.147.64.32/27",
			"subnet-0a2c1c8540f6b69aa": "10.147.64.0/27",
		},
		RouteTables: []*ec2.RouteTable{
			{
				RouteTableId: aws.String("rtb-014640137d63257c4"),
				VpcId:        aws.String("vpc-076177219e68e5052"),
			},
			{
				RouteTableId: aws.String("rtb-06e915943fb2a50d2"),
				VpcId:        aws.String("vpc-076177219e68e5052"),
			},
			{
				RouteTableId: aws.String("rtb-0ed04415dc476ad65"),
				VpcId:        aws.String("vpc-076177219e68e5052"),
			},
		},
	}

	task := &testmocks.MockTask{
		ID: taskId,
	}
	taskContext := &TaskContext{
		Task:          task,
		ModelsManager: mm,
		LockSet:       database.GetFakeLockSet(database.TargetVPC(tc.VPCID), database.TargetVPC("vpc-076177219e68e5052"), database.TargetIPControlWrite),
		IPAM:          ipcontrol,
		BaseAWSAccountAccess: &awsp.AWSAccountAccess{
			EC2svc: ec2Mock,
		},
		AWSAccountAccessProvider: &testmocks.MockAWSAccountAccessProvider{
			EC2: &ec2Other,
		},
		CMSNet: &testmocks.MockCMSNet{},
	}

	taskContext.performUpdateNetworkingTask(&tc.TaskConfig)

	if task.Status != tc.ExpectedTaskStatus {
		t.Fatalf("Incorrect task status. Expected %s but got %s", tc.ExpectedTaskStatus, task.Status)
	}

	testhelpers.SortIpcontrolContainersAndBlocks(&endContainers)
	testhelpers.SortIpcontrolContainersAndBlocks(&ipcontrol.ExistingContainers)
	if diff := cmp.Diff(endContainers, ipcontrol.ExistingContainers, cmpopts.EquateEmpty()); diff != "" {
		t.Fatalf("Expected end containers did not match mock containers: \n%s\n\nSide By Side Diff:\n%s", diff, testhelpers.ObjectGoPrintSideBySide(endContainers, ipcontrol.ExistingContainers))
	}

	testStateJson, _ := json.Marshal(*mm.VPCs[vpcKey].State)
	testState := database.VPCState{}
	err = json.Unmarshal([]byte(testStateJson), &testState)
	if err != nil {
		fmt.Println(err)
	}

	// Routes are in a random order, sort before diffing
	testhelpers.SortVPCState(&endState)
	testhelpers.SortVPCState(&testState)
	// Saved state
	if diff := cmp.Diff(endState, testState, cmpopts.EquateEmpty()); diff != "" {
		t.Fatalf("Expected end state did not match state saved to database: \n%s\n\nSide By Side Diff:\n%s", diff, testhelpers.ObjectGoPrintSideBySide(endState, testState))
	}
}
