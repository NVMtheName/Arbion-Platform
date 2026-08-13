variable "domain_name" {
  type = string
}
variable "zone_id" {
  type    = string
  default = null
}
variable "manage_dns_records" {
  type    = bool
  default = false
}
resource "aws_acm_certificate" "this" {

  domain_name               = var.domain_name
  subject_alternative_names = ["www.${var.domain_name}"]
  validation_method         = "DNS"
  lifecycle {

    create_before_destroy = true
  }
}
locals {

  validation = {
    for dvo in aws_acm_certificate.this.domain_validation_options : dvo.domain_name => {
      name = dvo.resource_record_name, type = dvo.resource_record_type, value = dvo.resource_record_value
    }
  }
}
resource "aws_route53_record" "validation" {

  for_each = var.manage_dns_records && var.zone_id != null ? local.validation : {

  }
  zone_id         = var.zone_id
  name            = each.value.name
  type            = each.value.type
  ttl             = 300
  records         = [each.value.value]
  allow_overwrite = false
}
resource "aws_acm_certificate_validation" "this" {

  certificate_arn         = aws_acm_certificate.this.arn
  validation_record_fqdns = var.manage_dns_records ? [for r in aws_route53_record.validation : r.fqdn] : []
  timeouts {

    create = "45m"
  }
}
output "certificate_arn" {
  value = aws_acm_certificate_validation.this.certificate_arn
}
output "dns_validation_records" {
  value = local.validation
}
output "dns_cutover_managed" {
  value = var.manage_dns_records
}
