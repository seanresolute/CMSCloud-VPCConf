package lib_test

import (
	"testing"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/lib"
	"github.com/go-test/deep"
)

type ObjectToMapTestCases struct {
	Name              string
	InputObject       interface{}
	ExpectedOutputMap map[string]interface{}
}

func TestObjectToMap(t *testing.T) {
	testCases := []*ObjectToMapTestCases{
		{
			Name:        "Simple string",
			InputObject: "Hello",
			ExpectedOutputMap: map[string]interface{}{
				"string": "Hello",
			},
		},
		{
			Name:        "uint64",
			InputObject: uint64(321),
			ExpectedOutputMap: map[string]interface{}{
				"uint64": uint64(321),
			},
		},
		{
			Name:        "Slice of strings",
			InputObject: []string{"a", "b", "c"},
			ExpectedOutputMap: map[string]interface{}{
				"slice": []string{"a", "b", "c"},
			},
		},
		{
			Name: "Security Group",
			InputObject: database.SecurityGroup{
				TemplateID:      1,
				SecurityGroupID: "sg-123",
				Rules: []*database.SecurityGroupRule{
					{
						Description: "SG123",
						IsEgress:    false,
						Protocol:    "tcp",
						FromPort:    1,
						ToPort:      65535,
						Source:      "1.2.3.4/32",
					},
				},
			},
			ExpectedOutputMap: map[string]interface{}{
				"TemplateID":      uint64(1),
				"SecurityGroupID": "sg-123",
				"Rules": []interface{}{
					map[string]interface{}{
						"Description":    "SG123",
						"IsEgress":       false,
						"Protocol":       "tcp",
						"FromPort":       int64(1),
						"ToPort":         int64(65535),
						"Source":         "1.2.3.4/32",
						"SourceIPV6CIDR": "",
					},
				},
			},
		},
		{
			Name: "Pointer To Security Group",
			InputObject: &database.SecurityGroup{
				TemplateID:      2,
				SecurityGroupID: "sg-321",
				Rules: []*database.SecurityGroupRule{
					{
						Description: "SG321",
						IsEgress:    true,
						Protocol:    "udp",
						FromPort:    1,
						ToPort:      65535,
						Source:      "1.2.3.4/32",
					},
				},
			},
			ExpectedOutputMap: map[string]interface{}{
				"TemplateID":      uint64(2),
				"SecurityGroupID": "sg-321",
				"Rules": []interface{}{
					map[string]interface{}{
						"Description":    "SG321",
						"IsEgress":       true,
						"Protocol":       "udp",
						"FromPort":       int64(1),
						"ToPort":         int64(65535),
						"Source":         "1.2.3.4/32",
						"SourceIPV6CIDR": "",
					},
				},
			},
		},
		{
			Name: "VPC State",
			InputObject: database.VPCState{
				VPCType:            database.VPCTypeV1,
				PublicRouteTableID: "rtb-123",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-321": {
						RouteTableID: "rt-321",
						Routes: []*database.RouteInfo{
							{
								Destination:      "9.8.7.6/32",
								TransitGatewayID: "tgw-123",
							},
						},
						SubnetType: database.SubnetTypeApp,
					},
				},
				InternetGateway: database.InternetGatewayInfo{
					InternetGatewayID:         "igw-123",
					IsInternetGatewayAttached: true,
				},
				AvailabilityZones: database.AZMap{
					"us-east-1a": &database.AvailabilityZoneInfra{
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							"Private": {
								{
									SubnetID:                "subnet-01234567",
									GroupName:               "private",
									RouteTableAssociationID: "rtbassoc-76543210",
								},
							},
						},
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "nat-123",
							EIPID:        "eipalloc-123",
						},
						PrivateRouteTableID: "rtb-0123",
					},
				},
				SecurityGroups: []*database.SecurityGroup{
					{
						TemplateID:      321,
						SecurityGroupID: "sg-321",
						Rules: []*database.SecurityGroupRule{
							{
								Description: "rds",
								IsEgress:    false,
								Protocol:    "-1",
								FromPort:    1024,
								ToPort:      1025,
								Source:      "1.2.3.4/32",
							},
						},
					},
				},
				S3FlowLogID:                     "fl-123",
				CloudWatchLogsFlowLogID:         "fl-987",
				ResolverQueryLogConfigurationID: "rqlc-123",
				ResolverQueryLogAssociationID:   "rqlca-321",
			},
			ExpectedOutputMap: map[string]interface{}{
				"VPCType":            database.VPCTypeV1,
				"PublicRouteTableID": "rtb-123",
				"RouteTables": map[string]interface{}{
					"rt-321": map[string]interface{}{
						"EdgeAssociationType": "",
						"RouteTableID":        "rt-321",
						"Routes": []interface{}{
							map[string]interface{}{
								"Destination":         "9.8.7.6/32",
								"InternetGatewayID":   "",
								"NATGatewayID":        "",
								"PeeringConnectionID": "",
								"TransitGatewayID":    "tgw-123",
								"VPCEndpointID":       "",
							},
						},
						"SubnetType": string(database.SubnetTypeApp),
					},
				},
				"InternetGateway": map[string]interface{}{
					"InternetGatewayID":         "igw-123",
					"IsInternetGatewayAttached": true,
					"RouteTableAssociationID":   "",
					"RouteTableID":              "",
				},
				"AvailabilityZones": map[string]interface{}{
					"us-east-1a": map[string]interface{}{
						"Subnets": map[string]interface{}{
							"Private": []interface{}{
								map[string]interface{}{
									"SubnetID":                "subnet-01234567",
									"GroupName":               "private",
									"RouteTableAssociationID": "rtbassoc-76543210",
									"CustomRouteTableID":      "",
								},
							},
						},
						"NATGateway": map[string]interface{}{
							"NATGatewayID": "nat-123",
							"EIPID":        "eipalloc-123",
						},
						"PrivateRouteTableID": "rtb-0123",
						"PublicRouteTableID":  "",
					},
				},
				"SecurityGroups": []interface{}{
					map[string]interface{}{
						"TemplateID":      uint64(321),
						"SecurityGroupID": "sg-321",
						"Rules": []interface{}{
							map[string]interface{}{
								"Description":    "rds",
								"IsEgress":       false,
								"Protocol":       "-1",
								"FromPort":       int64(1024),
								"ToPort":         int64(1025),
								"Source":         "1.2.3.4/32",
								"SourceIPV6CIDR": "",
							},
						},
					},
				},
				"CloudWatchLogsFlowLogID":         "fl-987",
				"ResolverQueryLogConfigurationID": "rqlc-123",
				"ResolverQueryLogAssociationID":   "rqlca-321",
				"S3FlowLogID":                     "fl-123",

				// Add all the empty objects
				"Firewall":                  map[string]interface{}{},
				"FirewallRouteTableID":      "",
				"PeeringConnections":        []interface{}{},
				"ResolverRuleAssociations":  []interface{}{},
				"TransitGatewayAttachments": []interface{}{},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			actualObject := lib.ObjectToMap(tc.InputObject)
			if diff := deep.Equal(tc.ExpectedOutputMap, actualObject); diff != nil {
				t.Error(diff)
			}
		})
	}
}
