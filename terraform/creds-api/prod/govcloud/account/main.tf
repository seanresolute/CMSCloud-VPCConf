provider "aws" {
  region  = "us-gov-west-1"
  version = "~> 2.70.0"

  allowed_account_ids = ["350521122370"]
}

terraform {
  required_version = ">= 0.12.23"

  backend "s3" {
    bucket         = "creds-api-prod-tfstate"
    key            = "account/state"
    region         = "us-gov-west-1"
    dynamodb_table = "creds-api-prod-lock-table"
  }
}

module "tagging" {
  source         = "../../../../modules/tagging"
  component_name = "creds-api"
  environment    = "prod"
}

module "common_account" {
  source         = "../../../../modules/ecs_app_common/account/"
  env            = "prod"
  appname        = "creds-api"
  component_name = "creds-api"
  use_rds        = false
}

output "aws_iam_role_app_arn" {
  value = module.common_account.aws_iam_role_app_arn
}

output "aws_iam_role_redeploy_arn" {
  value = module.common_account.aws_iam_role_redeploy_arn
}

resource "aws_dynamodb_table" "artifactdb" {
  name         = "ArtifactDB"
  billing_mode = "PAY_PER_REQUEST"

  hash_key  = "ProjectName"
  range_key = "BuildNumber"

  attribute {
    name = "ProjectName"
    type = "S"
  }

  attribute {
    name = "BuildNumber"
    type = "N"
  }
  tags = merge(module.tagging.common_tags)
}

resource "aws_dynamodb_table" "deploy_lock" {
  name         = "DeployLock"
  billing_mode = "PAY_PER_REQUEST"

  hash_key = "key"

  attribute {
    name = "key"
    type = "S"
  }
  tags = merge(module.tagging.common_tags)
}

resource "aws_s3_bucket" "artifact_db_binary" {
  bucket = "vpc-automation-artifact-db"
  acl    = "private"

  versioning {
    enabled = true
  }

  server_side_encryption_configuration {
    rule {
      apply_server_side_encryption_by_default {
        sse_algorithm = "AES256"
      }
    }
  }
  tags = merge(module.tagging.common_tags)
}

resource "aws_iam_policy" "artifactdb" {
  name = "use-artifact-db-and-update-binary"

  policy = <<POLICY
{
   "Version":"2012-10-17",
   "Statement":[
      {
         "Effect":"Allow",
         "Action":[
            "s3:ListBucket"
         ],
         "Resource":"arn:aws-us-gov:s3:::vpc-automation-artifact-db"
      },
      {
         "Effect":"Allow",
         "Action":[
            "s3:PutObject",
            "s3:GetObject"
         ],
         "Resource":"arn:aws-us-gov:s3:::vpc-automation-artifact-db/*"
      },
      {
          "Effect": "Allow",
          "Action": [
              "dynamodb:List*",
              "dynamodb:DescribeReservedCapacity*",
              "dynamodb:DescribeLimits",
              "dynamodb:DescribeTimeToLive"
          ],
          "Resource": "*"
      },
      {
          "Effect": "Allow",
          "Action": [
              "dynamodb:BatchGet*",
              "dynamodb:DescribeStream",
              "dynamodb:DescribeTable",
              "dynamodb:Get*",
              "dynamodb:Query",
              "dynamodb:Scan",
              "dynamodb:BatchWrite*",
              "dynamodb:CreateTable",
              "dynamodb:Delete*",
              "dynamodb:Update*",
              "dynamodb:PutItem"
          ],
          "Resource": "arn:aws-us-gov:dynamodb:*:*:table/ArtifactDB"
      }

   ]
}
POLICY
}

resource "aws_iam_role_policy_attachment" "cbc_jenkins_artifactdb" {
  role       = module.common_account.aws_iam_role_cbc_jenkins_id
  policy_arn = aws_iam_policy.artifactdb.arn
}
