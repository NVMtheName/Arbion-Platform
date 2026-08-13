variable "name" {

  type = string
}
resource "aws_kms_key" "this" {

  description             = "Arbion production data and secrets"
  enable_key_rotation     = true
  deletion_window_in_days = 30
  lifecycle {

    prevent_destroy = true
  }
}
resource "aws_kms_alias" "this" {

  name          = "alias/${var.name}-data"
  target_key_id = aws_kms_key.this.key_id
}
locals {

  names = toset(["credential-encryption-key", "ai-internal-service-token", "schwab-client-id", "schwab-client-secret", "redis-auth-token", "redis-url"])
}
resource "aws_secretsmanager_secret" "this" {

  for_each                = local.names
  name                    = "${var.name}/${each.key}"
  kms_key_id              = aws_kms_key.this.arn
  recovery_window_in_days = 30
  lifecycle {

    prevent_destroy = true
  }
}
output "kms_key_arn" {

  value = aws_kms_key.this.arn
}
output "secret_arns" {

  value = {
    for k, v in aws_secretsmanager_secret.this : k => v.arn
  }
}
