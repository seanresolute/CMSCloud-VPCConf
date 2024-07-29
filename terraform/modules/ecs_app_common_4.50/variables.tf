variable "appname" {
}

variable "component_name" {
  default = ""
}

variable "env" {
}

variable "vpc_id" {
}

variable "cert_arn" {
}

variable "additional_cert_arns" {
  type    = list(string)
  default = []
}

variable "private_subnets" {
  type    = list(string)
  default = []
}

variable "public_subnets" {
  type    = list(string)
  default = []
}

variable "alb_logs_enabled" {
  default = true
}

variable "use_public_alb" {
  default = false
}

variable "private_lb_ingress_cidrs" {
  type    = list(string)
  default = ["10.0.0.0/8"]
}
