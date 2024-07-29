provider "aws" {
  region              = var.region
  allowed_account_ids = [var.allowed_account_id]
}

terraform {
  required_version = ">= 1.3.7"

  required_providers {
    aws = {
      version = "~> 4.50"
    }
  }
}

module "init" {
  source     = "../../../../modules/init"
  account_id = var.allowed_account_id
  env        = var.env
  app_name   = var.app_name
  region     = var.region
}
