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

type AZExpansionFirewall struct {
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

func TestPerformAZExpansionFirewall(t *testing.T) {
	ExistingSubnetCIDRs := map[string]string{
		"subnet-09e7d2f3ff2fef14f": "10.231.228.192/27",
		"subnet-0f85a5ddeba30ccf9": "10.231.247.64/28",
		"subnet-0b26f71abca11e60e": "10.231.247.80/28",
		"subnet-01fa46b36c8035f18": "10.231.228.224/27",
		"subnet-08e39cd42435f00df": "10.231.228.128/27",
		"subnet-06cc995d7d692ca6a": "10.231.228.160/27",
	}

	startStateJson := `
{
 "VPCType": 3,
 "PublicRouteTableID": "",
 "RouteTables": {
  "rtb-018958950da4fb1e2": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": "vpce-0e5f4d8bdf838a577"
    }
   ],
   "SubnetType": "Public",
   "EdgeAssociationType": ""
  },
  "rtb-061b59ed52977561f": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "",
     "InternetGatewayID": "igw-0b6d4227e088f4946",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Firewall",
   "EdgeAssociationType": ""
  },
  "rtb-09bcdc2d740a57b52": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-043e8b3f91e046f83",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-0c7cda6e3a0587396": {
   "Routes": [
    {
     "Destination": "10.231.228.192/27",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": "vpce-0e5f4d8bdf838a577"
    },
    {
     "Destination": "10.231.228.224/27",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": "vpce-006ecf4d8f856c97b"
    }
   ],
   "SubnetType": "",
   "EdgeAssociationType": "IGW"
  },
  "rtb-0e1ee800efe859ab4": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-0520aea5bcb25420f",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-0e32a25310ffd51cc": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": "vpce-006ecf4d8f856c97b"
    }
   ],
   "SubnetType": "Public",
   "EdgeAssociationType": ""
  }
 },
 "InternetGateway": {
  "InternetGatewayID": "igw-0b6d4227e088f4946",
  "IsInternetGatewayAttached": true,
  "RouteTableID": "rtb-0c7cda6e3a0587396",
  "RouteTableAssociationID": "rtbassoc-00ba4c09acf88ec56"
 },
 "AvailabilityZones": {
  "us-west-2a": {
   "Subnets": {
    "Firewall": [
     {
      "SubnetID": "subnet-0f85a5ddeba30ccf9",
      "GroupName": "firewall",
      "RouteTableAssociationID": "rtbassoc-08312adbe8251056a",
      "CustomRouteTableID": ""
     }
    ],
    "Private": [
     {
      "SubnetID": "subnet-08e39cd42435f00df",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-06b80dfb60beb7872",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-09e7d2f3ff2fef14f",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-0d7a0a8c598a38fab",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-0e1ee800efe859ab4",
   "PublicRouteTableID": "rtb-018958950da4fb1e2",
   "NATGateway": {
    "NATGatewayID": "nat-0520aea5bcb25420f",
    "EIPID": "eipalloc-07787869572df45ac"
   }
  },
  "us-west-2b": {
   "Subnets": {
    "Firewall": [
     {
      "SubnetID": "subnet-0b26f71abca11e60e",
      "GroupName": "firewall",
      "RouteTableAssociationID": "rtbassoc-0d878dff012a1057c",
      "CustomRouteTableID": ""
     }
    ],
    "Private": [
     {
      "SubnetID": "subnet-06cc995d7d692ca6a",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-0568f818e2cb1c312",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-01fa46b36c8035f18",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-0264e12b269801fff",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-09bcdc2d740a57b52",
   "PublicRouteTableID": "rtb-0e32a25310ffd51cc",
   "NATGateway": {
    "NATGatewayID": "nat-043e8b3f91e046f83",
    "EIPID": "eipalloc-0b8da663eb850aa2a"
   }
  }
 },
 "TransitGatewayAttachments": null,
 "ResolverRuleAssociations": [],
 "SecurityGroups": null,
 "S3FlowLogID": "fl-08fecfcf5d1cd2e9e",
 "CloudWatchLogsFlowLogID": "fl-08d6d024368a7782d",
 "ResolverQueryLogConfigurationID": "rqlc-cb9a2c87b3504973",
 "ResolverQueryLogAssociationID": "rqlca-22d87c6dfb7f47d2",
 "Firewall": {
  "AssociatedSubnetIDs": [
   "subnet-0f85a5ddeba30ccf9",
   "subnet-0b26f71abca11e60e"
  ]
 },
 "FirewallRouteTableID": "rtb-061b59ed52977561f"
}
`

	existingContainersJson := `
{
 "Name": "/Global/AWS/V4/Commercial/West",
 "ResourceID": "",
 "Blocks": null,
 "Children": [
  {
   "Name": "/Global/AWS/V4/Commercial/West/Production",
   "ResourceID": "",
   "Blocks": [
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.150.14.0",
     "BlockType": "Environment CIDR Block",
     "Size": 23,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.150.16.0",
     "BlockType": "Environment CIDR Block",
     "Size": 20,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.150.32.0",
     "BlockType": "Environment CIDR Block",
     "Size": 19,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.150.64.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.150.128.0",
     "BlockType": "Environment CIDR Block",
     "Size": 17,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.231.228.64",
     "BlockType": "Environment CIDR Block",
     "Size": 26,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.231.245.112",
     "BlockType": "Environment CIDR Block",
     "Size": 28,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.231.250.160",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.231.254.0",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.150.0.0",
     "BlockType": "Environment CIDR Block",
     "Size": 16,
     "Status": "Aggregate"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.231.192.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Aggregate"
    }
   ],
   "Children": [
    {
     "Name": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod",
     "ResourceID": "vpc-0b035a87615f4a4e5",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod",
       "Address": "10.231.228.128",
       "BlockType": "VPC CIDR Block",
       "Size": 25,
       "Status": "Aggregate"
      },
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod",
       "Address": "10.231.247.64",
       "BlockType": "VPC CIDR Block",
       "Size": 27,
       "Status": "Aggregate"
      }
     ],
     "Children": [
      {
       "Name": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/firewall-a",
       "ResourceID": "subnet-0f85a5ddeba30ccf9",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/firewall-a",
         "Address": "10.231.247.64",
         "BlockType": "Subnet CIDR Block",
         "Size": 28,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/firewall-b",
       "ResourceID": "subnet-0b26f71abca11e60e",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/firewall-b",
         "Address": "10.231.247.80",
         "BlockType": "Subnet CIDR Block",
         "Size": 28,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/private-a",
       "ResourceID": "subnet-08e39cd42435f00df",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/private-a",
         "Address": "10.231.228.128",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/private-b",
       "ResourceID": "subnet-06cc995d7d692ca6a",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/private-b",
         "Address": "10.231.228.160",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/public-b",
       "ResourceID": "subnet-01fa46b36c8035f18",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/public-b",
         "Address": "10.231.228.224",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/public-a",
       "ResourceID": "subnet-09e7d2f3ff2fef14f",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/public-a",
         "Address": "10.231.228.192",
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
 "VPCType": 3,
 "PublicRouteTableID": "",
 "RouteTables": {
  "rtb-018958950da4fb1e2": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": "vpce-0e5f4d8bdf838a577"
    }
   ],
   "SubnetType": "Public",
   "EdgeAssociationType": ""
  },
  "rtb-061b59ed52977561f": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "",
     "InternetGatewayID": "igw-0b6d4227e088f4946",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Firewall",
   "EdgeAssociationType": ""
  },
  "rtb-09bcdc2d740a57b52": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-043e8b3f91e046f83",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-0c7cda6e3a0587396": {
   "Routes": [
    {
     "Destination": "10.231.228.192/27",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": "vpce-0e5f4d8bdf838a577"
    },
    {
     "Destination": "10.231.228.224/27",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": "vpce-006ecf4d8f856c97b"
    }
   ],
   "SubnetType": "",
   "EdgeAssociationType": "IGW"
  },
  "rtb-0e1ee800efe859ab4": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "nat-0520aea5bcb25420f",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": ""
    }
   ],
   "SubnetType": "Private",
   "EdgeAssociationType": ""
  },
  "rtb-0e32a25310ffd51cc": {
   "Routes": [
    {
     "Destination": "0.0.0.0/0",
     "NATGatewayID": "",
     "InternetGatewayID": "",
     "TransitGatewayID": "",
     "PeeringConnectionID": "",
     "VPCEndpointID": "vpce-006ecf4d8f856c97b"
    }
   ],
   "SubnetType": "Public",
   "EdgeAssociationType": ""
  }
 },
 "InternetGateway": {
  "InternetGatewayID": "igw-0b6d4227e088f4946",
  "IsInternetGatewayAttached": true,
  "RouteTableID": "rtb-0c7cda6e3a0587396",
  "RouteTableAssociationID": "rtbassoc-00ba4c09acf88ec56"
 },
 "AvailabilityZones": {
  "us-west-2a": {
   "Subnets": {
    "Firewall": [
     {
      "SubnetID": "subnet-0f85a5ddeba30ccf9",
      "GroupName": "firewall",
      "RouteTableAssociationID": "rtbassoc-08312adbe8251056a",
      "CustomRouteTableID": ""
     }
    ],
    "Private": [
     {
      "SubnetID": "subnet-08e39cd42435f00df",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-06b80dfb60beb7872",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-09e7d2f3ff2fef14f",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-0d7a0a8c598a38fab",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-0e1ee800efe859ab4",
   "PublicRouteTableID": "rtb-018958950da4fb1e2",
   "NATGateway": {
    "NATGatewayID": "nat-0520aea5bcb25420f",
    "EIPID": "eipalloc-07787869572df45ac"
   }
  },
  "us-west-2b": {
   "Subnets": {
    "Firewall": [
     {
      "SubnetID": "subnet-0b26f71abca11e60e",
      "GroupName": "firewall",
      "RouteTableAssociationID": "rtbassoc-0d878dff012a1057c",
      "CustomRouteTableID": ""
     }
    ],
    "Private": [
     {
      "SubnetID": "subnet-06cc995d7d692ca6a",
      "GroupName": "private",
      "RouteTableAssociationID": "rtbassoc-0568f818e2cb1c312",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-01fa46b36c8035f18",
      "GroupName": "public",
      "RouteTableAssociationID": "rtbassoc-0264e12b269801fff",
      "CustomRouteTableID": ""
     }
    ]
   },
   "PrivateRouteTableID": "rtb-09bcdc2d740a57b52",
   "PublicRouteTableID": "rtb-0e32a25310ffd51cc",
   "NATGateway": {
    "NATGatewayID": "nat-043e8b3f91e046f83",
    "EIPID": "eipalloc-0b8da663eb850aa2a"
   }
  },
  "us-west-2c": {
   "Subnets": {
    "Firewall": [
     {
      "SubnetID": "subnet-01c8ce983455ccc3b",
      "GroupName": "firewall",
      "RouteTableAssociationID": "",
      "CustomRouteTableID": ""
     }
    ],
    "Private": [
     {
      "SubnetID": "subnet-0efc51e7e00943190",
      "GroupName": "private",
      "RouteTableAssociationID": "",
      "CustomRouteTableID": ""
     }
    ],
    "Public": [
     {
      "SubnetID": "subnet-0e91f79a3c1870509",
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
 "S3FlowLogID": "fl-08fecfcf5d1cd2e9e",
 "CloudWatchLogsFlowLogID": "fl-08d6d024368a7782d",
 "ResolverQueryLogConfigurationID": "rqlc-cb9a2c87b3504973",
 "ResolverQueryLogAssociationID": "rqlca-22d87c6dfb7f47d2",
 "Firewall": {
  "AssociatedSubnetIDs": [
   "subnet-0f85a5ddeba30ccf9",
   "subnet-0b26f71abca11e60e"
  ]
 },
 "FirewallRouteTableID": "rtb-061b59ed52977561f"
}
`

	endContainersJson := `
{
 "Name": "/Global/AWS/V4/Commercial/West",
 "ResourceID": "",
 "Blocks": null,
 "Children": [
  {
   "Name": "/Global/AWS/V4/Commercial/West/Production",
   "ResourceID": "",
   "Blocks": [
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.150.14.0",
     "BlockType": "Environment CIDR Block",
     "Size": 23,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.150.16.0",
     "BlockType": "Environment CIDR Block",
     "Size": 20,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.150.32.0",
     "BlockType": "Environment CIDR Block",
     "Size": 19,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.150.64.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.150.128.0",
     "BlockType": "Environment CIDR Block",
     "Size": 17,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.231.250.160",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.231.254.0",
     "BlockType": "Environment CIDR Block",
     "Size": 27,
     "Status": "Free"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.150.0.0",
     "BlockType": "Environment CIDR Block",
     "Size": 16,
     "Status": "Aggregate"
    },
    {
     "ParentContainer": "",
     "Container": "/Global/AWS/V4/Commercial/West/Production",
     "Address": "10.231.192.0",
     "BlockType": "Environment CIDR Block",
     "Size": 18,
     "Status": "Aggregate"
    }
   ],
   "Children": [
    {
     "Name": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod",
     "ResourceID": "vpc-0b035a87615f4a4e5",
     "Blocks": [
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod",
       "Address": "10.231.228.64",
       "BlockType": "VPC CIDR Block",
       "Size": 26,
       "Status": "Aggregate"
      },
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod",
       "Address": "10.231.228.128",
       "BlockType": "VPC CIDR Block",
       "Size": 25,
       "Status": "Aggregate"
      },
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod",
       "Address": "10.231.245.112",
       "BlockType": "VPC CIDR Block",
       "Size": 28,
       "Status": "Aggregate"
      },
      {
       "ParentContainer": "",
       "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod",
       "Address": "10.231.247.64",
       "BlockType": "VPC CIDR Block",
       "Size": 27,
       "Status": "Aggregate"
      }
     ],
     "Children": [
      {
       "Name": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/public-b",
       "ResourceID": "subnet-01fa46b36c8035f18",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/public-b",
         "Address": "10.231.228.224",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/firewall-b",
       "ResourceID": "subnet-0b26f71abca11e60e",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/firewall-b",
         "Address": "10.231.247.80",
         "BlockType": "Subnet CIDR Block",
         "Size": 28,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/firewall-c",
       "ResourceID": "subnet-01c8ce983455ccc3b",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/firewall-c",
         "Address": "10.231.245.112",
         "BlockType": "Subnet CIDR Block",
         "Size": 28,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/private-a",
       "ResourceID": "subnet-08e39cd42435f00df",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/private-a",
         "Address": "10.231.228.128",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/private-b",
       "ResourceID": "subnet-06cc995d7d692ca6a",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/private-b",
         "Address": "10.231.228.160",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/public-a",
       "ResourceID": "subnet-09e7d2f3ff2fef14f",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/public-a",
         "Address": "10.231.228.192",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/firewall-a",
       "ResourceID": "subnet-0f85a5ddeba30ccf9",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/firewall-a",
         "Address": "10.231.247.64",
         "BlockType": "Subnet CIDR Block",
         "Size": 28,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/private-c",
       "ResourceID": "subnet-0efc51e7e00943190",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/private-c",
         "Address": "10.231.228.64",
         "BlockType": "Subnet CIDR Block",
         "Size": 27,
         "Status": "Deployed"
        }
       ],
       "Children": null
      },
      {
       "Name": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/public-c",
       "ResourceID": "subnet-0e91f79a3c1870509",
       "Blocks": [
        {
         "ParentContainer": "",
         "Container": "/Global/AWS/V4/Commercial/West/Production/921617238787-alc-test-west-prod/public-c",
         "Address": "10.231.228.96",
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
		"us-west-2c": {"subnet-0efc51e7e00943190", "subnet-0e91f79a3c1870509", "subnet-01c8ce983455ccc3b"},
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
		VPCID:  "vpc-0b035a87615f4a4e5",
		Region: "us-west-2",
		AZName: "us-west-2c",
	}

	ExistingVPCCIDRBlocks := []*ec2.VpcCidrBlockAssociation{
		{
			AssociationId: aws.String("vpc-cidr-assoc-0092dcfee4880b1db"),
			CidrBlock:     aws.String("10.231.228.128/25"),
			CidrBlockState: &ec2.VpcCidrBlockState{
				State: aws.String("associated"),
			},
		},
		{
			AssociationId: aws.String("vpc-cidr-assoc-0eb66c3d1d37a5af4"),
			CidrBlock:     aws.String("10.231.247.64/27"),
			CidrBlockState: &ec2.VpcCidrBlockState{
				State: aws.String("associated"),
			},
		},
	}

	ExistingRouteTables := []*ec2.RouteTable{
		{
			RouteTableId: aws.String("rtb-0b576afad67cf2225"),
			VpcId:        aws.String("vpc-0b035a87615f4a4e5"),
		},
		{
			RouteTableId: aws.String("rtb-0c7cda6e3a0587396"),
			VpcId:        aws.String("vpc-0b035a87615f4a4e5"),
		},
		{
			RouteTableId: aws.String("rtb-0e32a25310ffd51cc"),
			VpcId:        aws.String("vpc-0b035a87615f4a4e5"),
		},
		{
			RouteTableId: aws.String("rtb-061b59ed52977561f"),
			VpcId:        aws.String("vpc-0b035a87615f4a4e5"),
		},
		{
			RouteTableId: aws.String("rtb-09bcdc2d740a57b52"),
			VpcId:        aws.String("vpc-0b035a87615f4a4e5"),
		},
		{
			RouteTableId: aws.String("rtb-018958950da4fb1e2"),
			VpcId:        aws.String("vpc-0b035a87615f4a4e5"),
		},
		{
			RouteTableId: aws.String("rtb-0e1ee800efe859ab4"),
			VpcId:        aws.String("vpc-0b035a87615f4a4e5"),
		},
	}

	ExistingPeeringConnections := []*ec2.VpcPeeringConnection{}

	tc := AZExpansionFirewall{
		VPCName:                    "alc-test-west-prod",
		VPCID:                      "vpc-0b035a87615f4a4e5",
		Region:                     "us-west-2",
		Stack:                      "prod",
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
				AccountID: "921617238787",
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
		PrimaryCIDR:                            aws.String("10.231.228.128/25"),
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
