package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/jmoiron/sqlx"
)

type migration interface {
	f(*sqlx.Tx) error
}

// A migration that is just a list of SQL statements to perform
type staticMigration []string

func (m *staticMigration) f(tx *sqlx.Tx) error {
	for _, sql := range *m {
		_, err := tx.Exec(sql)
		if err != nil {
			return err
		}
	}
	return nil
}

type customMigration func(*sqlx.Tx) error

func (m customMigration) f(tx *sqlx.Tx) error {
	return m(tx)
}

func Migrate(db *sqlx.DB) error {
	// First make sure the migration table is present.
	rows, err := db.Query("SELECT 1 FROM information_schema.tables WHERE table_name = 'migration'")
	if err != nil {
		panic(err)
	}
	if !rows.Next() {
		sql := `
			CREATE TABLE migration (
				id serial PRIMARY KEY,
				index integer,
				applied_at timestamp with time zone DEFAULT current_timestamp
			)`
		_, err := db.Exec(sql)
		if err != nil {
			return err
		}
	}

	// Now perform any necessary migrations.
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	_, err = tx.Exec("LOCK TABLE migration")
	if err != nil {
		return err
	}

	var result *int
	err = tx.Get(&result, "SELECT MAX(index) FROM migration")
	if err != nil {
		return err
	}
	maxIndex := -1
	if result != nil { // nil indicates no migrations yet
		maxIndex = *result
	}

	migrations := allMigrations()
	for index := maxIndex + 1; index < len(migrations); index++ {
		log.Printf("Performing migration %d", index)
		err := migrations[index].f(tx)
		if err != nil {
			return err
		}
		_, err = tx.Exec("INSERT INTO migration (index) VALUES ($1)", index)
		if err != nil {
			return err
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}
	committed = true

	return nil
}

func allMigrations() []migration {
	return []migration{
		&staticMigration{
			`CREATE TABLE aws_account (
				id serial PRIMARY KEY,
				aws_id TEXT NOT NULL,
				name TEXT,
				is_gov_cloud boolean,
				UNIQUE (aws_id)
			)`,
			`CREATE TABLE session (
				id serial PRIMARY KEY,
				key bytea NOT NULL,
				created_at timestamp with time zone DEFAULT current_timestamp,
				cloud_tamer_token TEXT,
				UNIQUE (key)
			)`,
			`CREATE INDEX session_idx_created_at ON session (created_at)`,
			`CREATE TABLE session_aws_account (
				session_id integer REFERENCES session(id) NOT NULL,
				aws_account_id integer REFERENCES aws_account(id) NOT NULL,
				access_key_id TEXT,
				secret_access_key TEXT,
				session_token TEXT,
				PRIMARY KEY(session_id, aws_account_id)
			)`,
		},
		&staticMigration{
			`CREATE TABLE vpc (
				id serial PRIMARY KEY,
				aws_account_id integer REFERENCES aws_account(id) NOT NULL,
				aws_region TEXT NOT NULL,
				aws_id TEXT NOT NULL,
				UNIQUE (aws_region, aws_id)
			)`,
			`CREATE TABLE task (
				id serial PRIMARY KEY,
				aws_account_id integer REFERENCES aws_account(id),
				vpc_id integer REFERENCES vpc(id),
				description TEXT NOT NULL,
				status INTEGER NOT NULL,
				data jsonb NOT NULL,
				added_at timestamp with time zone DEFAULT current_timestamp
				CHECK(
					(aws_account_id IS NOT NULL AND vpc_id IS NULL)
					OR (aws_account_id IS NULL AND vpc_id IS NOT NULL)
				)
			)`,
			`CREATE TABLE task_log (
				id serial PRIMARY KEY,
				task_id integer REFERENCES task(id),
				added_at timestamp with time zone DEFAULT current_timestamp,
				message TEXT NOT NULL
			)`,
		},
		&staticMigration{
			`ALTER TABLE session_aws_account DROP CONSTRAINT session_aws_account_session_id_fkey`,
			`ALTER TABLE session_aws_account ADD FOREIGN KEY (session_id) REFERENCES session(id) ON DELETE CASCADE`,
		},
		&staticMigration{
			`CREATE TABLE task_reservation (
				id serial PRIMARY KEY,
				task_id integer REFERENCES task(id),
				reserved_by TEXT NOT NULL,
				reserved_at timestamp with time zone DEFAULT current_timestamp
			)`,
		},
		&staticMigration{
			`ALTER TABLE aws_account ALTER COLUMN name SET NOT NULL`,
			`ALTER TABLE aws_account ALTER COLUMN is_gov_cloud SET NOT NULL`,
			`ALTER TABLE migration ALTER COLUMN index SET NOT NULL`,
			`ALTER TABLE migration ALTER COLUMN applied_at SET NOT NULL`,
			`ALTER TABLE session ALTER COLUMN created_at SET NOT NULL`,
			`ALTER TABLE task ALTER COLUMN added_at SET NOT NULL`,
			`ALTER TABLE task_reservation ALTER COLUMN task_id SET NOT NULL`,
			`ALTER TABLE task_reservation ALTER COLUMN reserved_at SET NOT NULL`,
		},
		&staticMigration{
			`ALTER TABLE vpc ADD COLUMN state jsonb NULL`,
			`ALTER TABLE vpc ADD COLUMN name text NULL`,
			`UPDATE vpc SET name=aws_id`,
			`ALTER TABLE vpc ALTER COLUMN name SET NOT NULL`,
			`ALTER TABLE vpc ADD COLUMN is_deleted bool NOT NULL DEFAULT false`,
		},
		&staticMigration{
			`ALTER TABLE vpc ADD COLUMN issues jsonb NOT NULL DEFAULT '[]'`,
			`ALTER TABLE vpc ADD COLUMN config jsonb NOT NULL DEFAULT '{}'`,
			`ALTER TABLE vpc ADD COLUMN stack text NULL`,
			`UPDATE vpc SET stack = 'Test'`,
			`ALTER TABLE vpc ALTER COLUMN stack SET NOT NULL`,
			`ALTER TABLE task ADD depends_on_task_id integer REFERENCES task(id) NULL`,
		},
		&staticMigration{
			`ALTER TABLE aws_account ADD COLUMN project_name text NULL`,
			`UPDATE aws_account SET project_name = ''`,
			`ALTER TABLE aws_account ALTER COLUMN project_name SET NOT NULL`,
		},
		&staticMigration{
			`CREATE TABLE managed_transit_gateway_attachment (
				id serial PRIMARY KEY,
				transit_gateway_id TEXT NOT NULL,
				is_gov_cloud boolean NOT NULL,
				name TEXT NOT NULL,
				routes TEXT[] NOT NULL,
				UNIQUE (name, is_gov_cloud)
			)`,
			`CREATE TABLE configured_managed_transit_gateway_attachment (
				vpc_id integer REFERENCES vpc(id) NOT NULL,
				managed_transit_gateway_attachment_id integer REFERENCES managed_transit_gateway_attachment(id) NOT NULL,
				PRIMARY KEY(vpc_id, managed_transit_gateway_attachment_id)
			)`,
			`CREATE TABLE created_managed_transit_gateway_attachment (
				vpc_id integer REFERENCES vpc(id) NOT NULL,
				managed_transit_gateway_attachment_id integer REFERENCES managed_transit_gateway_attachment(id) ON DELETE CASCADE NOT NULL,
				transit_gateway_attachment_id TEXT NOT NULL,
				PRIMARY KEY(vpc_id, managed_transit_gateway_attachment_id)
			)`,
		},
		customMigration(func(tx *sqlx.Tx) error {
			// Special versions of structs for migrating Transit Gateway info. We must shadow
			// even the unchanged models to ensure the migration always works even if the
			// models change later.
			type RouteInfo struct {
				Destination       string
				NATGatewayID      string
				InternetGatewayID string
				TransitGatewayID  string
			}
			type PrivateSubnetInfra struct {
				AvailabilityZone           string
				RouteTableID               string
				RouteTableAssociationID    string
				EIPID                      string
				NATGatewayID               string
				Routes                     []*RouteInfo
				IsAttachedToTransitGateway bool `json:"IsAttachedToTransitGateway,omitempty"`
			}
			type PublicSubnetInfra struct {
				AvailabilityZone        string
				RouteTableAssociationID string
			}
			type TransitGatewayAttachment struct {
				ManagedTransitGatewayAttachmentID uint64 `json:"-"`
				TransitGatewayID                  string
				TransitGatewayAttachmentID        string
				SubnetIDs                         []string
			}
			type NetworkInfra struct {
				Public struct {
					InternetGatewayID         string
					RouteTableID              string
					Infra                     map[string]*PublicSubnetInfra
					Routes                    []*RouteInfo
					IsInternetGatewayAttached bool
				}
				Private                    map[string]*PrivateSubnetInfra
				TransitGatewayAttachmentID string                      `json:"TransitGatewayAttachmentID,omitempty"` // old field
				TransitGatewayAttachments  []*TransitGatewayAttachment // new field
			}
			type VPCConfig struct {
				ConnectPublic                      bool
				ConnectPrivate                     bool
				AttachTransitGateway               bool     `json:"AttachTransitGateway,omitempty"` // old field
				ManagedTransitGatewayAttachmentIDs []uint64 `json:"-"`                              // new field
			}
			const transitGatewayID = "tgw-0394354acd5bf4dd4"
			const transitGatewayRoutes = "{10.231.0.0/16}"

			_, err := tx.Exec("LOCK TABLE vpc")
			if err != nil {
				return err
			}
			_, err = tx.Exec("LOCK TABLE managed_transit_gateway_attachment")
			if err != nil {
				return err
			}
			_, err = tx.Exec("LOCK TABLE configured_managed_transit_gateway_attachment")
			if err != nil {
				return err
			}

			var managedID uint64
			err = tx.Get(&managedID, "INSERT INTO managed_transit_gateway_attachment (is_gov_cloud, name, transit_gateway_id, routes) VALUES (false, $1, $2, $3) RETURNING id", "Inter-VPC", transitGatewayID, transitGatewayRoutes)
			if err != nil {
				return err
			}

			var id uint64
			var stateb *[]byte
			var configb *[]byte
			q := "SELECT id, state, config FROM vpc"
			rows, err := tx.Query(q)
			if err != nil {
				return err
			}
			stateUpdates := make(map[uint64]*NetworkInfra)
			configUpdates := make(map[uint64]*VPCConfig)
			for rows.Next() {
				err := rows.Scan(&id, &stateb, &configb)
				if err != nil {
					return err
				}
				if stateb != nil {
					state := &NetworkInfra{}
					err := json.Unmarshal(*stateb, &state)
					if err != nil {
						return err
					}
					if state.TransitGatewayAttachmentID != "" {
						state.TransitGatewayAttachments = append(state.TransitGatewayAttachments, &TransitGatewayAttachment{
							ManagedTransitGatewayAttachmentID: managedID,
							TransitGatewayID:                  transitGatewayID,
							TransitGatewayAttachmentID:        state.TransitGatewayAttachmentID,
						})
						for subnetID, privateInfra := range state.Private {
							if privateInfra.IsAttachedToTransitGateway {
								state.TransitGatewayAttachments[0].SubnetIDs = append(state.TransitGatewayAttachments[0].SubnetIDs, subnetID)
							}
						}
						state.TransitGatewayAttachmentID = ""
						stateUpdates[id] = state
					}
				}
				if configb != nil {
					config := &VPCConfig{}
					err := json.Unmarshal(*configb, &config)
					if err != nil {
						return err
					}
					if config.AttachTransitGateway {
						config.AttachTransitGateway = false
						config.ManagedTransitGatewayAttachmentIDs = []uint64{managedID}
						configUpdates[id] = config
					}
				}
			}
			for id, state := range stateUpdates {
				data, err := json.Marshal(state)
				if err != nil {
					return err
				}
				q := "UPDATE vpc SET state=:state WHERE id=:id"
				_, err = tx.NamedExec(q, map[string]interface{}{
					"id":    id,
					"state": data,
				})
				if err != nil {
					return err
				}
				for _, tga := range state.TransitGatewayAttachments {
					q := "INSERT INTO created_managed_transit_gateway_attachment (vpc_id, managed_transit_gateway_attachment_id, transit_gateway_attachment_id) VALUES (:vpcID, :managedID, :attachmentID)"
					_, err = tx.NamedExec(q, map[string]interface{}{
						"vpcID":        id,
						"managedID":    tga.ManagedTransitGatewayAttachmentID,
						"attachmentID": tga.TransitGatewayAttachmentID,
					})
					if err != nil {
						return err
					}
				}
			}
			for id, config := range configUpdates {
				data, err := json.Marshal(config)
				if err != nil {
					return err
				}
				q := "UPDATE vpc SET config=:config WHERE id=:id"
				_, err = tx.NamedExec(q, map[string]interface{}{
					"id":     id,
					"config": data,
				})
				if err != nil {
					return err
				}
				for _, managedID := range config.ManagedTransitGatewayAttachmentIDs {
					q := "INSERT INTO configured_managed_transit_gateway_attachment (vpc_id, managed_transit_gateway_attachment_id) VALUES (:vpcID, :managedID)"
					_, err = tx.NamedExec(q, map[string]interface{}{
						"vpcID":     id,
						"managedID": managedID,
					})
					if err != nil {
						return err
					}
				}
			}

			return nil
		}),
		&staticMigration{
			`CREATE TABLE transit_gateway_resource_share (
				aws_account_id integer REFERENCES aws_account(id) NOT NULL,
				transit_gateway_id TEXT NOT NULL,
				aws_region TEXT NOT NULL,
				resource_share_id TEXT NOT NULL,
				PRIMARY KEY(aws_region, transit_gateway_id)
			)`,
		},
		&staticMigration{
			`UPDATE vpc SET stack=lower(stack);`,
		},
		// Convert old vpc.state JSON to new vpc.state JSON
		// We must shadow even the unchanged models to ensure the migration always works even if the
		// models change later.
		customMigration(func(tx *sqlx.Tx) error {
			type RouteInfo struct {
				Destination       string
				NATGatewayID      string
				InternetGatewayID string
				TransitGatewayID  string
			}

			type PrivateSubnetInfra struct {
				AvailabilityZone        string
				RouteTableID            string
				RouteTableAssociationID string
				EIPID                   string
				NATGatewayID            string
				Routes                  []*RouteInfo
			}
			type PublicSubnetInfra struct {
				AvailabilityZone        string
				RouteTableAssociationID string
			}

			type TransitGatewayAttachment struct {
				ManagedTransitGatewayAttachmentID uint64
				TransitGatewayID                  string
				TransitGatewayAttachmentID        string
				SubnetIDs                         []string
			}

			type OldNetworkInfra struct {
				Public struct {
					InternetGatewayID         string
					RouteTableID              string
					Infra                     map[string]*PublicSubnetInfra // subnet id -> public infra
					Routes                    []*RouteInfo
					IsInternetGatewayAttached bool
				}
				Private                   map[string]*PrivateSubnetInfra // subnet id -> private infra
				TransitGatewayAttachments []*TransitGatewayAttachment
			}

			type RouteTableInfo struct {
				RouteTableID string
				Routes       []*RouteInfo
			}

			type NATGatewayInfo struct {
				NATGatewayID string
				EIPID        string
			}

			type SubnetType string

			const (
				SubnetTypePrivate    SubnetType = "Private"
				SubnetTypePublic     SubnetType = "Public"
				SubnetTypeApp        SubnetType = "App"
				SubnetTypeData       SubnetType = "Data"
				SubnetTypeWeb        SubnetType = "Web"
				SubnetTypeTransport  SubnetType = "Transport"
				SubnetTypeSecurity   SubnetType = "Security"
				SubnetTypeManagement SubnetType = "Management"
				SubnetTypeShared     SubnetType = "Shared"
				SubnetTypeSharedOC   SubnetType = "Shared-OC"
			)

			type SubnetInfo struct {
				SubnetID                string
				GroupName               string
				RouteTableAssociationID string
				CustomRouteTable        *RouteTableInfo
			}

			type AvailabilityZoneInfra struct {
				Subnets           map[SubnetType][]*SubnetInfo
				PrivateRouteTable RouteTableInfo
				NATGateway        NATGatewayInfo
			}

			type InternetGatewayInfo struct {
				InternetGatewayID         string
				IsInternetGatewayAttached bool
			}

			type NetworkInfra struct {
				PublicRouteTable          RouteTableInfo
				InternetGateway           InternetGatewayInfo
				AvailabilityZones         map[string]*AvailabilityZoneInfra // AZ name -> info
				TransitGatewayAttachments []*TransitGatewayAttachment
			}

			_, err := tx.Exec("LOCK TABLE vpc")
			if err != nil {
				return err
			}

			var id uint64
			var stateb *[]byte
			q := "SELECT id, state FROM vpc"
			rows, err := tx.Query(q)
			if err != nil {
				return err
			}
			stateUpdates := make(map[uint64]*NetworkInfra)
			for rows.Next() {
				err := rows.Scan(&id, &stateb)
				if err != nil {
					return err
				}
				if stateb != nil {
					old := &OldNetworkInfra{}
					err := json.Unmarshal(*stateb, &old)
					if err != nil {
						return err
					}
					new := &NetworkInfra{
						TransitGatewayAttachments: old.TransitGatewayAttachments,
					}
					stateUpdates[id] = new

					new.AvailabilityZones = make(map[string]*AvailabilityZoneInfra)

					new.InternetGateway.InternetGatewayID = old.Public.InternetGatewayID
					new.InternetGateway.IsInternetGatewayAttached = old.Public.IsInternetGatewayAttached

					for subnetID, pi := range old.Public.Infra {
						az, ok := new.AvailabilityZones[pi.AvailabilityZone]
						if !ok {
							az = &AvailabilityZoneInfra{
								Subnets: map[SubnetType][]*SubnetInfo{},
							}
							new.AvailabilityZones[pi.AvailabilityZone] = az
						}
						az.Subnets[SubnetTypePublic] = append(
							az.Subnets[SubnetTypePublic],
							&SubnetInfo{
								SubnetID:                subnetID,
								RouteTableAssociationID: pi.RouteTableAssociationID,
							},
						)
					}

					new.PublicRouteTable.RouteTableID = old.Public.RouteTableID
					new.PublicRouteTable.Routes = old.Public.Routes

					for subnetID, pi := range old.Private {
						az, ok := new.AvailabilityZones[pi.AvailabilityZone]
						if !ok {
							az = &AvailabilityZoneInfra{
								Subnets: map[SubnetType][]*SubnetInfo{},
							}
							new.AvailabilityZones[pi.AvailabilityZone] = az
						}
						az.PrivateRouteTable.RouteTableID = pi.RouteTableID
						az.PrivateRouteTable.Routes = pi.Routes
						az.NATGateway.NATGatewayID = pi.NATGatewayID
						az.NATGateway.EIPID = pi.EIPID
						az.Subnets[SubnetTypePrivate] = append(
							az.Subnets[SubnetTypePrivate],
							&SubnetInfo{
								SubnetID:                subnetID,
								RouteTableAssociationID: pi.RouteTableAssociationID,
							},
						)
					}
				}
			}
			for id, state := range stateUpdates {
				data, err := json.Marshal(state)
				if err != nil {
					return err
				}
				q := "UPDATE vpc SET state=:state WHERE id=:id"
				_, err = tx.NamedExec(q, map[string]interface{}{
					"id":    id,
					"state": data,
				})
				if err != nil {
					return err
				}
			}

			return nil
		}),
		&staticMigration{
			`TRUNCATE TABLE session CASCADE`, // so "NOT NULL" doesn't fail
			`ALTER TABLE session ADD COLUMN user_id INTEGER NOT NULL`,
		},
		&staticMigration{
			`CREATE TABLE vpc_request (
				id serial PRIMARY KEY,
				added_at timestamp with time zone DEFAULT current_timestamp,
				aws_account_id integer REFERENCES aws_account(id) NOT NULL,
				requester_uid TEXT NOT NULL,
				requester_name TEXT NOT NULL,
				requester_email TEXT NOT NULL,
				status INTEGER NOT NULL,
				requested_config jsonb NOT NULL,
				jira_issue TEXT NOT NULL,
				approved_config jsonb NULL,
				task_id integer REFERENCES task(id) NULL
			)`,
		},
		&staticMigration{
			`CREATE TABLE configured_peering_connection (
				requester_vpc_id integer REFERENCES vpc(id) NOT NULL,
				accepter_vpc_id integer REFERENCES vpc(id) NOT NULL,
				requester_connect_private boolean NOT NULL,
				accepter_connect_private boolean NOT NULL,
				requester_connect_subnet_groups text[] NOT NULL,
				accepter_connect_subnet_groups text[] NOT NULL,
				PRIMARY KEY(requester_vpc_id, accepter_vpc_id)
			)`,
			`CREATE TABLE created_peering_connection (
				requester_vpc_id integer REFERENCES vpc(id) NOT NULL,
				accepter_vpc_id integer REFERENCES vpc(id) NOT NULL,
				peering_connection_id TEXT NOT NULL,
				is_accepted boolean NOT NULL,
				PRIMARY KEY(requester_vpc_id, accepter_vpc_id)
			)`,
			// Don't allow two peering connections with same VPCs where
			// requester and accepter swap.
			`CREATE UNIQUE INDEX configured_peering_connection_pair_idx ON configured_peering_connection (LEAST(requester_vpc_id, accepter_vpc_id), GREATEST(requester_vpc_id, accepter_vpc_id))`,
			`CREATE UNIQUE INDEX created_peering_connection_pair_idx ON created_peering_connection (LEAST(requester_vpc_id, accepter_vpc_id), GREATEST(requester_vpc_id, accepter_vpc_id))`,
		},
		&staticMigration{
			`CREATE TABLE security_group_set (
				id serial PRIMARY KEY,
				name TEXT NOT NULL,
				UNIQUE (name)
			)`,
			`CREATE TABLE security_group (
				id serial PRIMARY KEY,
				security_group_set_id integer REFERENCES security_group_set(id) NOT NULL,
				name TEXT NOT NULL,
				UNIQUE (security_group_set_id, name)
			)`,
			`CREATE TABLE security_group_rule (
				id serial PRIMARY KEY,
				description TEXT NOT NULL,
				security_group_id integer REFERENCES security_group(id) NOT NULL,
				is_egress boolean NOT NULL,
				protocol TEXT NOT NULL,
				from_port integer NOT NULL,
				to_port integer NOT NULL,
				source_cidr TEXT NOT NULL
			)`,
			`CREATE TABLE configured_security_group_set (
				vpc_id integer REFERENCES vpc(id) NOT NULL,
				security_group_set_id integer REFERENCES security_group_set(id) NOT NULL,
				PRIMARY KEY(vpc_id, security_group_set_id)
			)`,
			`CREATE TABLE created_security_group (
				vpc_id integer REFERENCES vpc(id) NOT NULL,
				security_group_id integer REFERENCES security_group(id) ON DELETE CASCADE NOT NULL,
				aws_id TEXT NOT NULL,
				PRIMARY KEY(vpc_id, aws_id),
				UNIQUE (vpc_id, security_group_id)
			)`,
		},
		&staticMigration{
			`ALTER TABLE vpc_request ALTER COLUMN jira_issue DROP NOT NULL`,
		},
		&staticMigration{
			`ALTER TABLE vpc_request ADD COLUMN comment TEXT NOT NULL default ''`,
		},
		&staticMigration{
			`CREATE TABLE batch_task (
				id serial PRIMARY KEY,
				description TEXT NOT NULL,
				added_at timestamp with time zone DEFAULT current_timestamp
			)`,
			`ALTER TABLE task ADD COLUMN batch_task_id integer REFERENCES batch_task(id) NULL`,
		},
		&staticMigration{
			`ALTER TABLE managed_transit_gateway_attachment ADD COLUMN subnet_types TEXT[] NOT NULL DEFAULT '{Private,App,Data,Web,Transport,Security,Management,Shared,Shared-OC}'`,
		},
		&staticMigration{
			`CREATE TABLE vpc_request_log (
				id serial PRIMARY KEY,
				added_at timestamp with time zone DEFAULT current_timestamp,
				message TEXT NOT NULL,
				retry_attempts integer NOT NULL,
				vpc_request_id integer REFERENCES vpc_request(id) NOT NULL,
				UNIQUE (message, vpc_request_id)
			)`,
		},
		&staticMigration{
			`ALTER TABLE security_group ADD COLUMN description TEXT NULL`,
			`UPDATE security_group SET description = name`,
			`ALTER TABLE security_group ALTER COLUMN description SET NOT NULL`,
		},
		&staticMigration{
			`ALTER TABLE vpc_request_log DROP CONSTRAINT vpc_request_log_message_vpc_request_id_key`,
			`CREATE UNIQUE INDEX vpc_request_log_message_vpc_request_id_key ON vpc_request_log (md5(message), vpc_request_id)`,
		},
		&staticMigration{
			`ALTER TABLE task_reservation ALTER COLUMN task_id DROP NOT NULL`,
		},
		&staticMigration{
			`CREATE TABLE managed_resolver_rule_set (
				id serial PRIMARY KEY,
				name TEXT NOT NULL,
				aws_region TEXT NOT NULL,
				aws_share_id TEXT NOT NULL,
				aws_account_id integer REFERENCES aws_account(id) NOT NULL,
				is_gov_cloud boolean NOT NULL,
				UNIQUE (name)
			)`,
			`CREATE TABLE resolver_rule (
				id serial PRIMARY KEY,
				ruleset_id integer REFERENCES managed_resolver_rule_set(id) NOT NULL,
				aws_id TEXT NOT NULL,
				description TEXT NOT NULL,
				UNIQUE (ruleset_id, aws_id)
			)`,
			`CREATE TABLE configured_managed_resolver_rule_set (
				vpc_id integer REFERENCES vpc(id) NOT NULL,
				managed_resolver_rule_set_id integer REFERENCES managed_resolver_rule_set(id) NOT NULL,
				PRIMARY KEY(vpc_id, managed_resolver_rule_set_id)
			)`,
		},
		customMigration(func(tx *sqlx.Tx) error {
			type ResolverRule struct {
				AWSID       string
				Description string
			}

			type ManagedResolverRuleSet struct {
				ID              uint64
				IsGovCloud      bool
				AccountID       string
				Name            string
				ResourceShareID string
				Rules           []*ResolverRule
			}

			var eastRules = []*ResolverRule{
				{
					AWSID:       "rslvr-rr-ddc9441e1e3e4478a",
					Description: "cms-local",
				},
				{
					AWSID:       "rslvr-rr-1f1b6d3bbee145e3a",
					Description: "awscloud-cms-local",
				},
			}
			var westRules = []*ResolverRule{
				{
					AWSID:       "rslvr-rr-44a8c5ecf18547b38",
					Description: "cms-local",
				},
				{
					AWSID:       "rslvr-rr-4cd33cfbdd5a4717a",
					Description: "awscloud-cms-local",
				},
			}
			var managedResolverRuleSets = map[string]ManagedResolverRuleSet{
				"us-east-1": {
					Name:            "east-private-dns",
					IsGovCloud:      false,
					AccountID:       "414072695903",
					ResourceShareID: "10b82d18-f8cd-4925-02bb-b437bb4be9a1",
					Rules:           eastRules,
				},
				"us-west-2": {
					Name:            "west-private-dns",
					IsGovCloud:      false,
					AccountID:       "414072695903",
					ResourceShareID: "14b82189-4c83-5c0d-605b-066d8b2f6223",
					Rules:           westRules,
				},
			}

			_, err := tx.Exec("LOCK TABLE vpc")
			if err != nil {
				return err
			}
			_, err = tx.Exec("LOCK TABLE managed_resolver_rule_set")
			if err != nil {
				return err
			}
			_, err = tx.Exec("LOCK TABLE configured_managed_resolver_rule_set")
			if err != nil {
				return err
			}

			var setID, ruleID, accountID uint64
			for region, ruleset := range managedResolverRuleSets {
				err = tx.Get(&accountID, "INSERT INTO aws_account (aws_id, name, is_gov_cloud, project_name) VALUES ($1, $2, $3, $4) ON CONFLICT (aws_id) DO UPDATE SET id=aws_account.id RETURNING id",
					ruleset.AccountID, ruleset.AccountID, ruleset.IsGovCloud, ruleset.AccountID,
				)
				if err != nil {
					return err
				}
				err = tx.Get(
					&setID, "INSERT INTO managed_resolver_rule_set (name, aws_region, aws_account_id, aws_share_id, is_gov_cloud) VALUES ($1, $2, $3, $4, $5) RETURNING id",
					ruleset.Name, region, accountID, ruleset.ResourceShareID, ruleset.IsGovCloud,
				)
				if err != nil {
					return err
				}
				for _, rule := range ruleset.Rules {
					err = tx.Get(
						&ruleID, "INSERT INTO resolver_rule (ruleset_id, aws_id, description) VALUES ($1, $2, $3) RETURNING id",
						setID, rule.AWSID, rule.Description,
					)
					if err != nil {
						return err
					}
				}
			}
			return nil
		}),
		&staticMigration{
			`ALTER TABLE vpc_request ADD COLUMN related_issues TEXT[] NOT NULL default '{}'`,
		},
		customMigration(func(tx *sqlx.Tx) error {
			// Update the subnet type and group name from 'transport' to 'transitive' for legacy VPCs
			// We must shadow even the unchanged models to ensure the migration always works even if the models change later.
			type SecurityGroupRule struct {
				Description    string
				IsEgress       bool
				Protocol       string
				FromPort       int64
				ToPort         int64
				SourceCIDR     string
				SourceIPV6CIDR string
			}

			type RouteInfo struct {
				Destination         string
				NATGatewayID        string
				InternetGatewayID   string
				TransitGatewayID    string
				PeeringConnectionID string
			}

			type TransitGatewayAttachment struct {
				ManagedTransitGatewayAttachmentID uint64 `json:"-"` // stored in created_managed_transit_gateway_attachment table
				TransitGatewayID                  string
				TransitGatewayAttachmentID        string
				SubnetIDs                         []string
			}

			type ResolverRuleAssociation struct {
				ResolverRuleID            string
				ResolverRuleAssociationID string
			}

			type SecurityGroup struct {
				TemplateID      uint64 `json:"-"` // stored in created_security_group table
				SecurityGroupID string
				Rules           []*SecurityGroupRule
			}

			type RouteTableInfo struct {
				RouteTableID string
				Routes       []*RouteInfo
			}

			type NATGatewayInfo struct {
				NATGatewayID string
				EIPID        string
			}

			type SubnetInfo struct {
				SubnetID                string
				GroupName               string
				RouteTableAssociationID string
				CustomRouteTable        *RouteTableInfo
			}

			type SubnetType string

			const (
				SubnetTypePrivate    SubnetType = "Private"
				SubnetTypePublic     SubnetType = "Public"
				SubnetTypeApp        SubnetType = "App"
				SubnetTypeData       SubnetType = "Data"
				SubnetTypeWeb        SubnetType = "Web"
				SubnetTypeTransport  SubnetType = "Transport"
				SubnetTypeTransitive SubnetType = "Transitive"
				SubnetTypeSecurity   SubnetType = "Security"
				SubnetTypeManagement SubnetType = "Management"
				SubnetTypeShared     SubnetType = "Shared"
				SubnetTypeSharedOC   SubnetType = "Shared-OC"
			)

			type AvailabilityZoneInfra struct {
				Subnets           map[SubnetType][]*SubnetInfo
				PrivateRouteTable RouteTableInfo
				NATGateway        NATGatewayInfo
			}

			type InternetGatewayInfo struct {
				InternetGatewayID         string
				IsInternetGatewayAttached bool
			}

			type VPCState struct {
				// "V3" VPCs are legacy VPCs not created by us, for which we support only:
				// - Import
				// - Manage transit gateway attachments and associated routes
				// - Manage peering connections
				// - Manage resolver rule sharing
				IsV3                      bool
				PublicRouteTable          RouteTableInfo
				InternetGateway           InternetGatewayInfo
				AvailabilityZones         map[string]*AvailabilityZoneInfra // AZ name -> info
				TransitGatewayAttachments []*TransitGatewayAttachment
				ResolverRuleAssociations  []*ResolverRuleAssociation
				PeeringConnections        []*PeeringConnection `json:"-"` // stored in created_peering_connection table
				SecurityGroups            []*SecurityGroup
				S3FlowLogID               string
				CloudWatchLogsFlowLogID   string
			}
			_, err := tx.Exec("LOCK TABLE vpc")
			if err != nil {
				return err
			}

			var id uint64
			var stateb *[]byte
			q := "SELECT id, state FROM vpc WHERE state -> 'IsV3' = 'true';"
			rows, err := tx.Query(q)
			if err != nil {
				return err
			}
			stateUpdates := make(map[uint64]*VPCState)
			for rows.Next() {
				err := rows.Scan(&id, &stateb)
				if err != nil {
					return err
				}
				if stateb != nil {
					state := &VPCState{}
					err := json.Unmarshal(*stateb, &state)
					if err != nil {
						return err
					}
					stateUpdates[id] = state

					for _, azInfo := range state.AvailabilityZones {
						for subnetType := range azInfo.Subnets {
							if subnetType == SubnetTypeTransitive {
								// shouldn't be possible, something is wrong
								err = fmt.Errorf("Existing transitive subnet type found for V3 VPC")
								return err
							}
						}
					}
					for _, azInfo := range state.AvailabilityZones {
						for subnetType, subnetInfoSlice := range azInfo.Subnets {
							if subnetType == SubnetTypeTransport {
								azInfo.Subnets[SubnetTypeTransitive] = subnetInfoSlice
								for _, subnetInfo := range subnetInfoSlice {
									subnetInfo.GroupName = "Transitive"
								}
								delete(azInfo.Subnets, SubnetTypeTransport)
							}
						}
					}
				}
			}
			for id, state := range stateUpdates {
				data, err := json.Marshal(state)
				if err != nil {
					return err
				}
				q := "UPDATE vpc SET state=:state WHERE id=:id"
				_, err = tx.NamedExec(q, map[string]interface{}{
					"id":    id,
					"state": data,
				})
				if err != nil {
					return err
				}
			}
			return nil
		}),
		&staticMigration{
			`CREATE TABLE task_lock (
				id serial PRIMARY KEY,
				worker_id TEXT NOT NULL,
				target_id TEXT NOT NULL,
				created_at timestamp with time zone DEFAULT current_timestamp,
				UNIQUE (target_id)
			)`,
			`CREATE OR REPLACE FUNCTION notify_new_task() RETURNS trigger as $$
			BEGIN  
			  PERFORM pg_notify('new_task', CONCAT('task_', NEW.id::text,'_', NEW.status::text));
			  RETURN NULL;
			END;
			$$ LANGUAGE plpgsql;`,
			`CREATE TRIGGER notify_new_task AFTER INSERT ON task FOR EACH ROW EXECUTE PROCEDURE notify_new_task();`,
			`CREATE OR REPLACE FUNCTION notify_lock_deleted() RETURNS trigger as $$
			BEGIN  
			  PERFORM pg_notify('new_task', CONCAT('lock_', OLD.id::text));
			  RETURN NULL;
			END;
			$$ LANGUAGE plpgsql;`,
			`CREATE TRIGGER lock_deleted AFTER DELETE ON task_lock FOR EACH ROW EXECUTE PROCEDURE notify_lock_deleted();`,
		},
		customMigration(func(tx *sqlx.Tx) error {
			type TransitGatewayAttachment struct {
				ManagedTransitGatewayAttachmentID uint64 `json:"-"` // stored in created_managed_transit_gateway_attachment table
				TransitGatewayID                  string
				TransitGatewayAttachmentID        string
				SubnetIDs                         []string
			}

			type ResolverRuleAssociation struct {
				ResolverRuleID            string
				ResolverRuleAssociationID string
			}

			type SecurityGroupRule struct {
				Description    string
				IsEgress       bool
				Protocol       string
				FromPort       int64
				ToPort         int64
				SourceCIDR     string
				SourceIPV6CIDR string
			}

			type SecurityGroup struct {
				TemplateID      uint64 `json:"-"` // stored in created_security_group table
				SecurityGroupID string
				Rules           []*SecurityGroupRule
			}

			type RouteInfo struct {
				Destination         string
				NATGatewayID        string
				InternetGatewayID   string
				TransitGatewayID    string
				PeeringConnectionID string
			}

			type OldRouteTableInfo struct {
				RouteTableID string
				Routes       []*RouteInfo
			}

			type SubnetInfo struct {
				SubnetID                string
				GroupName               string
				RouteTableAssociationID string
				CustomRouteTableID      string             // new field
				CustomRouteTable        *OldRouteTableInfo `json:"CustomRouteTable,omitempty"` // old field
			}

			type SubnetType string

			const (
				SubnetTypePrivate    SubnetType = "Private"
				SubnetTypePublic     SubnetType = "Public"
				SubnetTypeApp        SubnetType = "App"
				SubnetTypeData       SubnetType = "Data"
				SubnetTypeWeb        SubnetType = "Web"
				SubnetTypeTransport  SubnetType = "Transport"
				SubnetTypeTransitive SubnetType = "Transitive"
				SubnetTypeSecurity   SubnetType = "Security"
				SubnetTypeManagement SubnetType = "Management"
				SubnetTypeShared     SubnetType = "Shared"
				SubnetTypeSharedOC   SubnetType = "Shared-OC"
			)

			type RouteTableInfo struct {
				RouteTableID string `json:"-"`
				Routes       []*RouteInfo
				SubnetType   SubnetType
			}

			type NATGatewayInfo struct {
				NATGatewayID string
				EIPID        string
			}

			type AvailabilityZoneInfra struct {
				Subnets             map[SubnetType][]*SubnetInfo
				PrivateRouteTable   *OldRouteTableInfo `json:"PrivateRouteTable,omitempty"` // old field
				PrivateRouteTableID string             // new field
				NATGateway          NATGatewayInfo
			}

			type InternetGatewayInfo struct {
				InternetGatewayID         string
				IsInternetGatewayAttached bool
			}

			type VPCState struct {
				IsV3                      bool
				PublicRouteTable          *OldRouteTableInfo         `json:"PublicRouteTable,omitempty"` // old field
				PublicRouteTableID        string                     // new field
				RouteTables               map[string]*RouteTableInfo // RT id -> info
				InternetGateway           InternetGatewayInfo
				AvailabilityZones         map[string]*AvailabilityZoneInfra
				TransitGatewayAttachments []*TransitGatewayAttachment
				ResolverRuleAssociations  []*ResolverRuleAssociation
				PeeringConnections        []*PeeringConnection `json:"-"`
				SecurityGroups            []*SecurityGroup
				S3FlowLogID               string
				CloudWatchLogsFlowLogID   string
			}

			_, err := tx.Exec("LOCK TABLE vpc")
			if err != nil {
				return err
			}

			var id uint64
			var stateb *[]byte
			q := "SELECT id, state FROM vpc"
			rows, err := tx.Query(q)
			if err != nil {
				return err
			}
			stateUpdates := make(map[uint64]*VPCState)
			for rows.Next() {
				err := rows.Scan(&id, &stateb)
				if err != nil {
					return err
				}
				if stateb != nil {
					state := &VPCState{}
					err := json.Unmarshal(*stateb, &state)
					if err != nil {
						return err
					}
					stateUpdates[id] = state
					state.RouteTables = make(map[string]*RouteTableInfo)

					if state.PublicRouteTable.RouteTableID != "" {
						if state.IsV3 {
							err = fmt.Errorf("V4 public route table found for V3 VPC")
							return err
						}
						state.PublicRouteTableID = state.PublicRouteTable.RouteTableID
						state.RouteTables[state.PublicRouteTable.RouteTableID] = &RouteTableInfo{
							SubnetType: SubnetTypePublic,
							Routes:     state.PublicRouteTable.Routes,
						}
					}
					state.PublicRouteTable = nil
					for _, az := range state.AvailabilityZones {
						if az.PrivateRouteTable.RouteTableID != "" {
							if state.IsV3 {
								err = fmt.Errorf("V4 private route table found for V3 VPC")
								return err
							}
							az.PrivateRouteTableID = az.PrivateRouteTable.RouteTableID
							state.RouteTables[az.PrivateRouteTable.RouteTableID] = &RouteTableInfo{
								SubnetType: SubnetTypePrivate,
								Routes:     az.PrivateRouteTable.Routes,
							}
						}
						az.PrivateRouteTable = nil
						for subnetType, subnets := range az.Subnets {
							for _, subnet := range subnets {
								if subnet.CustomRouteTable != nil {
									subnet.CustomRouteTableID = subnet.CustomRouteTable.RouteTableID
									_, ok := state.RouteTables[subnet.CustomRouteTable.RouteTableID]
									if !ok {
										state.RouteTables[subnet.CustomRouteTable.RouteTableID] = &RouteTableInfo{
											SubnetType: subnetType,
											Routes:     subnet.CustomRouteTable.Routes,
										}
									} else {
										if subnet.CustomRouteTable.Routes != nil {
											if state.RouteTables[subnet.CustomRouteTable.RouteTableID].Routes == nil {
												state.RouteTables[subnet.CustomRouteTable.RouteTableID].Routes = subnet.CustomRouteTable.Routes
											} else {
												err = fmt.Errorf("Custom route table routes conflict with state for shared route table %s ", subnet.CustomRouteTable.RouteTableID)
												return err
											}
										}
									}
									subnet.CustomRouteTable = nil
								}
							}
						}
					}
				}
			}
			for id, state := range stateUpdates {
				data, err := json.Marshal(state)
				if err != nil {
					return err
				}
				q := "UPDATE vpc SET state=:state WHERE id=:id"
				_, err = tx.NamedExec(q, map[string]interface{}{
					"id":    id,
					"state": data,
				})
				if err != nil {
					return err
				}
			}
			return nil
		}),
		customMigration(func(tx *sqlx.Tx) error {
			// Replace VPCState.IsV3 with VPCState.VPCType
			// We must shadow even the unchanged models to ensure the migration always works even if the models change later.
			type VPCType int

			const (
				VPCTypeV1 VPCType = iota
				VPCTypeLegacy
				VPCTypeException
			)

			type RouteInfo struct {
				Destination         string
				NATGatewayID        string
				InternetGatewayID   string
				TransitGatewayID    string
				PeeringConnectionID string
			}

			type SubnetType string

			type RouteTableInfo struct {
				RouteTableID string `json:"-"` // filled in from keys of RouteTables map on VPCState
				Routes       []*RouteInfo
				SubnetType   SubnetType
			}

			type SubnetInfo struct {
				SubnetID                string
				GroupName               string
				RouteTableAssociationID string
				CustomRouteTableID      string
			}

			type NATGatewayInfo struct {
				NATGatewayID string
				EIPID        string
			}

			type AvailabilityZoneInfra struct {
				Subnets             map[SubnetType][]*SubnetInfo
				PrivateRouteTableID string
				NATGateway          NATGatewayInfo
			}

			type TransitGatewayAttachment struct {
				ManagedTransitGatewayAttachmentID uint64 `json:"-"` // stored in created_managed_transit_gateway_attachment table
				TransitGatewayID                  string
				TransitGatewayAttachmentID        string
				SubnetIDs                         []string
			}

			type ResolverRuleAssociation struct {
				ResolverRuleID            string
				ResolverRuleAssociationID string
			}

			type Region string

			type PeeringConnection struct {
				RequesterVPCID      string
				RequesterRegion     Region
				AccepterVPCID       string
				AccepterRegion      Region
				PeeringConnectionID string
				IsAccepted          bool
			}

			type SecurityGroupRule struct {
				Description    string
				IsEgress       bool
				Protocol       string
				FromPort       int64
				ToPort         int64
				SourceCIDR     string
				SourceIPV6CIDR string
			}

			type SecurityGroup struct {
				TemplateID      uint64 `json:"-"` // stored in created_security_group table
				SecurityGroupID string
				Rules           []*SecurityGroupRule
			}

			type InternetGatewayInfo struct {
				InternetGatewayID         string
				IsInternetGatewayAttached bool
			}

			type VPCState struct {
				IsV3                      bool    `json:"IsV3,omitempty"` // old field
				VPCType                   VPCType // new field
				PublicRouteTableID        string
				RouteTables               map[string]*RouteTableInfo // RT id -> info
				InternetGateway           InternetGatewayInfo
				AvailabilityZones         map[string]*AvailabilityZoneInfra // AZ name -> info
				TransitGatewayAttachments []*TransitGatewayAttachment
				ResolverRuleAssociations  []*ResolverRuleAssociation
				PeeringConnections        []*PeeringConnection `json:"-"` // stored in created_peering_connection table
				SecurityGroups            []*SecurityGroup
				S3FlowLogID               string
				CloudWatchLogsFlowLogID   string
			}
			_, err := tx.Exec("LOCK TABLE vpc")
			if err != nil {
				return err
			}

			var id uint64
			var stateb *[]byte
			q := "SELECT id, state FROM vpc"
			rows, err := tx.Query(q)
			if err != nil {
				return err
			}
			stateUpdates := make(map[uint64]*VPCState)
			for rows.Next() {
				err := rows.Scan(&id, &stateb)
				if err != nil {
					return err
				}
				if stateb != nil {
					state := &VPCState{}
					err := json.Unmarshal(*stateb, &state)
					if err != nil {
						return err
					}
					stateUpdates[id] = state
					if state.IsV3 {
						state.VPCType = VPCTypeLegacy
					} else {
						state.VPCType = VPCTypeV1
					}
					state.IsV3 = false
				}
			}
			for id, state := range stateUpdates {
				data, err := json.Marshal(state)
				if err != nil {
					return err
				}
				q := "UPDATE vpc SET state=:state WHERE id=:id"
				_, err = tx.NamedExec(q, map[string]interface{}{
					"id":    id,
					"state": data,
				})
				if err != nil {
					return err
				}
			}
			return nil
		}),
		&staticMigration{
			// enforce_one_row field prevents more than one row from
			//   being added to this table
			// if only_worker_name is NULL that means any worker is allowed
			`CREATE TABLE allow_tasks (
				enforce_one_row bool PRIMARY KEY DEFAULT TRUE,
				only_worker_name TEXT NULL,
				CONSTRAINT one_allowed_worker_name CHECK(enforce_one_row)
			)`,
			`INSERT INTO allow_tasks (only_worker_name) VALUES (NULL)`,
			`CREATE OR REPLACE FUNCTION notify_allow_tasks_changed() RETURNS trigger as $$
			BEGIN  
			  PERFORM pg_notify('new_task', CONCAT('only_allow_', NEW.only_worker_name::text));
			  RETURN NULL;
			END;
			$$ LANGUAGE plpgsql`,
			`CREATE TRIGGER allow_tasks_updated AFTER INSERT OR UPDATE ON allow_tasks FOR EACH ROW EXECUTE PROCEDURE notify_allow_tasks_changed()`,
		},
		customMigration(func(tx *sqlx.Tx) error {
			_, err := tx.Exec("LOCK TABLE managed_transit_gateway_attachment")
			if err != nil {
				return err
			}
			type ManagedTransitGatewayAttachment struct {
				region string
			}
			q := "ALTER TABLE managed_transit_gateway_attachment ADD COLUMN region TEXT NULL"
			_, err = tx.Exec(q)
			if err != nil {
				return err
			}
			var id uint64
			var name, region string
			var isGovCloud bool
			q = "SELECT id, is_gov_cloud, name FROM managed_transit_gateway_attachment"
			rows, err := tx.Query(q)
			if err != nil {
				return err
			}
			mtgas := make(map[uint64]*ManagedTransitGatewayAttachment)
			for rows.Next() {
				err := rows.Scan(&id, &isGovCloud, &name)
				if err != nil {
					return err
				}
				if isGovCloud {
					region = "us-gov-west-1"
				} else {
					if strings.Contains(strings.ToLower(name), "west") {
						region = "us-west-2"
					} else {
						region = "us-east-1"
					}
				}
				new := &ManagedTransitGatewayAttachment{
					region: region,
				}
				mtgas[id] = new
			}
			for id, mtag := range mtgas {
				q := "UPDATE managed_transit_gateway_attachment SET region=:region WHERE id=:id"
				_, err = tx.NamedExec(q, map[string]interface{}{
					"id":     id,
					"region": mtag.region,
				})
				if err != nil {
					return err
				}
			}
			q = "ALTER TABLE managed_transit_gateway_attachment ALTER COLUMN region SET NOT NULL"
			_, err = tx.Exec(q)
			if err != nil {
				return err
			}
			return nil
		}),
		&staticMigration{
			`ALTER TABLE security_group_rule RENAME COLUMN source_cidr TO source;`,
		},
		&staticMigration{
			`CREATE TABLE quickdns_request (
				id serial PRIMARY KEY,
				added_at timestamp with time zone DEFAULT current_timestamp,
				aws_account_id integer REFERENCES aws_account(id) NOT NULL,
				requester_uid TEXT NOT NULL,
				requester_name TEXT NOT NULL,
				requester_email TEXT NOT NULL,
				resource_type INTEGER NOT NULL,
				status INTEGER NOT NULL,
				requested_info jsonb NOT NULL,
				approved_info jsonb NULL,
				jira_issue TEXT NULL,
				task_id integer REFERENCES task(id) NULL
			)`,
			`CREATE TABLE dns_record (
				id serial PRIMARY KEY,
				data jsonb NOT NULL
			)`,
			`CREATE TABLE tls_record (
				id serial PRIMARY KEY,
				aws_id TEXT NOT NULL,
				data jsonb NOT NULL
			)`,
			`CREATE TABLE quickdns_record (
				id serial PRIMARY KEY,
				aws_account_id integer REFERENCES aws_account(id) NOT NULL,
				subject TEXT NOT NULL,
				region TEXT,
				dns_record_id INTEGER REFERENCES dns_record(id) NULL,
				tls_record_id INTEGER REFERENCES tls_record(id) NULL,
				created_at TIMESTAMP WITH TIME ZONE DEFAULT current_timestamp,
				deleted_at TIMESTAMP WITH TIME ZONE
				CHECK(
					(dns_record_id IS NOT NULL AND tls_record_id IS NULL)
					OR (dns_record_id IS NULL AND tls_record_id IS NOT NULL)
				)
				CHECK(
					(dns_record_id IS NOT NULL AND region IS NULL)
					OR (tls_record_id IS NOT NULL AND region IS NOT NULL)
				)
			)`,
			`CREATE UNIQUE INDEX quickdns_record_by_account ON quickdns_record (aws_account_id, subject, region)`,
		},
		&staticMigration{
			`ALTER TABLE session ADD COLUMN is_admin boolean NOT NULL DEFAULT false`,
			`ALTER TABLE session_aws_account ADD COLUMN is_approver boolean NOT NULL DEFAULT false`,
		},
		customMigration(func(tx *sqlx.Tx) error {
			// SecurityGroupRule.SourceCIDR -> SecurityGroupRule.Source
			// We must shadow even the unchanged models to ensure the migration always works even if the models change later.
			type VPCType int

			type RouteInfo struct {
				Destination         string
				NATGatewayID        string
				InternetGatewayID   string
				TransitGatewayID    string
				PeeringConnectionID string
			}

			type InternetGatewayInfo struct {
				InternetGatewayID         string
				IsInternetGatewayAttached bool
			}

			type SubnetInfo struct {
				SubnetID                string
				GroupName               string
				RouteTableAssociationID string
				CustomRouteTableID      string
			}

			type NATGatewayInfo struct {
				NATGatewayID string
				EIPID        string
			}

			type SubnetType string

			type RouteTableInfo struct {
				RouteTableID string `json:"-"` // filled in from keys of RouteTables map on VPCState
				Routes       []*RouteInfo
				SubnetType   SubnetType
			}

			type AvailabilityZoneInfra struct {
				Subnets             map[SubnetType][]*SubnetInfo
				PrivateRouteTableID string
				NATGateway          NATGatewayInfo
			}

			type TransitGatewayAttachment struct {
				ManagedTransitGatewayAttachmentID uint64 `json:"-"` // stored in created_managed_transit_gateway_attachment table
				TransitGatewayID                  string
				TransitGatewayAttachmentID        string
				SubnetIDs                         []string
			}

			type ResolverRuleAssociation struct {
				ResolverRuleID            string
				ResolverRuleAssociationID string
			}

			type Region string

			type PeeringConnection struct {
				RequesterVPCID      string
				RequesterRegion     Region
				AccepterVPCID       string
				AccepterRegion      Region
				PeeringConnectionID string
				IsAccepted          bool
			}

			type SecurityGroupRule struct {
				Description    string
				IsEgress       bool
				Protocol       string
				FromPort       int64
				ToPort         int64
				SourceCIDR     string `json:"SourceCIDR,omitempty"` // old field
				Source         string // new field
				SourceIPV6CIDR string
			}

			type SecurityGroup struct {
				TemplateID      uint64 `json:"-"` // stored in created_security_group table
				SecurityGroupID string
				Rules           []*SecurityGroupRule
			}

			type VPCState struct {
				VPCType                   VPCType // default type is V1
				PublicRouteTableID        string
				RouteTables               map[string]*RouteTableInfo // RT id -> info
				InternetGateway           InternetGatewayInfo
				AvailabilityZones         map[string]*AvailabilityZoneInfra // AZ name -> info
				TransitGatewayAttachments []*TransitGatewayAttachment
				ResolverRuleAssociations  []*ResolverRuleAssociation
				PeeringConnections        []*PeeringConnection `json:"-"` // stored in created_peering_connection table
				SecurityGroups            []*SecurityGroup
				S3FlowLogID               string
				CloudWatchLogsFlowLogID   string
			}

			_, err := tx.Exec("LOCK TABLE vpc")
			if err != nil {
				return err
			}

			rulesBySetID := make(map[int64][]*SecurityGroupRule)

			q := "SELECT security_group_id, description, is_egress, protocol, from_port, to_port, source FROM security_group_rule"
			rows, err := tx.Query(q)
			if err != nil {
				return err
			}
			for rows.Next() {
				rule := &SecurityGroupRule{}
				var setID *int64
				err := rows.Scan(
					&setID,
					&rule.Description,
					&rule.IsEgress,
					&rule.Protocol,
					&rule.FromPort,
					&rule.ToPort,
					&rule.Source,
				)
				if err != nil {
					return err
				}
				if _, ok := rulesBySetID[*setID]; !ok {
					rulesBySetID[*setID] = []*SecurityGroupRule{}
				}
				rulesBySetID[*setID] = append(rulesBySetID[*setID], rule)
			}

			var id uint64
			var stateb *[]byte
			stateUpdates := make(map[uint64]*VPCState)

			_, err = tx.Exec("DECLARE state_cursor CURSOR FOR SELECT id, state FROM vpc WHERE state -> 'VPCType' = '0' AND NOT is_deleted;")
			if err != nil {
				return err
			}
			defer tx.Exec("CLOSE state_cursor")

			for {
				row := tx.QueryRow("FETCH NEXT FROM state_cursor")
				err := row.Scan(&id, &stateb)
				if err != nil {
					if err == sql.ErrNoRows {
						break
					} else {
						return err
					}
				}
				if stateb != nil {
					state := &VPCState{}
					err := json.Unmarshal(*stateb, &state)
					if err != nil {
						return err
					}
					stateUpdates[id] = state
					for _, group := range state.SecurityGroups {
						for _, rule := range group.Rules {
							if rule.SourceIPV6CIDR != "" {
								// this is an IPV6 rule
								continue
							}

							if rule.Source != "" && rule.SourceCIDR != "" {
								// shouldn't be possible
								err = fmt.Errorf("A rule with both a Source and SourceCIDR value was found for VPC ID %d: %v+:", id, rule)
								return err
							}

							if rule.Source != "" && rule.SourceCIDR == "" {
								// Security group was created after the change
								continue
							}

							if rule.SourceCIDR != "" && rule.Source == "" {
								// VPC state has not been read/written since the change
								rule.Source = rule.SourceCIDR
								rule.SourceCIDR = ""
							}

							if rule.Source == "" && rule.SourceCIDR == "" {
								// VPC state has been read/written since the change
								var createdSetID *int64
								var accountID *int64
								var vpcID *string

								q := "SELECT vpc.aws_id AS vpc_id, aws_account.aws_id AS account_id FROM vpc INNER JOIN aws_account ON aws_account.id = aws_account_id WHERE vpc.id=$1;"
								row := tx.QueryRow(q, id)
								err = row.Scan(&vpcID, &accountID)
								if err != nil {
									if err == sql.ErrNoRows {
										err = fmt.Errorf("The query for the account ID and VPC ID of the VPC with DB ID %d didn't return a row\n", id)
										return err
									} else {
										return err
									}
								}

								sgID := group.SecurityGroupID
								if sgID == "" {
									err = fmt.Errorf("A security group without a SecurityGroupID was found for VPC ID %d\n", id)
									return err
								} else {
									q := "SELECT security_group_id FROM created_security_group WHERE aws_id=$1 AND vpc_id=$2"
									row := tx.QueryRow(q, sgID, id)
									err = row.Scan(&createdSetID)
									if err != nil {
										if err == sql.ErrNoRows {
											err = fmt.Errorf("The security group rule with AWS ID %s for the VPC with DB ID %d, VPC ID %s, and account ID %d has no record in the created_security_group table\n", sgID, id, *vpcID, *accountID)
											return err
										} else {
											return err
										}
									}
								}

								matchingRules := []*SecurityGroupRule{}
								for setID, dbRules := range rulesBySetID {
									for _, dbRule := range dbRules {
										if setID == *createdSetID &&
											dbRule.IsEgress == rule.IsEgress &&
											dbRule.Protocol == rule.Protocol &&
											dbRule.Description == rule.Description &&
											dbRule.FromPort == rule.FromPort &&
											dbRule.ToPort == rule.ToPort {
											matchingRules = append(matchingRules, dbRule)
										}
									}
								}

								if len(matchingRules) == 0 {
									err = fmt.Errorf("No matching rule was found for the VPC with DB ID %d, VPC ID %s, and account ID %d. Unmatched rule: %v (set ID %d)\n", id, *vpcID, *accountID, rule, *createdSetID)
									return err
								} else if len(matchingRules) > 1 {
									err = fmt.Errorf("Multiple matching rules were found for the VPC with DB ID %d, VPC ID %s, and account ID %d. Matched rule: %v (set ID %d)\n", id, *vpcID, *accountID, rule, *createdSetID)
									return err
								} else {
									rule.Source = matchingRules[0].Source
								}
							}
						}
					}
				}
			}

			for id, state := range stateUpdates {
				data, err := json.Marshal(state)
				if err != nil {
					return err
				}
				q := "UPDATE vpc SET state=:state WHERE id=:id"
				_, err = tx.NamedExec(q, map[string]interface{}{
					"id":    id,
					"state": data,
				})
				if err != nil {
					return err
				}
			}
			return nil
		}),
		customMigration(func(tx *sqlx.Tx) error {
			// Add group names to V1 public and private subnets
			// We must shadow even the unchanged models to ensure the migration always works even if the models change later.
			type RouteInfo struct {
				Destination         string
				NATGatewayID        string
				InternetGatewayID   string
				TransitGatewayID    string
				PeeringConnectionID string
			}

			type RouteTableInfo struct {
				RouteTableID string `json:"-"` // filled in from keys of RouteTables map on VPCState
				Routes       []*RouteInfo
				SubnetType   SubnetType
			}

			type InternetGatewayInfo struct {
				InternetGatewayID         string
				IsInternetGatewayAttached bool
			}

			type SubnetInfo struct {
				SubnetID                string
				GroupName               string
				RouteTableAssociationID string
				CustomRouteTableID      string
			}

			type NATGatewayInfo struct {
				NATGatewayID string
				EIPID        string
			}

			type SubnetType string

			const (
				SubnetTypePrivate    SubnetType = "Private"
				SubnetTypePublic     SubnetType = "Public"
				SubnetTypeApp        SubnetType = "App"
				SubnetTypeData       SubnetType = "Data"
				SubnetTypeWeb        SubnetType = "Web"
				SubnetTypeTransport  SubnetType = "Transport"
				SubnetTypeTransitive SubnetType = "Transitive"
				SubnetTypeSecurity   SubnetType = "Security"
				SubnetTypeManagement SubnetType = "Management"
				SubnetTypeShared     SubnetType = "Shared"
				SubnetTypeSharedOC   SubnetType = "Shared-OC"
			)

			type AvailabilityZoneInfra struct {
				Subnets             map[SubnetType][]*SubnetInfo
				PrivateRouteTableID string
				NATGateway          NATGatewayInfo
			}

			type TransitGatewayAttachment struct {
				ManagedTransitGatewayAttachmentID uint64 `json:"-"` // stored in created_managed_transit_gateway_attachment table
				TransitGatewayID                  string
				TransitGatewayAttachmentID        string
				SubnetIDs                         []string
			}

			type ResolverRuleAssociation struct {
				ResolverRuleID            string
				ResolverRuleAssociationID string
			}

			type Region string

			type PeeringConnection struct {
				RequesterVPCID      string
				RequesterRegion     Region
				AccepterVPCID       string
				AccepterRegion      Region
				PeeringConnectionID string
				IsAccepted          bool
			}

			type SecurityGroupRule struct {
				Description    string
				IsEgress       bool
				Protocol       string
				FromPort       int64
				ToPort         int64
				Source         string
				SourceIPV6CIDR string
			}

			type SecurityGroup struct {
				TemplateID      uint64 `json:"-"` // stored in created_security_group table
				SecurityGroupID string
				Rules           []*SecurityGroupRule
			}

			type VPCState struct {
				VPCType                   VPCType // default type is V1
				PublicRouteTableID        string
				RouteTables               map[string]*RouteTableInfo // RT id -> info
				InternetGateway           InternetGatewayInfo
				AvailabilityZones         map[string]*AvailabilityZoneInfra // AZ name -> info
				TransitGatewayAttachments []*TransitGatewayAttachment
				ResolverRuleAssociations  []*ResolverRuleAssociation
				PeeringConnections        []*PeeringConnection `json:"-"` // stored in created_peering_connection table
				SecurityGroups            []*SecurityGroup
				S3FlowLogID               string
				CloudWatchLogsFlowLogID   string
			}

			_, err := tx.Exec("LOCK TABLE vpc")
			if err != nil {
				return err
			}

			var id uint64
			var stateb *[]byte
			q := "SELECT id, state FROM vpc WHERE state -> 'VPCType' = '0';"
			rows, err := tx.Query(q)
			if err != nil {
				return err
			}
			stateUpdates := make(map[uint64]*VPCState)
			for rows.Next() {
				err := rows.Scan(&id, &stateb)
				if err != nil {
					return err
				}
				if stateb != nil {
					state := &VPCState{}
					err := json.Unmarshal(*stateb, &state)
					if err != nil {
						return err
					}
					stateUpdates[id] = state
					for _, az := range state.AvailabilityZones {
						for subnetType, subnets := range az.Subnets {
							for _, subnet := range subnets {
								if subnetType == SubnetTypePublic {
									subnet.GroupName = "public"
								}
								if subnetType == SubnetTypePrivate {
									subnet.GroupName = "private"
								}
							}
						}
					}
				}
			}
			for id, state := range stateUpdates {
				data, err := json.Marshal(state)
				if err != nil {
					return err
				}
				q := "UPDATE vpc SET state=:state WHERE id=:id"
				_, err = tx.NamedExec(q, map[string]interface{}{
					"id":    id,
					"state": data,
				})
				if err != nil {
					return err
				}
			}
			return nil
		}),
		&staticMigration{
			`ALTER TABLE vpc_request ADD COLUMN ip_justification TEXT NOT NULL default ''`,
		},
		&staticMigration{
			`CREATE TABLE label (
				id serial PRIMARY KEY,
				name TEXT NOT NULL,
				UNIQUE (name)
			)`,
			`CREATE TABLE vpc_label (
				vpc_id integer REFERENCES vpc(id) NOT NULL,
				label_id integer REFERENCES label(id) NOT NULL,
				PRIMARY KEY(vpc_id, label_id)
			)`,
			`CREATE TABLE account_label (
				account_id integer REFERENCES aws_account(id) NOT NULL,
				label_id integer REFERENCES label(id) NOT NULL,
				PRIMARY KEY(account_id, label_id)
			)`,
		},
		&staticMigration{
			`CREATE TABLE ip_usage (
				date DATE NOT NULL PRIMARY KEY,
				usage JSONB NOT NULL
			)`,
		},
		&staticMigration{
			`CREATE TABLE vpc_cidr (
				vpc_id integer REFERENCES vpc(id) NOT NULL,
				cidr inet NOT NULL,
				is_primary boolean NOT NULL DEFAULT false,
				UNIQUE (vpc_id, cidr)
			)`,
		},
		&staticMigration{
			`ALTER TABLE managed_transit_gateway_attachment ADD COLUMN is_default boolean NULL`,
			`UPDATE managed_transit_gateway_attachment SET is_default = false`,
			`ALTER TABLE managed_transit_gateway_attachment ALTER COLUMN is_default SET NOT NULL`,

			`ALTER TABLE security_group_set ADD COLUMN is_default boolean NULL`,
			`UPDATE security_group_set SET is_default = false`,
			`ALTER TABLE security_group_set ALTER COLUMN is_default SET NOT NULL`,

			`ALTER TABLE managed_resolver_rule_set ADD COLUMN is_default boolean NULL`,
			`UPDATE managed_resolver_rule_set SET is_default = false`,
			`ALTER TABLE managed_resolver_rule_set ALTER COLUMN is_default SET NOT NULL`,
		},
		&staticMigration{
			`ALTER TABLE vpc_request ADD COLUMN provisioned_vpc_id integer REFERENCES vpc(id) NULL`,
		},
		&staticMigration{
			`ALTER TABLE vpc_request ADD COLUMN request_type integer NULL`,
			`UPDATE vpc_request SET request_type = 0`,
			`ALTER TABLE vpc_request ALTER COLUMN request_type SET NOT NULL`,
		},
		&staticMigration{
			`LOCK TABLE aws_account`,
			`ALTER TABLE aws_account ADD COLUMN is_inactive bool NULL`,
			`UPDATE aws_account SET is_inactive = false`,
			`ALTER TABLE aws_account ALTER COLUMN is_inactive SET NOT NULL`,
		},
		&staticMigration{
			`LOCK TABLE session`,
			`ALTER TABLE session ADD COLUMN username text NULL`,
			`ALTER TABLE session ADD COLUMN expires_at timestamp with time zone DEFAULT current_timestamp`,
			`UPDATE session SET username = ''`,
			`ALTER TABLE session ALTER COLUMN username SET NOT NULL`,
		},
		&staticMigration{
			`ALTER TABLE security_group_set ADD COLUMN region TEXT NULL`,
			`UPDATE security_group_set SET region = ''`,
			`ALTER TABLE security_group_set ALTER COLUMN region SET NOT NULL`,
			`ALTER TABLE security_group_set ADD COLUMN is_gov_cloud boolean`,
			`UPDATE security_group_set SET is_gov_cloud = false`,
			`ALTER TABLE security_group_set ALTER COLUMN is_gov_cloud SET NOT NULL`,
			`ALTER TABLE security_group_set DROP CONSTRAINT security_group_set_name_key`,
			`ALTER TABLE security_group_set ADD CONSTRAINT security_group_set_name_key UNIQUE(name, region)`,
		},
		customMigration(func(tx *sqlx.Tx) error {
			_, err := tx.Exec("LOCK TABLE security_group_set")
			if err != nil {
				return err
			}

			_, err = tx.Exec("LOCK TABLE security_group")
			if err != nil {
				return err
			}

			_, err = tx.Exec("LOCK TABLE security_group_rule")
			if err != nil {
				return err
			}

			_, err = tx.Exec("LOCK TABLE configured_security_group_set")
			if err != nil {
				return err
			}

			_, err = tx.Exec("LOCK TABLE created_security_group")
			if err != nil {
				return err
			}

			type SecurityGroupRule struct {
				ID              uint64
				Description     string
				SecurityGroupID uint64
				IsEgress        bool
				Protocol        string
				FromPort        int64
				ToPort          int64
				Source          string
			}

			type SecurityGroup struct {
				ID                 uint64
				SecurityGroupSetID uint64
				Name               string
				Description        string
			}

			type SecurityGroupSet struct {
				ID         uint64
				Name       string
				IsDefault  bool
				Region     string
				IsGovCloud bool
			}

			type ConfiguredSecurityGroupSet struct {
				VPCID              uint64
				SecurityGroupSetID uint64
			}

			type CreatedSecurityGroup struct {
				VPCID           uint64
				SecurityGroupID uint64
				AWSID           string
			}

			regions := []string{"us-east-1", "us-west-2", "us-gov-west-1"}

			sgSetMapOldToNew := make(map[uint64][]uint64)

			sgMapOldToNew := make(map[uint64][]uint64)

			sgSetMapRegionOldToNew := make(map[string]map[uint64]uint64)
			sgSetMapRegionOldToNew["us-east-1"] = map[uint64]uint64{}
			sgSetMapRegionOldToNew["us-west-2"] = map[uint64]uint64{}
			sgSetMapRegionOldToNew["us-gov-west-1"] = map[uint64]uint64{}

			// TABLE STRUCTURE:
			//
			//           configured_security_group_set  -v
			// created_security_group -> security_group -> security_group_set
			// security_group_rule    -^
			//
			// The template consists of security_group_set template which has security_group templates which has security_group_rule's
			// We will need to do a deep copy of this structure as we create region specific security_group_set rows
			//
			// configured_security_group_set specifies which templates are applied to which VPCs
			// created_security_group specifies which security groups have been created in AWS in specific VPCs from these templates
			// We will need to update all rows in these tables to point to the new region specific structures based on what region the VPC is in

			// Get every secuirty group set we will use this list to generate region specific sets for each one
			q := "SELECT * FROM security_group_set"
			rows, err := tx.Query(q)
			if err != nil {
				return err
			}

			var sets []SecurityGroupSet
			for rows.Next() {
				set := &SecurityGroupSet{}
				err := rows.Scan(&set.ID, &set.Name, &set.IsDefault, &set.Region, &set.IsGovCloud)
				if err != nil {
					return err
				}
				sets = append(sets, *set)
			}

			// Create new region specific secuirty group sets
			// Record a mapping between the original set and the new set in sgSetMapOldToNew
			// Record a nested mapping between region and original set ID to the new set ID created for that region in sgSetMapRegionOldToNew
			// We will use these maps later to create associations of rows in other tables that reference these original sets.
			for _, set := range sets {
				var newID uint64

				for _, region := range regions {
					is_gov_cloud := false
					if region == "us-gov-west-1" {
						is_gov_cloud = true
					}

					err := tx.Get(&newID, "INSERT INTO security_group_set (name, is_default, region, is_gov_cloud) VALUES ($1, $2, $3, $4) RETURNING id", set.Name, set.IsDefault, region, is_gov_cloud)
					if err != nil {
						return err
					}
					sgSetMapOldToNew[set.ID] = append(sgSetMapOldToNew[set.ID], newID)
					sgSetMapRegionOldToNew[region][set.ID] = newID
				}
			}

			// Get all security group templates to continue the deep copy the security group template structure
			q = "SELECT * FROM security_group"
			rows, err = tx.Query(q)
			if err != nil {
				return err
			}

			var sgs []SecurityGroup
			for rows.Next() {
				sg := &SecurityGroup{}
				err := rows.Scan(&sg.ID, &sg.SecurityGroupSetID, &sg.Name, &sg.Description)
				if err != nil {
					return err
				}

				sgs = append(sgs, *sg)
			}

			// For each security group template, deep copy for each of the associated security group set templates being sure to update the security group set ID
			//
			// Initial state:
			// security_group1 -> security_group_set_old1
			//
			// End state:
			// security_group1      -> security_group_set_old1
			// security_group1_east -> security_group_set_old1_east
			// security_group1_west -> security_group_set_old1_west
			// security_group1_gov  -> security_group_set_old1_gov

			for _, sg := range sgs {
				var newID uint64
				for _, mappedID := range sgSetMapOldToNew[sg.SecurityGroupSetID] {
					err = tx.Get(&newID, "INSERT INTO security_group (security_group_set_id, name, description) VALUES ($1, $2, $3) RETURNING id", mappedID, sg.Name, sg.Description)
					if err != nil {
						return err
					}

					sgMapOldToNew[sg.ID] = append(sgMapOldToNew[sg.ID], newID)
				}
			}

			// Get all security group template rules to continue the deep copy the security group template structure
			q = "SELECT * FROM security_group_rule"
			rows, err = tx.Query(q)
			if err != nil {
				return err
			}

			var rules []SecurityGroupRule
			for rows.Next() {
				rule := &SecurityGroupRule{}
				err := rows.Scan(&rule.ID, &rule.Description, &rule.SecurityGroupID, &rule.IsEgress, &rule.Protocol, &rule.FromPort, &rule.ToPort, &rule.Source)
				if err != nil {
					return err
				}
				rules = append(rules, *rule)
			}

			// For each security group template, deep copy for each of the associated security group set templetes being sure to update the security group ID
			//
			// Initial state:
			// security_group_rule1 -> security_group_old1
			//
			// End state:
			// security_group_rule1      -> security_group_old1
			// security_group_rule1_east -> security_group_east1
			// security_group_rule1_west -> security_group_west1
			// security_group_rule1_gov  -> security_group_gov1

			for _, rule := range rules {
				for _, mappedID := range sgMapOldToNew[rule.SecurityGroupID] {
					_, err = tx.Exec("INSERT INTO security_group_rule (description, security_group_id, is_egress, protocol, from_port, to_port, source) VALUES ($1, $2, $3, $4, $5, $6, $7)", rule.Description, mappedID, rule.IsEgress, rule.Protocol, rule.FromPort, rule.ToPort, rule.Source)
					if err != nil {
						return err
					}
				}
			}

			// Update each configured security group set to point from the old set ID to the associated new set based on the region of the configured VPC

			var configuredSGSets []ConfiguredSecurityGroupSet
			q = "SELECT * FROM configured_security_group_set"
			rows, err = tx.Query(q)
			if err != nil {
				return err
			}

			for rows.Next() {
				sgSet := &ConfiguredSecurityGroupSet{}
				err := rows.Scan(&sgSet.VPCID, &sgSet.SecurityGroupSetID)
				if err != nil {
					return err
				}

				configuredSGSets = append(configuredSGSets, *sgSet)
			}

			for _, sgSet := range configuredSGSets {
				var region string
				err = tx.Get(&region, "SELECT aws_region FROM vpc WHERE id=$1", sgSet.VPCID)
				if err != nil {
					return err
				}

				q = "UPDATE configured_security_group_set SET security_group_set_id = $2 WHERE vpc_id = $1 AND security_group_set_id = $3"
				_, err := tx.Exec(q, sgSet.VPCID, sgSetMapRegionOldToNew[region][sgSet.SecurityGroupSetID], sgSet.SecurityGroupSetID)
				if err != nil {
					return err
				}
			}

			// Update each created security group to point from the old created security group ID to the associated new created security group
			// based on the region of the VPC associated with this created security group

			q = "SELECT * FROM created_security_group"
			rows, err = tx.Query(q)
			if err != nil {
				return err
			}

			var createdSgs []CreatedSecurityGroup
			for rows.Next() {
				createdSg := &CreatedSecurityGroup{}
				err := rows.Scan(&createdSg.VPCID, &createdSg.SecurityGroupID, &createdSg.AWSID)
				if err != nil {
					return err
				}

				createdSgs = append(createdSgs, *createdSg)
			}

			for _, createdSg := range createdSgs {
				var region string
				err = tx.Get(&region, "SELECT aws_region FROM vpc WHERE id=$1", createdSg.VPCID)
				if err != nil {
					return err
				}

				var oldSgSetID uint64
				err = tx.Get(&oldSgSetID, "SELECT security_group_set_id FROM security_group WHERE id=$1", createdSg.SecurityGroupID)
				if err != nil {
					return err
				}

				newSgSetID := sgSetMapRegionOldToNew[region][oldSgSetID]

				var newSgID uint64
				err = tx.Get(&newSgID, "SELECT id FROM security_group WHERE security_group_set_id=$1", newSgSetID)
				if err != nil {
					return err
				}

				q = "UPDATE created_security_group SET security_group_id=$1 WHERE aws_id=$2"
				_, err := tx.Exec(q, newSgID, createdSg.AWSID)
				if err != nil {
					return err
				}
			}

			return nil
		}),
		&staticMigration{
			`LOCK TABLE session_aws_account`,
			`ALTER TABLE session_aws_account ADD COLUMN expiration TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT '-infinity'::TIMESTAMP WITHOUT TIME ZONE`,
		},
		&staticMigration{
			`CREATE TABLE micro_service_heartbeats (service_name TEXT PRIMARY KEY, last_success timestamp with time zone DEFAULT current_timestamp)`,
		},
	}
}
