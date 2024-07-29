provider "aws" {
  region  = "us-east-1"
  version = "~> 2.70.0"

  allowed_account_ids = ["921617238787"]
}

terraform {
  required_version = "= 0.12.23"
}

data "aws_caller_identity" "current" {
}

module "tagging" {
  source         = "../../modules/tagging"
  component_name = "prefix-lists"
  environment    = "prod"
}

resource "aws_dynamodb_table" "prefix_lists_lock_table" {
  name           = "prefix-lists-lock-table"
  read_capacity  = 20
  write_capacity = 20
  hash_key       = "LockID"

  attribute {
    name = "LockID"
    type = "S"
  }
  tags = merge(module.tagging.common_tags)
}

resource "aws_s3_bucket" "private_bucket" {
  bucket = "prefix-lists-tfstate"
  acl    = "private"

  versioning {
    enabled = true
  }

  lifecycle_rule {
    enabled = true

    abort_incomplete_multipart_upload_days = 14

    expiration {
      expired_object_delete_marker = true
    }

    noncurrent_version_transition {
      days          = 30
      storage_class = "STANDARD_IA"
    }

    noncurrent_version_expiration {
      days = 365
    }
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

resource "aws_s3_bucket_public_access_block" "public_access_block" {
  bucket = aws_s3_bucket.private_bucket.id

  # Block new public ACLs and uploading public objects
  block_public_acls = true

  # Retroactively remove public access granted through public ACLs
  ignore_public_acls = true

  # Block new public bucket policies
  block_public_policy = true

  # Retroactivley block public and cross-account access if bucket has public policies
  restrict_public_buckets = true
}
