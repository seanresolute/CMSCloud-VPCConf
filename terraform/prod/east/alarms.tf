variable east_ipcontrol_alarm_definitions {
  default = {
    "east-dev-test" = {
      Region    = "Commercial/East"
      Zone      = "Development and Test"
      Metric    = "Free IP"
      Threshold = 16384
    }
    "east-impl" = {
      Region    = "Commercial/East"
      Zone      = "Implementation"
      Metric    = "Free IP"
      Threshold = 4096
    }
    "east-lower-app" = {
      Region    = "Commercial/East"
      Zone      = "Lower-App"
      Metric    = "Free IP"
      Threshold = 1024
    }
    "east-lower-data" = {
      Region    = "Commercial/East"
      Zone      = "Lower-Data"
      Metric    = "Free IP"
      Threshold = 8192
    }
    "east-lower-shared" = {
      Region    = "Commercial/East"
      Zone      = "Lower-Shared"
      Metric    = "Free IP"
      Threshold = 1024
    }
    "east-lower-shared-oc" = {
      Region    = "Commercial/East"
      Zone      = "Lower-Shared-OC"
      Metric    = "Free IP"
      Threshold = 1024
    }
    "east-lower-web" = {
      Region    = "Commercial/East"
      Zone      = "Lower-Web"
      Metric    = "Free IP"
      Threshold = 1024
    }
    "east-management" = {
      Region    = "Commercial/East"
      Zone      = "Management"
      Metric    = "Free IP"
      Threshold = 4096
    }
    "east-prod-app" = {
      Region    = "Commercial/East"
      Zone      = "Prod-App"
      Metric    = "Free IP"
      Threshold = 8192
    }
    "east-prod-data" = {
      Region    = "Commercial/East"
      Zone      = "Prod-Data"
      Metric    = "Free IP"
      Threshold = 8192
    }
    "east-prod-shared" = {
      Region    = "Commercial/East"
      Zone      = "Prod-Shared"
      Metric    = "Free IP"
      Threshold = 1024
    }
    "east-prod-shared-oc" = {
      Region    = "Commercial/East"
      Zone      = "Prod-Shared-OC"
      Metric    = "Free IP"
      Threshold = 1024
    }
    "east-prod-web" = {
      Region    = "Commercial/East"
      Zone      = "Prod-Web"
      Metric    = "Free IP"
      Threshold = 512
    }
    "east-prod-legacy" = {
      Region    = "Commercial/East"
      Zone      = "Production"
      Metric    = "Free IP"
      Threshold = 8192
    }
    "east-security" = {
      Region    = "Commercial/East"
      Zone      = "Security"
      Metric    = "Free IP"
      Threshold = 512
    }
    "east-transport" = {
      Region    = "Commercial/East"
      Zone      = "Transport"
      Metric    = "Free IP"
      Threshold = 1024
    }
  }
}

variable west_ipcontrol_alarm_definitions {
  default = {
    "west-dev-test" = {
      Region    = "Commercial/West"
      Zone      = "Development and Test"
      Metric    = "Free IP"
      Threshold = 8192
    }
    "west-impl" = {
      Region    = "Commercial/West"
      Zone      = "Implementation"
      Metric    = "Free IP"
      Threshold = 8192
    }
    "west-lower-app" = {
      Region    = "Commercial/West"
      Zone      = "Lower-App"
      Metric    = "Free IP"
      Threshold = 2048
    }
    "west-lower-data" = {
      Region    = "Commercial/West"
      Zone      = "Lower-Data"
      Metric    = "Free IP"
      Threshold = 2048
    }
    "west-lower-shared" = {
      Region    = "Commercial/West"
      Zone      = "Lower-Shared"
      Metric    = "Free IP"
      Threshold = 256
    }
    # Unused in West
    # "west-lower-shared-oc" = {
    #     Region = "Commercial/West"
    #     Zone = "Lower-Shared-OC"
    #     Metric = "Free IP"
    #     Threshold = 1024
    # }
    "west-lower-web" = {
      Region    = "Commercial/West"
      Zone      = "Lower-Web"
      Metric    = "Free IP"
      Threshold = 512
    }
    "west-management" = {
      Region    = "Commercial/West"
      Zone      = "Management"
      Metric    = "Free IP"
      Threshold = 192
    }
    "west-prod-app" = {
      Region    = "Commercial/West"
      Zone      = "Prod-App"
      Metric    = "Free IP"
      Threshold = 1024
    }
    "west-prod-data" = {
      Region    = "Commercial/West"
      Zone      = "Prod-Data"
      Metric    = "Free IP"
      Threshold = 1536
    }
    "west-prod-shared" = {
      Region    = "Commercial/West"
      Zone      = "Prod-Shared"
      Metric    = "Free IP"
      Threshold = 1024
    }
    # Unused in West
    # "west-prod-shared-oc" = {
    #     Region = "Commercial/West"
    #     Zone = "Prod-Shared-OC"
    #     Metric = "Free IP"
    #     Threshold = 1024
    # }
    "west-prod-web" = {
      Region    = "Commercial/West"
      Zone      = "Prod-Web"
      Metric    = "Free IP"
      Threshold = 512
    }
    "west-prod-legacy" = {
      Region    = "Commercial/West"
      Zone      = "Production"
      Metric    = "Free IP"
      Threshold = 16384
    }
    "west-security" = {
      Region    = "Commercial/West"
      Zone      = "Security"
      Metric    = "Free IP"
      Threshold = 128
    }
    "west-transport" = {
      Region    = "Commercial/West"
      Zone      = "Transport"
      Metric    = "Free IP"
      Threshold = 512
    }
  }
}

