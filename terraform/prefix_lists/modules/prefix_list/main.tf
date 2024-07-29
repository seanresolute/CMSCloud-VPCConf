data "aws_region" "current" {}

variable "pl_entries" {
  default = {
    "10.128.0.0/15"     = "AWS-OC East+West"
    "10.131.125.0/24"   = "ldap-impl-vpc"
    "10.138.1.0/24"     = "ldap-prod-vpc"
    "10.138.132.0/22"   = "enterpriseV3-ssval"
    "10.220.120.0/22"   = "Greenfield Lower-Shared West"
    "10.220.126.0/23"   = "Greenfield Prod-Shared West"
    "10.220.128.0/20"   = "Greenfield Prod-Shared West"
    "10.223.120.0/22"   = "Greenfield Lower-Shared East"
    "10.203.144.0/20"   = "Greenfield Lower-Shared East"
    "10.223.126.0/23"   = "Greenfield Prod-Shared East"
    "10.223.128.0/20"   = "Greenfield Prod-Shared East"
    "10.231.9.128/26"   = "Security West Dev"
    "10.231.244.64/26"  = "Security West Prod"
    "10.232.32.0/19"    = "cisco-vpn-prod"
    "10.235.58.0/24"    = "ldap-dev-vpc"
    "10.240.120.0/22"   = "Greenfield Lower-Shared GovWest"
    "10.240.126.0/23"   = "Greenfield Prod-Shared GovWest"
    "10.240.128.0/20"   = "Greenfield Prod-Shared GovWest"
    "10.242.7.192/26"   = "Security East Dev"
    "10.242.193.192/26" = "Security East Prod"
    "10.244.96.0/19"    = "enterpriseV3-sharedService"
    "10.252.0.0/22"     = "dev sec ops prod"
    "10.228.33.0/24"    = "shared service vnet (MAG)"
    "10.228.35.32/27"   = "infoblox (MAG)"
    "10.228.62.0/27"    = "trendmicro (MAG)"
    "10.135.240.0/20"   = "cisco-vpn-west-prod"
  }
}

variable "region_specific_pl_entries" {
  default = {
    "us-gov-west-1" = {
      "10.239.204.0/22" = "EXTERNAL-SPLUNK-3rdParty"
    }

    "us-east-1" = {
    }

    "us-west-2" = {
    }
  }
}

module "tagging" {
  source         = "../tagging"
  component_name = "NetworkOps"
  environment    = var.env
}

resource "aws_ec2_managed_prefix_list" "this" {
  name           = "cmscloud-${var.vpc_version}-shared-services-${var.pl_env}-${count.index + 1}"
  count          = var.shard_count
  max_entries    = var.max_entries
  address_family = var.address_family

  dynamic "entry" {
    for_each = merge(var.pl_entries, var.region_specific_pl_entries["${data.aws_region.current.name}"])
    content {
      cidr        = entry.key
      description = entry.value
    }
  }

  tags = merge(module.tagging.common_tags)
}

variable "shard_count" {
  default = 1
}

variable "env" {
  type = string
}

variable "pl_env" {
  type = string
}

variable "max_entries" {
  type    = number
  default = 32
}

variable "address_family" {
  default = "IPv4"
}

variable "vpc_version" {
  type    = string
  default = "v4"
}

output "ids" {
  value = aws_ec2_managed_prefix_list.this[*].id
}
