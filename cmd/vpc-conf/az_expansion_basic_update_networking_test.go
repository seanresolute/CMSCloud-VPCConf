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

type AZExpansionBasicUpdateNetworking struct {
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

func TestPerformAZExpansionBasicUpdateNetworking(t *testing.T) {
	ExistingSubnetCIDRs := map[string]string{
		"subnet-0486a11f96c282030": "10.147.135.0/27",
		"subnet-01fa1b2c78e0e63e8": "10.147.105.64/27",
		"subnet-0007daebb265febbf": "10.147.105.96/27",
		"subnet-0f8e0c829d21d8302": "10.147.135.32/27",
	}

	startStateJson := `
{
 "VPCType": 0,
 "PublicRouteTableID": "rtb-0f89259709059944c",
 "RouteTables": {
  "rtb-04634c0f8a9f0f3a2": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-016ed4a82a1443d7d",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-0f89259709059944c": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "",
     "InternetGatewayID": "igw-0442d3616f8c93bba",
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
  "InternetGatewayID": "igw-0442d3616f8c93bba",
  "IsInternetGatewayAttached": true,
  "RouteTableID": "",
  "RouteTableAssociationID": ""
 },
 "AvailabilityZones": {
  "us-east-1a": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-01fa1b2c78e0e63e8",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-03b47132befe09e72",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0007daebb265febbf",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-0e00109d1a11df289",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-04634c0f8a9f0f3a2",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-016ed4a82a1443d7d",
    "EIPID": "eipalloc-03ce53f41c78f41ad"
   }
  },
  "us-east-1f": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-0486a11f96c282030",
      "GroupName": "private",
      "RouteTableAssociationID": "",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0f8e0c829d21d8302",
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
 "S3FlowLogID": "fl-07117751de9c72618",
 "CloudWatchLogsFlowLogID": "",
 "ResolverQueryLogConfigurationID": "rqlc-6c687553b79d4a72",
 "ResolverQueryLogAssociationID": "rqlca-d4c06970cd4a4d93",
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
   "Address": "10.147.51.0",
   "BlockType": "Environment CIDR Block",
   "Size": 25,
   "Status": "Free"
  },
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
   "Address": "10.202.158.0",
   "BlockType": "Environment CIDR Block",
   "Size": 24,
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
   "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test",
   "ResourceID": "vpc-012c3c9f6f0982663",
   "Blocks": [
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test",
     "Address": "10.147.105.64",
     "BlockType": "VPC CIDR Block",
     "Size": 26,
     "Status": "Aggregate"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test",
     "Address": "10.147.135.0",
     "BlockType": "VPC CIDR Block",
     "Size": 26,
     "Status": "Aggregate"
    }
   ],
   "Children": [
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test/private-a",
     "ResourceID": "subnet-01fa1b2c78e0e63e8",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test/private-a",
       "Address": "10.147.105.64",
       "BlockType": "Subnet CIDR Block",
       "Size": 27,
       "Status": "Deployed"
      }
     ],
     "Children": null
    },
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test/public-a",
     "ResourceID": "subnet-0007daebb265febbf",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test/public-a",
       "Address": "10.147.105.96",
       "BlockType": "Subnet CIDR Block",
       "Size": 27,
       "Status": "Deployed"
      }
     ],
     "Children": null
    },
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test/private-f",
     "ResourceID": "subnet-0486a11f96c282030",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test/private-f",
       "Address": "10.147.135.0",
       "BlockType": "Subnet CIDR Block",
       "Size": 27,
       "Status": "Deployed"
      }
     ],
     "Children": null
    },
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test/public-f",
     "ResourceID": "subnet-0f8e0c829d21d8302",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test/public-f",
       "Address": "10.147.135.32",
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
 "PublicRouteTableID": "rtb-0f89259709059944c",
 "RouteTables": {
  "rtb-04634c0f8a9f0f3a2": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-016ed4a82a1443d7d",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-0b87dbf2bc455eaf8": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-0a0eea14a1914b9a1",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-0f89259709059944c": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "",
     "InternetGatewayID": "igw-0442d3616f8c93bba",
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
  "InternetGatewayID": "igw-0442d3616f8c93bba",
  "IsInternetGatewayAttached": true,
  "RouteTableID": "",
  "RouteTableAssociationID": ""
 },
 "AvailabilityZones": {
  "us-east-1a": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-01fa1b2c78e0e63e8",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-03b47132befe09e72",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0007daebb265febbf",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-0e00109d1a11df289",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-04634c0f8a9f0f3a2",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-016ed4a82a1443d7d",
    "EIPID": "eipalloc-03ce53f41c78f41ad"
   }
  },
  "us-east-1f": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-0486a11f96c282030",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-09ac4640a63318eb8",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0f8e0c829d21d8302",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-0d37995c0d7de63f3",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-0b87dbf2bc455eaf8",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-0a0eea14a1914b9a1",
    "EIPID": "eipalloc-00ec8059a8a3589a7"
   }
  }
 },
 "TransitGatewayAttachments": null,
 "ResolverRuleAssociations": [],
 "SecurityGroups": null,
 "S3FlowLogID": "fl-07117751de9c72618",
 "CloudWatchLogsFlowLogID": "",
 "ResolverQueryLogConfigurationID": "rqlc-6c687553b79d4a72",
 "ResolverQueryLogAssociationID": "rqlca-d4c06970cd4a4d93",
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
   "Address": "10.147.51.0",
   "BlockType": "Environment CIDR Block",
   "Size": 25,
   "Status": "Free"
  },
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
   "Address": "10.202.158.0",
   "BlockType": "Environment CIDR Block",
   "Size": 24,
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
   "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test",
   "ResourceID": "vpc-012c3c9f6f0982663",
   "Blocks": [
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test",
     "Address": "10.147.105.64",
     "BlockType": "VPC CIDR Block",
     "Size": 26,
     "Status": "Aggregate"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test",
     "Address": "10.147.135.0",
     "BlockType": "VPC CIDR Block",
     "Size": 26,
     "Status": "Aggregate"
    }
   ],
   "Children": [
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test/private-a",
     "ResourceID": "subnet-01fa1b2c78e0e63e8",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test/private-a",
       "Address": "10.147.105.64",
       "BlockType": "Subnet CIDR Block",
       "Size": 27,
       "Status": "Deployed"
      }
     ],
     "Children": null
    },
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test/public-a",
     "ResourceID": "subnet-0007daebb265febbf",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test/public-a",
       "Address": "10.147.105.96",
       "BlockType": "Subnet CIDR Block",
       "Size": 27,
       "Status": "Deployed"
      }
     ],
     "Children": null
    },
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test/private-f",
     "ResourceID": "subnet-0486a11f96c282030",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test/private-f",
       "Address": "10.147.135.0",
       "BlockType": "Subnet CIDR Block",
       "Size": 27,
       "Status": "Deployed"
      }
     ],
     "Children": null
    },
    {
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test/public-f",
     "ResourceID": "subnet-0f8e0c829d21d8302",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-pls-east-test/public-f",
       "Address": "10.147.135.32",
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

	preDefinedSubnetIDQueue := map[string][]string{}

	preDefinedNatGatewayIDQueue := []string{
		"nat-0a0eea14a1914b9a1"}
	preDefinedRouteTableIDQueue := []string{
		"rtb-0b87dbf2bc455eaf8"}
	preDefinedRouteTableAssociationIDQueue := []string{
		"rtbassoc-0e00109d1a11df289", "rtbassoc-0d37995c0d7de63f3", "rtbassoc-09ac4640a63318eb8"}
	preDefinedEIPQueue := []string{
		"eipalloc-00ec8059a8a3589a7"}

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

	TaskConfig := database.UpdateNetworkingTaskData{
		VPCID:     "vpc-012c3c9f6f0982663",
		AWSRegion: "us-east-1",
		NetworkingConfig: database.NetworkingConfig{
			ConnectPublic:  true,
			ConnectPrivate: true,
		},
	}

	ExistingVPCCIDRBlocks := []*ec2.VpcCidrBlockAssociation{
		{
			AssociationId: aws.String("vpc-cidr-assoc-0fe737f73d565b52d"),
			CidrBlock:     aws.String("10.147.105.64/26"),
			CidrBlockState: &ec2.VpcCidrBlockState{
				State: aws.String("associated"),
			},
		},
		{
			AssociationId: aws.String("vpc-cidr-assoc-057c7d95e3b3501bb"),
			CidrBlock:     aws.String("10.147.135.0/26"),
			CidrBlockState: &ec2.VpcCidrBlockState{
				State: aws.String("associated"),
			},
		},
	}

	ExistingRouteTables := []*ec2.RouteTable{
		{
			RouteTableId: aws.String("rtb-04634c0f8a9f0f3a2"),
			VpcId:        aws.String("vpc-012c3c9f6f0982663"),
		},
		{
			RouteTableId: aws.String("rtb-0e5c423796049fdeb"),
			VpcId:        aws.String("vpc-012c3c9f6f0982663"),
		},
		{
			RouteTableId: aws.String("rtb-0f89259709059944c"),
			VpcId:        aws.String("vpc-012c3c9f6f0982663"),
		},
	}

	tc := AZExpansionBasicUpdateNetworking{
		VPCName:               "alc-pls-east-test",
		VPCID:                 "vpc-012c3c9f6f0982663",
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
		PrimaryCIDR:                            aws.String("10.147.105.64/26"),
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

	// Saved state
	if diff := cmp.Diff(endState, testState, cmpopts.EquateEmpty()); diff != "" {
		t.Fatalf("Expected end state did not match state saved to database: \n%s\n\nSide By Side Diff:\n%s", diff, testhelpers.ObjectGoPrintSideBySide(endState, testState))
	}
}
