provider "aws" {
  region  = "us-gov-west-1"
  version = "~> 3.0"

  allowed_account_ids = ["849804945443"]
}

terraform {
  required_version = "= 0.12.23"

  backend "s3" {
    bucket         = "prefix-lists-tfstate"
    key            = "state-prefix-lists-east"
    region         = "us-gov-west-1"
    dynamodb_table = "prefix-lists-lock-table"
  }
}