variable govwest_ipcontrol_alarm_definitions {
  default = {
    "govwest-dev-test" = {
      Region    = "GovCloud/West"
      Zone      = "Development and Test"
      Metric    = "Free IP"
      Threshold = 4096
    }
    "govwest-impl" = {
      Region    = "GovCloud/West"
      Zone      = "Implementation"
      Metric    = "Free IP"
      Threshold = 8192
    }
    "govwest-lower-app" = {
      Region    = "GovCloud/West"
      Zone      = "Lower-App"
      Metric    = "Free IP"
      Threshold = 1024
    }
    "govwest-lower-data" = {
      Region    = "GovCloud/West"
      Zone      = "Lower-Data"
      Metric    = "Free IP"
      Threshold = 1024
    }
    "govwest-lower-shared" = {
      Region    = "GovCloud/West"
      Zone      = "Lower-Shared"
      Metric    = "Free IP"
      Threshold = 384
    }
    # Unused in GovWest
    # "govwest-lower-shared-oc" = {
    #     Region = "GovCloud/West"
    #     Zone = "Lower-Shared-OC"
    #     Metric = "Free IP"
    #     Threshold = 1024
    # }
    "govwest-lower-web" = {
      Region    = "GovCloud/West"
      Zone      = "Lower-Web"
      Metric    = "Free IP"
      Threshold = 1024
    }
    "govwest-management" = {
      Region    = "GovCloud/West"
      Zone      = "Management"
      Metric    = "Free IP"
      Threshold = 256
    }
    "govwest-prod-app" = {
      Region    = "GovCloud/West"
      Zone      = "Prod-App"
      Metric    = "Free IP"
      Threshold = 1024
    }
    "govwest-prod-data" = {
      Region    = "GovCloud/West"
      Zone      = "Prod-Data"
      Metric    = "Free IP"
      Threshold = 1024
    }
    "govwest-prod-shared" = {
      Region    = "GovCloud/West"
      Zone      = "Prod-Shared"
      Metric    = "Free IP"
      Threshold = 1024
    }
    # Unused in GovWest
    # "govwest-prod-shared-oc" = {
    #     Region = "GovCloud/West"
    #     Zone = "Prod-Shared-OC"
    #     Metric = "Free IP"
    #     Threshold = 1024
    # }
    "govwest-prod-web" = {
      Region    = "GovCloud/West"
      Zone      = "Prod-Web"
      Metric    = "Free IP"
      Threshold = 512
    }
    "govwest-prod-legacy" = {
      Region    = "GovCloud/West"
      Zone      = "Production"
      Metric    = "Free IP"
      Threshold = 8192
    }
    "govwest-security" = {
      Region    = "GovCloud/West"
      Zone      = "Security"
      Metric    = "Free IP"
      Threshold = 128
    }
    "govwest-transport" = {
      Region    = "GovCloud/West"
      Zone      = "Transport"
      Metric    = "Free IP"
      Threshold = 512
    }
  }
}

module "ipcontrol_tagging" {
  source         = "../../modules/tagging"
  component_name = "ipcontrol"
  environment    = "prod"
}

// Topic for alerts
resource "aws_sns_topic" "ipcontrol_alarm" {
  name = "ipcontrol-prod-alarm"
  tags = merge(module.ipcontrol_tagging.common_tags)
}


module "ip_utilization_alarms_east" {
  source = "../../modules/utilization_alarms"

  alarm_definitions   = var.east_ipcontrol_alarm_definitions
  metric_namespace    = "ipcontrol-utilization"
  alarm_sns_topic_arn = aws_sns_topic.ipcontrol_alarm.arn
}

module "ip_utilization_alarms_west" {
  source = "../../modules/utilization_alarms"

  alarm_definitions   = var.west_ipcontrol_alarm_definitions
  metric_namespace    = "ipcontrol-utilization"
  alarm_sns_topic_arn = aws_sns_topic.ipcontrol_alarm.arn
}

module "ip_utilization_alarms_govwest" {
  source = "../../modules/utilization_alarms"

  alarm_definitions   = var.govwest_ipcontrol_alarm_definitions
  metric_namespace    = "ipcontrol-utilization"
  alarm_sns_topic_arn = aws_sns_topic.ipcontrol_alarm.arn
}
