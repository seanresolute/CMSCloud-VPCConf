variable "component_name" {
  type    = string
  default = "invalid"
}

variable "environment" {
  type    = string
  default = "invalid"
}

output "common_tags" {
  value = {
    "cms-cloud-environment" = upper(var.environment)
    "cms-cloud-component"   = var.component_name
    "Automated"             = "true"
  }
}