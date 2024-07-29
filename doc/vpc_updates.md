# VPC Update Model/Process

Every VPC that the database knows about has a `Config` and a `State`. These are primarily stored as JSON in the `config` and `state` fields of each VPC row, but there are also some auxiliary tables that used when we wish to make use of foreign key relations.

## Config
This describes configurable choices that have been made about the VPC, things like whether to connect its subnets to the internet and which other VPCs to peer it with. This is updated whenever the user clicks "Update networking" or "Update security groups" in the UI.

## State
This describes the current state of the VPC's infrastructure in AWS. It is initially established when the VPC is created or imported, and gets updated every time we make an API call that makes some change to the infrastructure.

## Operations

### Import
This action brings a VPC under management by checking the state of the VPC's infrastructure in AWS and creating a `State` and `Config` to reflect that infrastructure. On import, a VPC is checked for [compliant structure](https://docs.google.com/document/d/1XPsZPiUMtvnq9GTJ9eX9i54vGvahp4Bs27-usdf2Y6s/edit#) to make sure it is compatible with management by VPC Conf.  

### Unimport
This action removes a VPC from management. Some resources which are managed (e.g. resolver rules, peering connections, transit gateway attachments, security groups) are not currently re-importable.

### Establish exception
This action creates a record of a VPC that cannot be managed due to a non-compliant structure. Such VPCs are managed by an external tool, [EVM](https://github.com/CMSgov/CMS-AWS-West-Network-Architecture/pull/232), which uses VPC Conf as the source of truth for the list of VPCs that it is responsible for managing.

### Update networking/security groups/resolver rules
This makes changes to the VPC's `Config` (e.g. enable connecting public subnets to the internet) and then fires a non-interactive task to make actual changes to the infrastructure. By the end of the task the `State` should reflect all the new/changed infrastructure needed (e.g. an internet gateway being created) to make that change take effect. Note that unmanaged resources should not be imported as part of an 'Update networking' action.

### Check for issues
This does two things:
1. Detect if any part of the `State` no longer matches what is live in AWS, normally due to manual changes done outside of the application.
2. Detect if any resources have the wrong tags. (Tags are not recorded as part of the state.)

### Repair
This does three things:
1. Update the `State` to fix any of the mismatches detected by "Check for issues."
2. Update any incorrect tags.
3. Run "Update networking" and "Update security groups" to make the API calls necessary to make the `State` conform to the `Config`.

### Example
Here is a concrete example: a VPC has been configured to have an attachment to a transit gateway, and the attachment was previously created, which includes routes being added to the route tables of various subnets. Then one of those routes (say, in the `us-east-1a` region) got deleted by a user in the AWS Admin Console. Here is what would happen if you did various actions:
- **Check for issues**: the task worker would see that `vpc.State.AvailabilityZones["us-east-1a"].PrivateRouteTable.Routes` contains routes for the desired destinations but the actual route table does not, so it would report an issue.
- **Repair**: the task worker would observe the above problem and would delete the relevant route from the state, and save the new state to the database.
- **Update Networking after repairing the state**: the task worker would observe that the state is missing the desired route and would add it and update the state, after which checking for issues or repairing would no longer find any problems.
- **Update networking *without repairing the state first***: the task worker would not see any mismatch between the `State` and `Config` so it would not make any changes.

