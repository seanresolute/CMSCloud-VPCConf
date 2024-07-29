variable "appname" {
}

variable "component_name" {
  default = ""
}

variable "additional_app_names" {
  type    = list(string)
  default = []
}

variable "env" {
}

variable "use_rds" {
  default = true
}