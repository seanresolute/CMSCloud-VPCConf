# InterVPC Transit Gateway Prefix List Infrastructure as Code

NOTE: THIS FOLDER SHOULD BE PULLED OUT INTO A NETWORK OPS REPO

This terraform manages the prefix lists used in VPCs to provide routes on the InterVPC Transit Gateway.

`./modules/prefix_list/main.tf` is where the CIDRs for the prefix lists are defined. Putting a CIDR in `pl_entries`. These CIDRs will be applied to all prefix lists in all regions on `terraform apply`.

`region_specific_pl_entries` works similarly, however CIDRs are added to a specific region in this map and those CIDRs will be added to all prefix lists in the associated region but not the other regions.

After any change is made in `./modules/prefix_list/main.tf` run ‘terraform plan’ verify that there are no manual changes that will be lost and then run ‘terraform apply’ in `./east` `./west` with AWS creds for account 921617238787
and run `terraform apply` in `./govwest` with AWS creds for account 849804945443.

Remember on any change to apply the changes to all 3 regions (commercial east and west, as well as govcloud) so that connectivity is consistent regardless of where the VPC is deployed and what version VPC it has.
