# VPC Conf Infrastructure as Code

This is the terraform for VPC Conf infrastructure and related microservices. There are many terraform environments that all need to be individually initialized ‘terraform init’ and applied ‘terraform apply’.
However, they often share the same modules to avoid copy pasting the same code.

The core VPC application is in `./init`, `./dev`, and `./prod`

The creds-api folder contains terraform specific to the creds-api microservice.

The ‘init’ folders generally are just run once per account and set up the state terraform S3 bucket and dynamo-db lock table.

The ‘account’ folders contain resources that are global and not associated with any AWS region.

The application code is either in an ‘app’ folder, for example creds-api app east would be in `./creds-api/prod/commercial/east/app` or implicitly the deepest directory in the path, for example VPC Conf prod east application is located in `./prod/east`.

The first time executing a specific environment, first cd to a directory containing main.tf, and execute `terraform init`.

Once initialized, `terraform plan` will produce what changes would be made should the code be deployed.

`terraform apply` will deploy the infrastructure as code associated with that `main.tf`

