# VPC Resources and Objects Operations Manager

vroom was built to be able to perform operations across a selectable group of VPCs and/or accounts. It is entirely command line driven, but interacts with VPC Conf. It currently supports a number of operations (you can see the list in main.go, search for "List of available commands"). It is very useful to be able to perform ad-hoc operations across any number of vpcs. It technically only directly supports v4/greenfield VPCs.

## Pre-requisites

* Connect to VPN
* Run a local instance of redis, listening on port 6379, for caching purposes
* Set the following environment variables
    * VROOM_USERNAME - set to your EUA ID
    * VROOM_PASSWORD - set to your password
    * CLOUDTAMER_BASE_URL - set to the URL of cloudtamer, up to and including "/api"
    * VPCCONF_BASE_URL - set to the URL of vpc-conf, up to and including "/provision"
    * CLOUDTAMER_IDMS_ID - set to the authentication database id within cloudtamer/kion (currently 2)
    * CLOUDTAMER_ADMIN_GROUP_ID - set to the group to use to pull the temporary creds

## Target selection

To select your target, you have multiple options available:

* --vpc <vpcID>
* --region <us-east-1/us-west-2/us-gov-west-1>
* --account <accountID>
* --stack <dev/test/impl/prod>

If you use the --vpc selector, it will ignore the remaining options and find that vpc specifically. Due to how it searches for the vpc (search within VPC Conf), you can, in theory, put a partial matching string as the vpc id and it should hopefully find it. If there is more than one VPC that returns from that search, it will not proceed

## Basic command-line execution flow

1. vroom <selectors> refresh
1. vroom <selectors> <action>

## Code flow

1. Read in command line flags/args
1. Spawn off requested number of threads (default: 1) - an ideal number from experience is around 20-30
    1. Each thread has a job runner
    1. Each job runner executes the requested action
1. Create jobs for every selected VPC
1. After all jobs complete (regardless of success or failure), list all vpcs operated on, and some basic stats (stats may be slightly broken currently)

## New Features

Writing simple new functionality is fairly simple to plug-in to the existing framework:

1. Create new function (cf. showVPCData() or findIP() for somewhat shorter examples)
1. Add a text to function mapping in the `commands` map