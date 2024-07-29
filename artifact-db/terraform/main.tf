resource "aws_dynamodb_table" "artifact_db" {
  name           = "ArtifactDB"
  hash_key       = "ProjectName"
  range_key      = "BuildNumber"
  billing_mode   = "PROVISIONED"
  read_capacity  = 1
  write_capacity = 1

  attribute {
    name = "ProjectName"
    type = "S"
  }

  attribute {
    name = "BuildNumber"
    type = "N"
  }
}

# https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/iam-policy-example-data-crud.html
data "aws_iam_policy_document" "client" {
  statement {
    resources = [aws_dynamodb_table.artifact_db.arn]

    actions = [
      "dynamodb:BatchGetItem",
      "dynamodb:BatchWriteItem",
      "dynamodb:ConditionCheckItem",
      "dynamodb:PutItem",
      "dynamodb:DescribeTable",
      "dynamodb:DeleteItem",
      "dynamodb:GetItem",
      "dynamodb:Scan",
      "dynamodb:Query",
      "dynamodb:UpdateItem"
    ]
  }
}

output "dynamodb_table_arn" {
  value = aws_dynamodb_table.artifact_db.arn
}

output "client_iam_policy" {
  value = data.aws_iam_policy_document.client
}
