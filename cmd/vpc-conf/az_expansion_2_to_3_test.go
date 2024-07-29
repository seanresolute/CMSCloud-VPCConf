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

type AZExpansion2to3 struct {
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

	TaskConfig database.AddAvailabilityZoneTaskData

	ExpectedTaskStatus database.TaskStatus
	ExpectedEndState   database.VPCState
}

func TestPerformAZExpansion2to3(t *testing.T) {
	ExistingSubnetCIDRs := map[string]string{
		"subnet-014f9cfc075b16f5a": "10.147.51.32/27",
		"subnet-0b703ba29977d171b": "10.147.51.0/27",
		"subnet-0307e1585344dcc7d": "10.147.51.64/27",
		"subnet-0207517373eb0bcd7": "10.147.51.96/27",
	}

	startStateJson := `
{
 "VPCType": 0,
 "PublicRouteTableID": "rtb-0162b00e6214372d4",
 "RouteTables": {
  "rtb-0162b00e6214372d4": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "",
     "InternetGatewayID": "igw-05c8b4723e72d95d2",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Public",
   "EdgeAssociationType": ""
  },
  "rtb-080c5d241dac7dcb3": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-04a84e81f3e70d3d4",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-0e83857d6443592be": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-0cf279b4cd5581e15",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  }
 },
 "InternetGateway": {
  "InternetGatewayID": "igw-05c8b4723e72d95d2",
  "IsInternetGatewayAttached": true,
  "RouteTableID": "",
  "RouteTableAssociationID": ""
 },
 "AvailabilityZones": {
  "us-east-1a": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-0b703ba29977d171b",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-093d60d67ffde02e1",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0307e1585344dcc7d",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-046cd46139085e4d4",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-080c5d241dac7dcb3",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-04a84e81f3e70d3d4",
    "EIPID": "eipalloc-0516c75e061c35b2a"
   }
  },
  "us-east-1b": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-014f9cfc075b16f5a",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-0e12ba18f9d45c9aa",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0207517373eb0bcd7",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-053005446ab709bed",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-0e83857d6443592be",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-0cf279b4cd5581e15",
    "EIPID": "eipalloc-0f55cdb12c06861ea"
   }
  }
 },
 "TransitGatewayAttachments": null,
 "ResolverRuleAssociations": [],
 "SecurityGroups": null,
 "S3FlowLogID": "fl-0edf7283bd3fa3f81",
 "CloudWatchLogsFlowLogID": "",
 "ResolverQueryLogConfigurationID": "rqlc-e847850c1dca415a",
 "ResolverQueryLogAssociationID": "rqlca-65dcbfc07c6a46ef",
 "Firewall": null,
 "FirewallRouteTableID": ""
}
`

	existingContainersJson := `
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
   "Address": "10.147.64.0",
   "BlockType": "Environment CIDR Block",
   "Size": 25,
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
   "Address": "10.147.105.64",
   "BlockType": "Environment CIDR Block",
   "Size": 26,
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
   "Address": "10.203.96.0",
   "BlockType": "Environment CIDR Block",
   "Size": 19,
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
   "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test",
   "ResourceID": "vpc-06a0a146872dd4d3b",
   "Blocks": [
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test",
     "Address": "10.147.51.0",
     "BlockType": "VPC CIDR Block",
     "Size": 25,
     "Status": "Aggregate"
    }
   ],
   "Children": [
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/private-a",
     "ResourceID": "subnet-0b703ba29977d171b",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/private-a",
       "Address": "10.147.51.0",
       "BlockType": "Subnet CIDR Block",
       "Size": 27,
       "Status": "Deployed"
      }
     ],
     "Children": null
    },
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/private-b",
     "ResourceID": "subnet-014f9cfc075b16f5a",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/private-b",
       "Address": "10.147.51.32",
       "BlockType": "Subnet CIDR Block",
       "Size": 27,
       "Status": "Deployed"
      }
     ],
     "Children": null
    },
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/public-a",
     "ResourceID": "subnet-0307e1585344dcc7d",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/public-a",
       "Address": "10.147.51.64",
       "BlockType": "Subnet CIDR Block",
       "Size": 27,
       "Status": "Deployed"
      }
     ],
     "Children": null
    },
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/public-b",
     "ResourceID": "subnet-0207517373eb0bcd7",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/public-b",
       "Address": "10.147.51.96",
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
`

	endStateJson := `
{
 "VPCType": 0,
 "PublicRouteTableID": "rtb-0162b00e6214372d4",
 "RouteTables": {
  "rtb-0162b00e6214372d4": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "",
     "InternetGatewayID": "igw-05c8b4723e72d95d2",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Public",
   "EdgeAssociationType": ""
  },
  "rtb-080c5d241dac7dcb3": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-04a84e81f3e70d3d4",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-0e83857d6443592be": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-0cf279b4cd5581e15",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  }
 },
 "InternetGateway": {
  "InternetGatewayID": "igw-05c8b4723e72d95d2",
  "IsInternetGatewayAttached": true,
  "RouteTableID": "",
  "RouteTableAssociationID": ""
 },
 "AvailabilityZones": {
  "us-east-1a": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-0b703ba29977d171b",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-093d60d67ffde02e1",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0307e1585344dcc7d",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-046cd46139085e4d4",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-080c5d241dac7dcb3",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-04a84e81f3e70d3d4",
    "EIPID": "eipalloc-0516c75e061c35b2a"
   }
  },
  "us-east-1b": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-014f9cfc075b16f5a",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-0e12ba18f9d45c9aa",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0207517373eb0bcd7",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-053005446ab709bed",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-0e83857d6443592be",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-0cf279b4cd5581e15",
    "EIPID": "eipalloc-0f55cdb12c06861ea"
   }
  },
  "us-east-1c": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-0735b030d92cdf6b2",
      "GroupName": "private",
      "RouteTableAssociationID": "",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0c27d6714dda2ddba",
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
 "S3FlowLogID": "fl-0edf7283bd3fa3f81",
 "CloudWatchLogsFlowLogID": "",
 "ResolverQueryLogConfigurationID": "rqlc-e847850c1dca415a",
 "ResolverQueryLogAssociationID": "rqlca-65dcbfc07c6a46ef",
 "Firewall": null,
 "FirewallRouteTableID": ""
}
`

	endContainersJson := `
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
   "Address": "10.147.64.0",
   "BlockType": "Environment CIDR Block",
   "Size": 25,
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
   "Address": "10.203.96.0",
   "BlockType": "Environment CIDR Block",
   "Size": 19,
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
   "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test",
   "ResourceID": "vpc-06a0a146872dd4d3b",
   "Blocks": [
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test",
     "Address": "10.147.51.0",
     "BlockType": "VPC CIDR Block",
     "Size": 25,
     "Status": "Aggregate"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test",
     "Address": "10.147.105.64",
     "BlockType": "VPC CIDR Block",
     "Size": 26,
     "Status": "Aggregate"
    }
   ],
   "Children": [
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/private-a",
     "ResourceID": "subnet-0b703ba29977d171b",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/private-a",
       "Address": "10.147.51.0",
       "BlockType": "Subnet CIDR Block",
       "Size": 27,
       "Status": "Deployed"
      }
     ],
     "Children": null
    },
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/private-b",
     "ResourceID": "subnet-014f9cfc075b16f5a",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/private-b",
       "Address": "10.147.51.32",
       "BlockType": "Subnet CIDR Block",
       "Size": 27,
       "Status": "Deployed"
      }
     ],
     "Children": null
    },
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/public-a",
     "ResourceID": "subnet-0307e1585344dcc7d",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/public-a",
       "Address": "10.147.51.64",
       "BlockType": "Subnet CIDR Block",
       "Size": 27,
       "Status": "Deployed"
      }
     ],
     "Children": null
    },
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/public-b",
     "ResourceID": "subnet-0207517373eb0bcd7",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/public-b",
       "Address": "10.147.51.96",
       "BlockType": "Subnet CIDR Block",
       "Size": 27,
       "Status": "Deployed"
      }
     ],
     "Children": null
    },
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/private-c",
     "ResourceID": "subnet-0735b030d92cdf6b2",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/private-c",
       "Address": "10.147.105.64",
       "BlockType": "Subnet CIDR Block",
       "Size": 27,
       "Status": "Deployed"
      }
     ],
     "Children": null
    },
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/public-c",
     "ResourceID": "subnet-0c27d6714dda2ddba",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-case1-east-test/public-c",
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
`

	endContainers := testmocks.ContainerTree{}
	err := json.Unmarshal([]byte(endContainersJson), &endContainers)
	if err != nil {
		fmt.Println(err)
	}

	preDefinedSubnetIDQueue := map[string][]string{
		"us-east-1c": {"subnet-0735b030d92cdf6b2", "subnet-0c27d6714dda2ddba"},
	}

	preDefinedNatGatewayIDQueue := []string{}
	preDefinedRouteTableIDQueue := []string{}
	preDefinedRouteTableAssociationIDQueue := []string{}
	preDefinedEIPQueue := []string{}

	startState := database.VPCState{}
	err = json.Unmarshal([]byte(startStateJson), &startState)
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

	TaskConfig := database.AddAvailabilityZoneTaskData{
		VPCID:  "vpc-06a0a146872dd4d3b",
		Region: "us-east-1",
		AZName: "us-east-1c",
	}

	ExistingVPCCIDRBlocks := []*ec2.VpcCidrBlockAssociation{
		{
			AssociationId: aws.String("vpc-cidr-assoc-09aa1bfb68a31d01b"),
			CidrBlock:     aws.String("10.147.51.0/25"),
			CidrBlockState: &ec2.VpcCidrBlockState{
				State: aws.String("associated"),
			},
		},
	}

	ExistingRouteTables := []*ec2.RouteTable{
		{
			RouteTableId: aws.String("rtb-0162b00e6214372d4"),
			VpcId:        aws.String("vpc-06a0a146872dd4d3b"),
		},
		{
			RouteTableId: aws.String("rtb-0189e59ead3d4a5e2"),
			VpcId:        aws.String("vpc-06a0a146872dd4d3b"),
		},
		{
			RouteTableId: aws.String("rtb-0e83857d6443592be"),
			VpcId:        aws.String("vpc-06a0a146872dd4d3b"),
		},
		{
			RouteTableId: aws.String("rtb-080c5d241dac7dcb3"),
			VpcId:        aws.String("vpc-06a0a146872dd4d3b"),
		},
	}

	tc := AZExpansion2to3{
		VPCName:               "alc-case1-east-test",
		VPCID:                 "vpc-06a0a146872dd4d3b",
		Region:                "us-east-1",
		Stack:                 "test",
		TaskConfig:            TaskConfig,
		ExistingSubnetCIDRs:   ExistingSubnetCIDRs,
		ExistingVPCCIDRBlocks: ExistingVPCCIDRBlocks,
		StartState:            startState,
		ExistingContainers:    existingContainers,
		ExistingRouteTables:   ExistingRouteTables,
		ExpectedTaskStatus:    database.TaskStatusSuccessful,
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
		},
	}

	ipcontrol := &testmocks.MockIPControl{
		ExistingContainers: tc.ExistingContainers,
		BlocksDeleted:      []string{},
	}

	ec2 := &testmocks.MockEC2{
		//        PeeringConnections:      &tc.ExistingPeeringConnections,
		PrimaryCIDR:                            aws.String("10.147.51.0/25"),
		CIDRBlockAssociationSet:                tc.ExistingVPCCIDRBlocks,
		RouteTables:                            tc.ExistingRouteTables,
		SubnetCIDRs:                            tc.ExistingSubnetCIDRs,
		PreDefinedSubnetIDQueue:                preDefinedSubnetIDQueue,
		PreDefinedNatGatewayIDQueue:            preDefinedNatGatewayIDQueue,
		PreDefinedRouteTableIDQueue:            preDefinedRouteTableIDQueue,
		PreDefinedRouteTableAssociationIDQueue: preDefinedRouteTableAssociationIDQueue,
		PreDefinedEIPQueue:                     preDefinedEIPQueue,
	}
	task := &testmocks.MockTask{
		ID: taskId,
	}
	taskContext := &TaskContext{
		Task:          task,
		ModelsManager: mm,
		LockSet:       database.GetFakeLockSet(database.TargetVPC(tc.VPCID), database.TargetIPControlWrite),
		IPAM:          ipcontrol,
		BaseAWSAccountAccess: &awsp.AWSAccountAccess{
			EC2svc: ec2,
		},
		CMSNet: &testmocks.MockCMSNet{},
	}

	taskContext.performAddAvailabilityZoneTask(&tc.TaskConfig)

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

	// Saved state
	if diff := cmp.Diff(endState, testState, cmpopts.EquateEmpty()); diff != "" {
		t.Fatalf("Expected end state did not match state saved to database: \n%s\n\nSide By Side Diff:\n%s", diff, testhelpers.ObjectGoPrintSideBySide(endState, testState))
	}
}
