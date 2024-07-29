provider "aws" {
  region  = "us-east-1"
  version = "~> 2.70.0"

  allowed_account_ids = ["546085968493"]
}

terraform {
  required_version = "= 0.12.23"

  backend "s3" {
    bucket         = "vpc-conf-automation-prod-east-tfstate"
    key            = "account/state"
    region         = "us-east-1"
    dynamodb_table = "vpc-conf-automation-prod-east-lock-table"
  }
}

module "tagging" {
  source         = "../../modules/tagging"
  component_name = "vpcconf"
  environment    = "prod"
}

module "vpc_conf_account" {
  source = "../../modules/ecs_app_common/account"

  env                  = "prod"
  appname              = "vpc-conf"
  component_name       = "vpcconf"
  additional_app_names = []
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
  bucket = "artifact-db-546085968493-us-east-1"
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
         "Resource":"arn:aws:s3:::artifact-db-546085968493-us-east-1"
      },
      {
         "Effect":"Allow",
         "Action":[
            "s3:PutObject",
            "s3:GetObject"
         ],
         "Resource":"arn:aws:s3:::artifact-db-546085968493-us-east-1/*"
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
          "Resource": "arn:aws:dynamodb:*:*:table/ArtifactDB"
      }

   ]
}
POLICY
}

resource "aws_iam_role_policy_attachment" "cbc_jenkins_artifactdb" {
  role       = module.vpc_conf_account.aws_iam_role_cbc_jenkins_id
  policy_arn = aws_iam_policy.artifactdb.arn
}
