provider "aws" {
  region  = "us-west-2"
  version = "~> 3.0"

  allowed_account_ids = ["921617238787"]
}

terraform {
  required_version = "= 0.12.23"

  backend "s3" {
    bucket         = "prefix-lists-tfstate"
    key            = "state-prefix-lists-west"
    region         = "us-east-1"
    dynamodb_table = "prefix-lists-lock-table"
  }
}
