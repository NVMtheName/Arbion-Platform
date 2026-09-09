variable "aws_region" {

  type    = string
  default = "us-east-1"
}
variable "environment" {

  type    = string
  default = "production"
}
variable "vpc_cidr" {

  type    = string
  default = "10.40.0.0/16"
}
variable "availability_zones" {

  type    = list(string)
  default = ["us-east-1a", "us-east-1b"]
  validation {

    condition     = length(var.availability_zones) >= 2
    error_message = "At least two AZs are required."
  }
}
variable "domain_name" {

  type    = string
  default = "arbion.ai"
}
variable "route53_zone_id" {

  type     = string
  default  = null
  nullable = true
}
variable "manage_dns_records" {

  type    = bool
  default = false
}
variable "image_tag" {

  type        = string
  default     = "bootstrap"
  description = "Immutable Git SHA deployment workflow registers release revisions."
}
variable "db_instance_class" {

  type    = string
  default = "db.t4g.medium"
}
variable "db_backup_retention_days" {

  type    = number
  default = 14
}
variable "cache_node_type" {

  type    = string
  default = "cache.t4g.small"
}
variable "nat_gateway_per_az" {

  type    = bool
  default = true
}
variable "service_scaling" {

  type = map(object({
    min = number, max = number, cpu = number, memory = number
  }))
  default = {
    web = {
      min = 2, max = 6, cpu = 60, memory = 70
      }, api = {
      min = 2, max = 8, cpu = 60, memory = 70
      }, ai = {
      min = 2, max = 6, cpu = 65, memory = 75
    }
  }
}
variable "alarm_email" {

  type     = string
  default  = null
  nullable = true
}
variable "security_log_retention_days" {

  type        = number
  default     = 365
  description = "CloudTrail security log retention in CloudWatch Logs."
  validation {

    condition     = contains([365, 400, 545, 731, 1096, 1827, 2192, 2557, 2922, 3288, 3653], var.security_log_retention_days)
    error_message = "Security logs must use a supported CloudWatch retention of at least one year."
  }
}
variable "audit_evidence_retention_days" {

  type        = number
  default     = 2555
  description = "Governance-mode object retention for CloudTrail and AWS Config evidence."
  validation {

    condition     = var.audit_evidence_retention_days >= 365
    error_message = "Audit evidence must be retained for at least 365 days."
  }
}
variable "tags" {

  type = map(string)
  default = {

  }
}
