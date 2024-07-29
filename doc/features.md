# VPCConf Features

## VPC Creation and Deletion
VPCConf coordinates with IPControl to make sure that the CIDRs used are not already in use and will not be used by other VPCs in the future (unless the VPC gets deleted first).

## VPC Networking

### Internet Connections
VPCConf supports creating Internet Gateways and NAT Gateways and updating subnet route tables to point to them.

### Transit Gateway Attachments
VPCConf has "Managed Transit Gateway Attachments" (sort of like templates) which, once defined, can be selected for any VPC. When one is selected for use in a VPC:
- The Transit Gateway will be shared using Resource Access Manager (requires a resource share to be created manually and its ID given to VPCConf, a one-time process per Transit Gateway for it to be shared with any number of VPCs)
- A Transit Gateway Attachment will be created in the private subnets for the VPC.
- Routes will be added to various route tables in the VPC. The specific routes and which subnets get them added to their route tables is configured in the template.

### Peering Connections
VPCConf supports peering any two managed VPCs. Which subnets to connect is configurable (but connecting public subnets is not allowed) and routes will be automatically created to match the configuration.

### Security Group Templates
VPCConf supports creating sets of predefined static security groups which can then be added to any VPC by checking a box.

### CMSNet Connections
VPCConf assists in connecting to CMSNet via a multi-step process:
1. Create a new "zoned" subnet whose Zone Type matches CMSNet zone that you want to connect to.
2. Connect the zoned subnet to a specific IP/CIDR on CMSNet (via a transit gateway and subnet route tables).
3. [does not involve VPCConf] Coordinate with Verizon to accept incoming connections from your IP/CIDR.

### AWS Route53 Resolver Rules
VPCConf can help manage distribution and association of centralized AWS Route53 Resolver Rules from ITOPS controlled accounts. The general use-case for this feature is to allow any ADO to access the "private DNS" zones like `cms.local` and `awscloud.cms.local` for purposes of joining the active directory domains

## Batch Tasks
VPCConf has a Batch Task interface, where changes can be made and tasks started for entire classes of VPCs at once. These are scheduled as individual tasks for each VPC but the interface allows you to track a whole group of tasks together.

## Network Firewall
VPC Conf can create VPCs with the [Network Firewall](https://aws.amazon.com/network-firewall/?whats-new-cards.sort-by=item.additionalFields.postDateTime&whats-new-cards.sort-order=desc) service. These VPCs have their own type, with a distinct [architecture](https://confluenceent.cms.gov/display/ITOPS/Network+Firewall+VPC+Design+Doc#NetworkFirewallVPCDesignDoc-Architecture) that supports the feature.  VPC Conf can also perform a migration to add or remove Network Firewall from a VPC. 