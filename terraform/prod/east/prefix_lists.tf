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
  }
}

module "pl-legacy-dev" {
  source = "../../prefix_lists/modules/prefix_list"

  vpc_version = "v3"
  env         = "prod"
  pl_env      = "dev"
  pl_entries  = merge(var.pl_entries)
}

module "pl-legacy-impl" {
  source = "../../prefix_lists/modules/prefix_list"

  vpc_version = "v3"
  env         = "prod"
  pl_env      = "impl"
  pl_entries  = merge(var.pl_entries)
}

module "pl-legacy-prod" {
  source = "../../prefix_lists/modules/prefix_list"

  vpc_version = "v3"
  env         = "prod"
  pl_env      = "prod"
  pl_entries  = merge(var.pl_entries)
}

### TODO: Re-enable these modules when vpc-conf supports a per-environment default
# module "pl-greenfield-dev" {
#   source = "../../prefix_lists/modules/prefix_list"

#   env = "prod"
#   pl_env = "dev"
#   pl_entries = merge(var.pl_entries)
# }

# module "pl-greenfield-impl" {
#   source = "../../prefix_lists/modules/prefix_list"

#   env = "prod"
#   pl_env = "impl"
#   pl_entries = merge(var.pl_entries)
# }

module "pl-greenfield-prod" {
  source = "../../prefix_lists/modules/prefix_list"

  env        = "prod"
  pl_env     = "prod"
  pl_entries = merge(var.pl_entries)
}

output "prefix_lists" {
  value = {
    "legacy-dev"  = module.pl-legacy-dev.ids
    "legacy-impl" = module.pl-legacy-impl.ids
    "legacy-prod" = module.pl-legacy-prod.ids
    # "greenfield-dev" = module.pl-greenfield-dev.ids
    # "greenfield-impl" = module.pl-greenfield-impl.ids
    "greenfield-prod" = module.pl-greenfield-prod.ids
  }
}
