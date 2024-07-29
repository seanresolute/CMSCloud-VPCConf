module "pl-legacy-dev" {
  source = "../modules/prefix_list"

  vpc_version = "v3"
  env         = "prod"
  pl_env      = "dev"
}

module "pl-legacy-impl" {
  source = "../modules/prefix_list"

  vpc_version = "v3"
  env         = "prod"
  pl_env      = "impl"
}

module "pl-legacy-prod" {
  source = "../modules/prefix_list"

  vpc_version = "v3"
  env         = "prod"
  pl_env      = "prod"
}

### TODO: Re-enable these modules when vpc-conf supports a per-environment default
# module "pl-greenfield-dev" {
#   source = "../modules/prefix_list"

#   env = "prod"
#   pl_env = "dev"
# }

# module "pl-greenfield-impl" {
#   source = "../modules/prefix_list"

#   env = "prod"
#   pl_env = "impl"
# }

module "pl-greenfield-prod" {
  source = "../modules/prefix_list"

  env    = "prod"
  pl_env = "prod"
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
