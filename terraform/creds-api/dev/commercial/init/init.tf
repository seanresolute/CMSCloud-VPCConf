provider "aws" {
  region  = var.region
  version = "~> 2.70.0"

  allowed_account_ids = [var.allowed_account_id]
}

terraform {
  required_version = ">= 0.12.23"
}

module "init" {
  source   = "../../../modules/init"
  env      = var.env
  app_name = var.app_name
  region   = var.region
}