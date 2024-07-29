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

type AZExpansionSplitContainer struct {
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

func TestPerformAZExpansionSplitContainer(t *testing.T) {
	ExistingSubnetCIDRs := map[string]string{
		"subnet-0d0bb1f452aa74a25": "10.242.38.128/25",
		"subnet-0ae1da9f2e29b525e": "10.242.38.0/25",
		"subnet-0273fbcc71fec6efa": "10.147.135.0/27",
		"subnet-077c0de2fa41b1e5f": "10.147.135.32/27",
	}

	startStateJson := `
{
 "VPCType": 0,
 "PublicRouteTableID": "rtb-072af189bd65f4389",
 "RouteTables": {
  "rtb-0403a13821efadec5": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-080b52f6a6d3d2e8e",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-072af189bd65f4389": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "",
     "InternetGatewayID": "igw-0f74c01f18a5d2fe6",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Public",
   "EdgeAssociationType": ""
  },
  "rtb-0ac72ee67888985c2": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-07af11f9db6bb2512",
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
  "InternetGatewayID": "igw-0f74c01f18a5d2fe6",
  "IsInternetGatewayAttached": true,
  "RouteTableID": "",
  "RouteTableAssociationID": ""
 },
 "AvailabilityZones": {
  "us-east-1a": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-0ae1da9f2e29b525e",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-0b7307895746ec784",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0273fbcc71fec6efa",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-0fee6c9f722539d0f",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-0403a13821efadec5",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-080b52f6a6d3d2e8e",
    "EIPID": "eipalloc-0db35392cb2e7ae2a"
   }
  },
  "us-east-1b": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-0d0bb1f452aa74a25",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-0476f3ef270c75b75",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-077c0de2fa41b1e5f",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-02dcb2a61ea67f1e4",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-0ac72ee67888985c2",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-07af11f9db6bb2512",
    "EIPID": "eipalloc-0bf4dba758146a8a2"
   }
  }
 },
 "TransitGatewayAttachments": null,
 "ResolverRuleAssociations": [],
 "SecurityGroups": null,
 "S3FlowLogID": "fl-01ee47280b178daef",
 "CloudWatchLogsFlowLogID": "",
 "ResolverQueryLogConfigurationID": "rqlc-52e1d9e3061a4431",
 "ResolverQueryLogAssociationID": "rqlca-839248d4ccaa4385",
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
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test",
     "ResourceID": "vpc-0d85a0c0ad4cdab3f",
     "Blocks": null,
     "Children": [
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/private",
       "ResourceID": "",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/private",
         "Address": "10.242.38.0",
         "BlockType": "VPC CIDR Block",
         "Size": 24,
         "Status": "Aggregate"
        }
       ],
       "Children": [
        {
         "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/private/private-b",
         "ResourceID": "subnet-0d0bb1f452aa74a25",
         "Blocks": [
          {
           "ParentContainer": "",
           "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/private/private-b",
           "Address": "10.242.38.128",
           "BlockType": "Subnet CIDR Block",
           "Size": 25,
           "Status": "Deployed"
          }
         ],
         "Children": null
        },
        {
         "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/private/private-a",
         "ResourceID": "subnet-0ae1da9f2e29b525e",
         "Blocks": [
          {
           "ParentContainer": "",
           "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/private/private-a",
           "Address": "10.242.38.0",
           "BlockType": "Subnet CIDR Block",
           "Size": 25,
           "Status": "Deployed"
          }
         ],
         "Children": null
        }
       ]
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/public",
       "ResourceID": "",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/public",
         "Address": "10.147.135.0",
         "BlockType": "VPC CIDR Block",
         "Size": 26,
         "Status": "Aggregate"
        }
       ],
       "Children": [
        {
         "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/public/public-b",
         "ResourceID": "subnet-077c0de2fa41b1e5f",
         "Blocks": [
          {
           "ParentContainer": "",
           "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/public/public-b",
           "Address": "10.147.135.32",
           "BlockType": "Subnet CIDR Block",
           "Size": 27,
           "Status": "Deployed"
          }
         ],
         "Children": null
        },
        {
         "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/public/public-a",
         "ResourceID": "subnet-0273fbcc71fec6efa",
         "Blocks": [
          {
           "ParentContainer": "",
           "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/public/public-a",
           "Address": "10.147.135.0",
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
 ]
}
`

	endStateJson := `
{
 "VPCType": 0,
 "PublicRouteTableID": "rtb-072af189bd65f4389",
 "RouteTables": {
  "rtb-0403a13821efadec5": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-080b52f6a6d3d2e8e",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-072af189bd65f4389": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "",
     "InternetGatewayID": "igw-0f74c01f18a5d2fe6",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Public",
   "EdgeAssociationType": ""
  },
  "rtb-0ac72ee67888985c2": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-07af11f9db6bb2512",
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
  "InternetGatewayID": "igw-0f74c01f18a5d2fe6",
  "IsInternetGatewayAttached": true,
  "RouteTableID": "",
  "RouteTableAssociationID": ""
 },
 "AvailabilityZones": {
  "us-east-1a": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-0ae1da9f2e29b525e",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-0b7307895746ec784",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0273fbcc71fec6efa",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-0fee6c9f722539d0f",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-0403a13821efadec5",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-080b52f6a6d3d2e8e",
    "EIPID": "eipalloc-0db35392cb2e7ae2a"
   }
  },
  "us-east-1b": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-0d0bb1f452aa74a25",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-0476f3ef270c75b75",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-077c0de2fa41b1e5f",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-02dcb2a61ea67f1e4",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-0ac72ee67888985c2",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-07af11f9db6bb2512",
    "EIPID": "eipalloc-0bf4dba758146a8a2"
   }
  },
  "us-east-1f": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-0915aabf160ff104f",
      "GroupName": "private",
      "RouteTableAssociationID": "",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-04ec953d05445f9d0",
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
 "S3FlowLogID": "fl-01ee47280b178daef",
 "CloudWatchLogsFlowLogID": "",
 "ResolverQueryLogConfigurationID": "rqlc-52e1d9e3061a4431",
 "ResolverQueryLogAssociationID": "rqlca-839248d4ccaa4385",
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
     "Address": "10.203.99.0",
     "BlockType": "Environment CIDR Block",
     "Size": 24,
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
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test",
     "ResourceID": "vpc-0d85a0c0ad4cdab3f",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test",
       "Address": "10.203.98.32",
       "BlockType": "VPC CIDR Block",
       "Size": 27,
       "Status": "Free"
      },
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test",
       "Address": "10.203.98.64",
       "BlockType": "VPC CIDR Block",
       "Size": 26,
       "Status": "Free"
      },
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test",
       "Address": "10.203.98.0",
       "BlockType": "VPC CIDR Block",
       "Size": 24,
       "Status": "Aggregate"
      }
     ],
     "Children": [
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/private",
       "ResourceID": "",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/private",
         "Address": "10.242.38.0",
         "BlockType": "VPC CIDR Block",
         "Size": 24,
         "Status": "Aggregate"
        }
       ],
       "Children": [
        {
         "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/private/private-a",
         "ResourceID": "subnet-0ae1da9f2e29b525e",
         "Blocks": [
          {
           "ParentContainer": "",
           "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/private/private-a",
           "Address": "10.242.38.0",
           "BlockType": "Subnet CIDR Block",
           "Size": 25,
           "Status": "Deployed"
          }
         ],
         "Children": null
        },
        {
         "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/private/private-b",
         "ResourceID": "subnet-0d0bb1f452aa74a25",
         "Blocks": [
          {
           "ParentContainer": "",
           "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/private/private-b",
           "Address": "10.242.38.128",
           "BlockType": "Subnet CIDR Block",
           "Size": 25,
           "Status": "Deployed"
          }
         ],
         "Children": null
        }
       ]
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/public",
       "ResourceID": "",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/public",
         "Address": "10.147.135.0",
         "BlockType": "VPC CIDR Block",
         "Size": 26,
         "Status": "Aggregate"
        }
       ],
       "Children": [
        {
         "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/public/public-a",
         "ResourceID": "subnet-0273fbcc71fec6efa",
         "Blocks": [
          {
           "ParentContainer": "",
           "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/public/public-a",
           "Address": "10.147.135.0",
           "BlockType": "Subnet CIDR Block",
           "Size": 27,
           "Status": "Deployed"
          }
         ],
         "Children": null
        },
        {
         "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/public/public-b",
         "ResourceID": "subnet-077c0de2fa41b1e5f",
         "Blocks": [
          {
           "ParentContainer": "",
           "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/public/public-b",
           "Address": "10.147.135.32",
           "BlockType": "Subnet CIDR Block",
           "Size": 27,
           "Status": "Deployed"
          }
         ],
         "Children": null
        }
       ]
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/public-f",
       "ResourceID": "subnet-04ec953d05445f9d0",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/public-f",
         "Address": "10.203.98.0",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/private-f",
       "ResourceID": "subnet-0915aabf160ff104f",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp7-east-test-east-test/private-f",
         "Address": "10.203.98.128",
         "BlockType": "Subnet CIDR Block",
         "Size": 25,
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
		"us-east-1f": {"subnet-04ec953d05445f9d0", "subnet-0915aabf160ff104f"},
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
		VPCID:  "vpc-0d85a0c0ad4cdab3f",
		Region: "us-east-1",
		AZName: "us-east-1f",
	}

	ExistingVPCCIDRBlocks := []*ec2.VpcCidrBlockAssociation{
		{
			AssociationId: aws.String("vpc-cidr-assoc-072b2b84d83f8fbfd"),
			CidrBlock:     aws.String("10.242.38.0/24"),
			CidrBlockState: &ec2.VpcCidrBlockState{
				State: aws.String("associated"),
			},
		},
		{
			AssociationId: aws.String("vpc-cidr-assoc-0d24675ff3baf5290"),
			CidrBlock:     aws.String("10.147.135.0/26"),
			CidrBlockState: &ec2.VpcCidrBlockState{
				State: aws.String("associated"),
			},
		},
	}

	ExistingRouteTables := []*ec2.RouteTable{
		{
			RouteTableId: aws.String("rtb-0dfa53252c45e9608"),
			VpcId:        aws.String("vpc-0d85a0c0ad4cdab3f"),
		},
		{
			RouteTableId: aws.String("rtb-0ac72ee67888985c2"),
			VpcId:        aws.String("vpc-0d85a0c0ad4cdab3f"),
		},
		{
			RouteTableId: aws.String("rtb-072af189bd65f4389"),
			VpcId:        aws.String("vpc-0d85a0c0ad4cdab3f"),
		},
		{
			RouteTableId: aws.String("rtb-0403a13821efadec5"),
			VpcId:        aws.String("vpc-0d85a0c0ad4cdab3f"),
		},
	}

	ExistingPeeringConnections := []*ec2.VpcPeeringConnection{}

	tc := AZExpansionSplitContainer{
		VPCName:                    "alc-az-exp7-east-test-east-test",
		VPCID:                      "vpc-0d85a0c0ad4cdab3f",
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
		},
	}

	ipcontrol := &testmocks.MockIPControl{
		ExistingContainers: tc.ExistingContainers,
		BlocksDeleted:      []string{},
	}

	ec2 := &testmocks.MockEC2{
		PeeringConnections:                     &tc.ExistingPeeringConnections,
		PrimaryCIDR:                            aws.String("10.242.38.0/24"),
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
