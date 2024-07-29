variable "appname" {
  type = string
}

variable "env" {
  type = string
}

variable "vpc_id" {
  type = string
}

variable "private_subnets" {
  type = list(string)
}

variable "private_lb_ingress_cidrs" {
  type = list(string)
}

variable "use_public_alb" {
  type = bool
}

variable "cert_arn" {
  type = string
}

variable "redeploy_iam_role_arn" {
  type = string
}

variable "replicas" {
  type = number
}
