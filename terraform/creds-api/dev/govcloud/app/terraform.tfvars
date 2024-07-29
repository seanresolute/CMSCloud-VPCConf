appname = "creds-api"
env     = "dev"
vpc_id  = "vpc-0b4b4cdecaaf89a78"
# note: these are actually shared subnets
private_subnets          = ["subnet-0493eecb422c51e73", "subnet-0859bae1425edc666", "subnet-0f5c851b328d56316"]
private_lb_ingress_cidrs = ["10.0.0.0/8"]
use_public_alb           = false
cert_arn                 = "arn:aws-us-gov:acm:us-gov-west-1:350522771286:certificate/d9475899-ac58-4ca2-a70e-a5b974558a40"
redeploy_iam_role_arn    = "arn:aws-us-gov:iam::350522771286:role/redeploy-creds-api-dev"
replicas                 = 1
is_govcloud              = true
