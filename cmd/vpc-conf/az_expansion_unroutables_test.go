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

type AZExpansionUnroutables struct {
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

func TestPerformAZExpansionUnroutables(t *testing.T) {
	ExistingSubnetCIDRs := map[string]string{
		"subnet-0cdc7f1df250c7a40": "100.71.128.0/17",
		"subnet-0dc82a959cb474438": "10.147.51.96/27",
		"subnet-068e9cba2475f7327": "10.147.51.0/27",
		"subnet-0a0e4c6b3ffdfe5a5": "100.71.0.0/17",
		"subnet-0fd55eeef060855b0": "10.147.51.32/27",
		"subnet-0c6088378775339ee": "10.147.51.64/27",
	}

	startStateJson := `
{
 "VPCType": 0,
 "PublicRouteTableID": "rtb-09ede228ce9118404",
 "RouteTables": {
  "rtb-0250c15edd4546926": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-0976fdd4883a817e4",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Unroutable",
   "EdgeAssociationType": ""
  },
  "rtb-02d619af5c6a62d6a": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-08a2f06ddaf68537f",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-06ebb1a4f6a44dda5": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-0976fdd4883a817e4",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-09ede228ce9118404": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "",
     "InternetGatewayID": "igw-081fd9e23e4279d84",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Public",
   "EdgeAssociationType": ""
  },
  "rtb-0f08fc1fa4770d045": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-08a2f06ddaf68537f",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Unroutable",
   "EdgeAssociationType": ""
  }
 },
 "InternetGateway": {
  "InternetGatewayID": "igw-081fd9e23e4279d84",
  "IsInternetGatewayAttached": true,
  "RouteTableID": "",
  "RouteTableAssociationID": ""
 },
 "AvailabilityZones": {
  "us-east-1a": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-068e9cba2475f7327",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-083e2c1e75e05185d",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0c6088378775339ee",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-0b4a9802a114818d7",
      "CustomRouteTableID": ""
     }
    ],
    "Unroutable": [
     {
      "SubnetID": "subnet-0a0e4c6b3ffdfe5a5",
      "GroupName": "unroutable",
      "RouteTableAssociationID": "rtbassoc-0a9a975aabd1c1a6c",
      "CustomRouteTableID": "rtb-0f08fc1fa4770d045"
     }
    ]
   },
   "PrivateRouteTableID": "rtb-02d619af5c6a62d6a",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-08a2f06ddaf68537f",
    "EIPID": "eipalloc-02eed366cd4a42d02"
   }
  },
  "us-east-1b": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-0fd55eeef060855b0",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-0e51114c441bfbf89",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0dc82a959cb474438",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-084618e6405e08a59",
      "CustomRouteTableID": ""
     }
    ],
    "Unroutable": [
     {
      "SubnetID": "subnet-0cdc7f1df250c7a40",
      "GroupName": "unroutable",
      "RouteTableAssociationID": "rtbassoc-0fffbfe32828a6a0a",
      "CustomRouteTableID": "rtb-0250c15edd4546926"
     }
    ]
   },
   "PrivateRouteTableID": "rtb-06ebb1a4f6a44dda5",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-0976fdd4883a817e4",
    "EIPID": "eipalloc-019d3bfea3952f8d8"
   }
  }
 },
 "TransitGatewayAttachments": null,
 "ResolverRuleAssociations": [],
 "SecurityGroups": null,
 "S3FlowLogID": "fl-0cb5c5729ee16de9d",
 "CloudWatchLogsFlowLogID": "",
 "ResolverQueryLogConfigurationID": "rqlc-489173c498734c8f",
 "ResolverQueryLogAssociationID": "rqlca-f7c3d65cd03243ca",
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
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test",
     "ResourceID": "vpc-0e3a6546a97af3fc7",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test",
       "Address": "10.147.51.0",
       "BlockType": "VPC CIDR Block",
       "Size": 25,
       "Status": "Aggregate"
      }
     ],
     "Children": [
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/private-a",
       "ResourceID": "subnet-068e9cba2475f7327",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/private-a",
         "Address": "10.147.51.0",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/private-b",
       "ResourceID": "subnet-0fd55eeef060855b0",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/private-b",
         "Address": "10.147.51.32",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/public-a",
       "ResourceID": "subnet-0c6088378775339ee",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/public-a",
         "Address": "10.147.51.64",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/public-b",
       "ResourceID": "subnet-0dc82a959cb474438",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/public-b",
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
 ]
}
`

	endStateJson := `
{
 "VPCType": 0,
 "PublicRouteTableID": "rtb-09ede228ce9118404",
 "RouteTables": {
  "rtb-0250c15edd4546926": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-0976fdd4883a817e4",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Unroutable",
   "EdgeAssociationType": ""
  },
  "rtb-02d619af5c6a62d6a": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-08a2f06ddaf68537f",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-06ebb1a4f6a44dda5": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-0976fdd4883a817e4",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-09ede228ce9118404": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "",
     "InternetGatewayID": "igw-081fd9e23e4279d84",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Public",
   "EdgeAssociationType": ""
  },
  "rtb-0f08fc1fa4770d045": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-08a2f06ddaf68537f",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Unroutable",
   "EdgeAssociationType": ""
  }
 },
 "InternetGateway": {
  "InternetGatewayID": "igw-081fd9e23e4279d84",
  "IsInternetGatewayAttached": true,
  "RouteTableID": "",
  "RouteTableAssociationID": ""
 },
 "AvailabilityZones": {
  "us-east-1a": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-068e9cba2475f7327",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-083e2c1e75e05185d",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0c6088378775339ee",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-0b4a9802a114818d7",
      "CustomRouteTableID": ""
     }
    ],
    "Unroutable": [
     {
      "SubnetID": "subnet-0a0e4c6b3ffdfe5a5",
      "GroupName": "unroutable",
      "RouteTableAssociationID": "rtbassoc-0a9a975aabd1c1a6c",
      "CustomRouteTableID": "rtb-0f08fc1fa4770d045"
     }
    ]
   },
   "PrivateRouteTableID": "rtb-02d619af5c6a62d6a",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-08a2f06ddaf68537f",
    "EIPID": "eipalloc-02eed366cd4a42d02"
   }
  },
  "us-east-1b": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-0fd55eeef060855b0",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-0e51114c441bfbf89",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0dc82a959cb474438",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-084618e6405e08a59",
      "CustomRouteTableID": ""
     }
    ],
    "Unroutable": [
     {
      "SubnetID": "subnet-0cdc7f1df250c7a40",
      "GroupName": "unroutable",
      "RouteTableAssociationID": "rtbassoc-0fffbfe32828a6a0a",
      "CustomRouteTableID": "rtb-0250c15edd4546926"
     }
    ]
   },
   "PrivateRouteTableID": "rtb-06ebb1a4f6a44dda5",
   "PublicRouteTableID": "",
   "NATGateway": {
    "NATGatewayID": "nat-0976fdd4883a817e4",
    "EIPID": "eipalloc-019d3bfea3952f8d8"
   }
  },
  "us-east-1c": {
   "Subnets": {
    "Private": [
     {
      "SubnetID": "subnet-0d47ac3b4a324228e",
      "GroupName": "private",
      "RouteTableAssociationID": "",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-068e6a602fc3aaea6",
      "GroupName": "public",
      "RouteTableAssociationID": "",
      "CustomRouteTableID": ""
     }
    ],
    "Unroutable": [
     {
      "SubnetID": "subnet-0215d7a70bac433f8",
      "GroupName": "unroutable",
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
 "S3FlowLogID": "fl-0cb5c5729ee16de9d",
 "CloudWatchLogsFlowLogID": "",
 "ResolverQueryLogConfigurationID": "rqlc-489173c498734c8f",
 "ResolverQueryLogAssociationID": "rqlca-f7c3d65cd03243ca",
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
     "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test",
     "ResourceID": "vpc-0e3a6546a97af3fc7",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test",
       "Address": "10.147.51.0",
       "BlockType": "VPC CIDR Block",
       "Size": 25,
       "Status": "Aggregate"
      },
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test",
       "Address": "10.147.105.64",
       "BlockType": "VPC CIDR Block",
       "Size": 26,
       "Status": "Aggregate"
      }
     ],
     "Children": [
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/private-b",
       "ResourceID": "subnet-0fd55eeef060855b0",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/private-b",
         "Address": "10.147.51.32",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/public-a",
       "ResourceID": "subnet-0c6088378775339ee",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/public-a",
         "Address": "10.147.51.64",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/private-c",
       "ResourceID": "subnet-0d47ac3b4a324228e",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/private-c",
         "Address": "10.147.105.64",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/private-a",
       "ResourceID": "subnet-068e9cba2475f7327",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/private-a",
         "Address": "10.147.51.0",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/public-b",
       "ResourceID": "subnet-0dc82a959cb474438",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/public-b",
         "Address": "10.147.51.96",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/public-c",
       "ResourceID": "subnet-068e6a602fc3aaea6",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-az-exp3-east-test/public-c",
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

	endContainers := testmocks.ContainerTree{}
	err := json.Unmarshal([]byte(endContainersJson), &endContainers)
	if err != nil {
		fmt.Println(err)
	}

	preDefinedSubnetIDQueue := map[string][]string{
		"us-east-1c": {"subnet-0d47ac3b4a324228e", "subnet-068e6a602fc3aaea6", "subnet-0215d7a70bac433f8"},
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
		VPCID:  "vpc-0e3a6546a97af3fc7",
		Region: "us-east-1",
		AZName: "us-east-1c",
	}

	ExistingVPCCIDRBlocks := []*ec2.VpcCidrBlockAssociation{
		{
			AssociationId: aws.String("vpc-cidr-assoc-01965653be6df4d84"),
			CidrBlock:     aws.String("10.147.51.0/25"),
			CidrBlockState: &ec2.VpcCidrBlockState{
				State: aws.String("associated"),
			},
		},
		{
			AssociationId: aws.String("vpc-cidr-assoc-0f5669680b5689b9a"),
			CidrBlock:     aws.String("100.71.0.0/16"),
			CidrBlockState: &ec2.VpcCidrBlockState{
				State: aws.String("associated"),
			},
		},
	}

	ExistingRouteTables := []*ec2.RouteTable{
		{
			RouteTableId: aws.String("rtb-02d619af5c6a62d6a"),
			VpcId:        aws.String("vpc-0e3a6546a97af3fc7"),
		},
		{
			RouteTableId: aws.String("rtb-0250c15edd4546926"),
			VpcId:        aws.String("vpc-0e3a6546a97af3fc7"),
		},
		{
			RouteTableId: aws.String("rtb-06ebb1a4f6a44dda5"),
			VpcId:        aws.String("vpc-0e3a6546a97af3fc7"),
		},
		{
			RouteTableId: aws.String("rtb-02082e5e26ce4f5bf"),
			VpcId:        aws.String("vpc-0e3a6546a97af3fc7"),
		},
		{
			RouteTableId: aws.String("rtb-09ede228ce9118404"),
			VpcId:        aws.String("vpc-0e3a6546a97af3fc7"),
		},
		{
			RouteTableId: aws.String("rtb-0f08fc1fa4770d045"),
			VpcId:        aws.String("vpc-0e3a6546a97af3fc7"),
		},
	}

	tc := AZExpansionUnroutables{
		VPCName:                    "alc-az-exp3-east-test",
		VPCID:                      "vpc-0e3a6546a97af3fc7",
		Region:                     "us-east-1",
		Stack:                      "test",
		TaskConfig:                 TaskConfig,
		ExistingSubnetCIDRs:        ExistingSubnetCIDRs,
		ExistingVPCCIDRBlocks:      ExistingVPCCIDRBlocks,
		StartState:                 startState,
		ExistingContainers:         existingContainers,
		ExistingRouteTables:        ExistingRouteTables,
		ExistingPeeringConnections: []*ec2.VpcPeeringConnection{},
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
		//        PeeringConnections:      &tc.ExistingPeeringConnections,
		PrimaryCIDR:                            aws.String("10.147.51.0/25"),
		CIDRBlockAssociationSet:                tc.ExistingVPCCIDRBlocks,
		RouteTables:                            tc.ExistingRouteTables,
		SubnetCIDRs:                            tc.ExistingSubnetCIDRs,
		PeeringConnections:                     &tc.ExistingPeeringConnections,
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
