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

type AZExpansionZonedSubnet struct {
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

func TestPerformAZExpansionZonedSubnet(t *testing.T) {
	ExistingSubnetCIDRs := map[string]string{
		"subnet-0d6f71ecc209c3ded": "10.223.194.160/28",
		"subnet-0fecc33897decd0ce": "10.147.51.96/27",
		"subnet-0e370af2cb2ed21af": "10.147.51.0/27",
		"subnet-0fd5466273881a155": "10.223.194.176/28",
		"subnet-0ae0bb426f47a3ad3": "10.147.51.64/27",
		"subnet-048769580d753491b": "10.147.51.32/27",
	}

	startStateJson := `
{
 "VPCType": 0,
 "PublicRouteTableID": "rtb-00f9bc4f3d94a4bfd",
 "RouteTables": {
  "rtb-00f9bc4f3d94a4bfd": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "",
     "InternetGatewayID": "igw-0365c7befac2a04e7",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Public",
   "EdgeAssociationType": ""
  },
  "rtb-05d1170b896672605": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-030776dc0251af386",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Data",
   "EdgeAssociationType": ""
  },
  "rtb-0a2563f7179650875": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-0aba851204d0d65c8",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Data",
   "EdgeAssociationType": ""
  },
  "rtb-0b5a867c9e6edeab5": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-030776dc0251af386",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-0fd857e2865283315": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-0aba851204d0d65c8",
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
  "InternetGatewayID": "igw-0365c7befac2a04e7",
  "IsInternetGatewayAttached": true,
  "RouteTableID": "",
  "RouteTableAssociationID": ""
 },
 "AvailabilityZones": {
  "us-east-1a": {
   "Subnets": {
    "Data": [
     {
      "SubnetID": "subnet-0d6f71ecc209c3ded",
      "GroupName": "data",
      "RouteTableAssociationID": "rtbassoc-097c4f6da9df4b064",
      "CustomRouteTableID": "rtb-0a2563f7179650875"
     }
    ],
    "Private": [
     {
      "SubnetID": "subnet-0e370af2cb2ed21af",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-09575180b35b56477",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0ae0bb426f47a3ad3",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-06df726a819203f39",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-0fd857e2865283315",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-0aba851204d0d65c8",
    "EIPID": "eipalloc-045f70be89c34032a"
   }
  },
  "us-east-1b": {
   "Subnets": {
    "Data": [
     {
      "SubnetID": "subnet-0fd5466273881a155",
      "GroupName": "data",
      "RouteTableAssociationID": "rtbassoc-0c5e56b5ebc052828",
      "CustomRouteTableID": "rtb-05d1170b896672605"
     }
    ],
    "Private": [
     {
      "SubnetID": "subnet-048769580d753491b",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-03b2e66616ec78cf7",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0fecc33897decd0ce",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-03c9924b69043dd86",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-0b5a867c9e6edeab5",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-030776dc0251af386",
    "EIPID": "eipalloc-00ad08451e0258e3d"
   }
  }
 },
 "TransitGatewayAttachments": null,
 "ResolverRuleAssociations": [],
 "SecurityGroups": null,
 "S3FlowLogID": "fl-0f4f8681bd782cd8b",
 "CloudWatchLogsFlowLogID": "",
 "ResolverQueryLogConfigurationID": "rqlc-cb145478d27e4063",
 "ResolverQueryLogAssociationID": "rqlca-6f9bb51f81274bda",
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
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test",
     "ResourceID": "vpc-01fe543d7926866a2",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test",
       "Address": "10.147.51.0",
       "BlockType": "VPC CIDR Block",
       "Size": 25,
       "Status": "Aggregate"
      }
     ],
     "Children": [
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/private-b",
       "ResourceID": "subnet-048769580d753491b",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/private-b",
         "Address": "10.147.51.32",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/public-a",
       "ResourceID": "subnet-0ae0bb426f47a3ad3",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/public-a",
         "Address": "10.147.51.64",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/private-a",
       "ResourceID": "subnet-0e370af2cb2ed21af",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/private-a",
         "Address": "10.147.51.0",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/public-b",
       "ResourceID": "subnet-0fecc33897decd0ce",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/public-b",
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
  },
  {
   "Name": "/Global/AWS/V4/Commercial/East/Lower-Data",
   "ResourceID": "",
   "Blocks": [
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Lower-Data",
     "Address": "10.223.201.192",
     "BlockType": "Environment CIDR Block",
     "Size": 26,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Lower-Data",
     "Address": "10.223.203.128",
     "BlockType": "Environment CIDR Block",
     "Size": 25,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Lower-Data",
     "Address": "10.223.204.0",
     "BlockType": "Environment CIDR Block",
     "Size": 22,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Lower-Data",
     "Address": "10.223.208.0",
     "BlockType": "Environment CIDR Block",
     "Size": 20,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Lower-Data",
     "Address": "10.223.224.0",
     "BlockType": "Environment CIDR Block",
     "Size": 19,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Lower-Data",
     "Address": "10.223.32.0",
     "BlockType": "Environment CIDR Block",
     "Size": 19,
     "Status": "Aggregate"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Lower-Data",
     "Address": "10.223.192.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Aggregate"
    }
   ],
   "Children": [
    {
     "Name": "/Global/AWS/V4/Commercial/East/Lower-Data/346570397073-alc-az-exp2-east-test",
     "ResourceID": "vpc-01fe543d7926866a2",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Lower-Data/346570397073-alc-az-exp2-east-test",
       "Address": "10.223.194.160",
       "BlockType": "VPC CIDR Block",
       "Size": 27,
       "Status": "Aggregate"
      }
     ],
     "Children": [
      {
       "Name": "/Global/AWS/V4/Commercial/East/Lower-Data/346570397073-alc-az-exp2-east-test/data-a",
       "ResourceID": "subnet-0d6f71ecc209c3ded",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Lower-Data/346570397073-alc-az-exp2-east-test/data-a",
         "Address": "10.223.194.160",
         "BlockType": "Subnet CIDR Block",
         "Size": 28,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Lower-Data/346570397073-alc-az-exp2-east-test/data-b",
       "ResourceID": "subnet-0fd5466273881a155",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Lower-Data/346570397073-alc-az-exp2-east-test/data-b",
         "Address": "10.223.194.176",
         "BlockType": "Subnet CIDR Block",
         "Size": 28,
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
 "PublicRouteTableID": "rtb-00f9bc4f3d94a4bfd",
 "RouteTables": {
  "rtb-00f9bc4f3d94a4bfd": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "",
     "InternetGatewayID": "igw-0365c7befac2a04e7",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Public",
   "EdgeAssociationType": ""
  },
  "rtb-05d1170b896672605": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-030776dc0251af386",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Data",
   "EdgeAssociationType": ""
  },
  "rtb-0a2563f7179650875": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-0aba851204d0d65c8",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Data",
   "EdgeAssociationType": ""
  },
  "rtb-0b5a867c9e6edeab5": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-030776dc0251af386",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-0fd857e2865283315": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-0aba851204d0d65c8",
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
  "InternetGatewayID": "igw-0365c7befac2a04e7",
  "IsInternetGatewayAttached": true,
  "RouteTableID": "",
  "RouteTableAssociationID": ""
 },
 "AvailabilityZones": {
  "us-east-1a": {
   "Subnets": {
    "Data": [
     {
      "SubnetID": "subnet-0d6f71ecc209c3ded",
      "GroupName": "data",
      "RouteTableAssociationID": "rtbassoc-097c4f6da9df4b064",
      "CustomRouteTableID": "rtb-0a2563f7179650875"
     }
    ],
    "Private": [
     {
      "SubnetID": "subnet-0e370af2cb2ed21af",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-09575180b35b56477",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0ae0bb426f47a3ad3",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-06df726a819203f39",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-0fd857e2865283315",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-0aba851204d0d65c8",
    "EIPID": "eipalloc-045f70be89c34032a"
   }
  },
  "us-east-1b": {
   "Subnets": {
    "Data": [
     {
      "SubnetID": "subnet-0fd5466273881a155",
      "GroupName": "data",
      "RouteTableAssociationID": "rtbassoc-0c5e56b5ebc052828",
      "CustomRouteTableID": "rtb-05d1170b896672605"
     }
    ],
    "Private": [
     {
      "SubnetID": "subnet-048769580d753491b",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-03b2e66616ec78cf7",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0fecc33897decd0ce",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-03c9924b69043dd86",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-0b5a867c9e6edeab5",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-030776dc0251af386",
    "EIPID": "eipalloc-00ad08451e0258e3d"
   }
  },
  "us-east-1e": {
   "Subnets": {
    "Data": [
     {
      "SubnetID": "subnet-08c85851cd535a97f",
      "GroupName": "data",
      "RouteTableAssociationID": "",
      "CustomRouteTableID": ""
     }
    ],
    "Private": [
     {
      "SubnetID": "subnet-059e2b3a6942e4744",
      "GroupName": "private",
      "RouteTableAssociationID": "",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0b0facc12683f76f1",
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
 "S3FlowLogID": "fl-0f4f8681bd782cd8b",
 "CloudWatchLogsFlowLogID": "",
 "ResolverQueryLogConfigurationID": "rqlc-cb145478d27e4063",
 "ResolverQueryLogAssociationID": "rqlca-6f9bb51f81274bda",
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
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test",
     "ResourceID": "vpc-01fe543d7926866a2",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test",
       "Address": "10.147.51.0",
       "BlockType": "VPC CIDR Block",
       "Size": 25,
       "Status": "Aggregate"
      },
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test",
       "Address": "10.147.105.64",
       "BlockType": "VPC CIDR Block",
       "Size": 26,
       "Status": "Aggregate"
      }
     ],
     "Children": [
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/public-e",
       "ResourceID": "subnet-0b0facc12683f76f1",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/public-e",
         "Address": "10.147.105.96",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/private-b",
       "ResourceID": "subnet-048769580d753491b",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/private-b",
         "Address": "10.147.51.32",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/private-e",
       "ResourceID": "subnet-059e2b3a6942e4744",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/private-e",
         "Address": "10.147.105.64",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/public-a",
       "ResourceID": "subnet-0ae0bb426f47a3ad3",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/public-a",
         "Address": "10.147.51.64",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/public-b",
       "ResourceID": "subnet-0fecc33897decd0ce",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/public-b",
         "Address": "10.147.51.96",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/private-a",
       "ResourceID": "subnet-0e370af2cb2ed21af",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp2-east-test/private-a",
         "Address": "10.147.51.0",
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
  },
  {
   "Name": "/Global/AWS/V4/Commercial/East/Lower-Data",
   "ResourceID": "",
   "Blocks": [
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Lower-Data",
     "Address": "10.223.201.208",
     "BlockType": "Environment CIDR Block",
     "Size": 28,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Lower-Data",
     "Address": "10.223.201.224",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Lower-Data",
     "Address": "10.223.203.128",
     "BlockType": "Environment CIDR Block",
     "Size": 25,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Lower-Data",
     "Address": "10.223.204.0",
     "BlockType": "Environment CIDR Block",
     "Size": 22,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Lower-Data",
     "Address": "10.223.208.0",
     "BlockType": "Environment CIDR Block",
     "Size": 20,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Lower-Data",
     "Address": "10.223.224.0",
     "BlockType": "Environment CIDR Block",
     "Size": 19,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Lower-Data",
     "Address": "10.223.32.0",
     "BlockType": "Environment CIDR Block",
     "Size": 19,
     "Status": "Aggregate"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/East/Lower-Data",
     "Address": "10.223.192.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Aggregate"
    }
   ],
   "Children": [
    {
     "Name": "/Global/AWS/V4/Commercial/East/Lower-Data/346570397073-alc-az-exp2-east-test",
     "ResourceID": "vpc-01fe543d7926866a2",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Lower-Data/346570397073-alc-az-exp2-east-test",
       "Address": "10.223.194.160",
       "BlockType": "VPC CIDR Block",
       "Size": 27,
       "Status": "Aggregate"
      },
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Lower-Data/346570397073-alc-az-exp2-east-test",
       "Address": "10.223.201.192",
       "BlockType": "VPC CIDR Block",
       "Size": 28,
       "Status": "Aggregate"
      }
     ],
     "Children": [
      {
       "Name": "/Global/AWS/V4/Commercial/East/Lower-Data/346570397073-alc-az-exp2-east-test/data-a",
       "ResourceID": "subnet-0d6f71ecc209c3ded",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Lower-Data/346570397073-alc-az-exp2-east-test/data-a",
         "Address": "10.223.194.160",
         "BlockType": "Subnet CIDR Block",
         "Size": 28,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Lower-Data/346570397073-alc-az-exp2-east-test/data-b",
       "ResourceID": "subnet-0fd5466273881a155",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Lower-Data/346570397073-alc-az-exp2-east-test/data-b",
         "Address": "10.223.194.176",
         "BlockType": "Subnet CIDR Block",
         "Size": 28,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Lower-Data/346570397073-alc-az-exp2-east-test/data-e",
       "ResourceID": "subnet-08c85851cd535a97f",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Lower-Data/346570397073-alc-az-exp2-east-test/data-e",
         "Address": "10.223.201.192",
         "BlockType": "Subnet CIDR Block",
         "Size": 28,
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

	preDefinedSubnetIDQueue := map[string][]string{
		"us-east-1e": {"subnet-059e2b3a6942e4744", "subnet-0b0facc12683f76f1", "subnet-08c85851cd535a97f"},
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
		VPCID:  "vpc-01fe543d7926866a2",
		Region: "us-east-1",
		AZName: "us-east-1e",
	}

	ExistingVPCCIDRBlocks := []*ec2.VpcCidrBlockAssociation{
		{
			AssociationId: aws.String("vpc-cidr-assoc-01ece19bc541cadcc"),
			CidrBlock:     aws.String("10.147.51.0/25"),
			CidrBlockState: &ec2.VpcCidrBlockState{
				State: aws.String("associated"),
			},
		},
		{
			AssociationId: aws.String("vpc-cidr-assoc-09754ecf421b0594d"),
			CidrBlock:     aws.String("10.223.194.160/27"),
			CidrBlockState: &ec2.VpcCidrBlockState{
				State: aws.String("associated"),
			},
		},
	}

	ExistingRouteTables := []*ec2.RouteTable{
		{
			RouteTableId: aws.String("rtb-0b5a867c9e6edeab5"),
			VpcId:        aws.String("vpc-01fe543d7926866a2"),
		},
		{
			RouteTableId: aws.String("rtb-00f9bc4f3d94a4bfd"),
			VpcId:        aws.String("vpc-01fe543d7926866a2"),
		},
		{
			RouteTableId: aws.String("rtb-03d2cb8dc4705458b"),
			VpcId:        aws.String("vpc-01fe543d7926866a2"),
		},
		{
			RouteTableId: aws.String("rtb-0fd857e2865283315"),
			VpcId:        aws.String("vpc-01fe543d7926866a2"),
		},
		{
			RouteTableId: aws.String("rtb-05d1170b896672605"),
			VpcId:        aws.String("vpc-01fe543d7926866a2"),
		},
		{
			RouteTableId: aws.String("rtb-0a2563f7179650875"),
			VpcId:        aws.String("vpc-01fe543d7926866a2"),
		},
	}

	tc := AZExpansionZonedSubnet{
		VPCName:               "alc-az-exp2-east-test",
		VPCID:                 "vpc-01fe543d7926866a2",
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
