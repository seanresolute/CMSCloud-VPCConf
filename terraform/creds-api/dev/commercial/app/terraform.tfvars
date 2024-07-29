appname = "creds-api"
env     = "dev"
vpc_id  = "vpc-051197127570a412f"
# note: these are actually shared subnets
private_subnets          = ["subnet-0e850dda260b98871", "subnet-0ed6508c39acfa5b5", "subnet-07f7984785c36aa1e"]
private_lb_ingress_cidrs = ["10.0.0.0/8"]
use_public_alb           = false
cert_arn                 = "arn:aws:acm:us-west-2:346570397073:certificate/0e05560f-f301-415e-b1cf-8b36f411b497"
redeploy_iam_role_arn    = "arn:aws:iam::346570397073:role/redeploy-creds-api-dev"
replicas                 = 1
is_govcloud              = false
