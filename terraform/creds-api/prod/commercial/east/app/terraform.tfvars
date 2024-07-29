appname = "creds-api"
env     = "prod"
vpc_id  = "vpc-0e8a39fbb69113f27"
# note: these are actually shared subnets
private_subnets          = ["subnet-0bb11156c6a4656db", "subnet-099eda91026ccccf3", "subnet-049ddc7dc6d86ba01"]
private_lb_ingress_cidrs = ["10.0.0.0/8"]
use_public_alb           = false
cert_arn                 = "arn:aws:acm:us-east-1:546085968493:certificate/560a0581-9346-42c1-8b7b-c826fbd1b259"
redeploy_iam_role_arn    = "arn:aws:iam::546085968493:role/redeploy-creds-api-prod"
replicas                 = 2
