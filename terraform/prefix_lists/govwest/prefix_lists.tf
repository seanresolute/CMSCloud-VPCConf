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
    # "greenfield-dev" = module.pl-greenfield-dev.ids
    # "greenfield-impl" = module.pl-greenfield-impl.ids
    "greenfield-prod" = module.pl-greenfield-prod.ids
  }
}
