appname = "creds-api"
env     = "prod"
vpc_id  = "vpc-026d1736088070597"
# note: these are actually shared subnets
private_subnets          = ["subnet-0e730c7729382c0e7", "subnet-0c85361f15094a19c", "subnet-088a5f273c7e25d1d"]
private_lb_ingress_cidrs = ["10.0.0.0/8"]
use_public_alb           = false
cert_arn                 = "arn:aws-us-gov:acm:us-gov-west-1:350521122370:certificate/538f8e0f-3aa4-4793-91bc-b5fd7071cc12"
redeploy_iam_role_arn    = "arn:aws-us-gov:iam::350521122370:role/redeploy-creds-api-prod"
replicas                 = 2
