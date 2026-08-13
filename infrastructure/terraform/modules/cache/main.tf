variable "name" {
  type = string
}
variable "subnet_ids" {
  type = list(string)
}
variable "security_group_id" {
  type = string
}
variable "kms_key_arn" {
  type = string
}
variable "node_type" {
  type = string
}
variable "auth_token_secret_arn" {
  type = string
}
data "aws_secretsmanager_secret_version" "auth" {

  secret_id = var.auth_token_secret_arn
}
resource "aws_elasticache_subnet_group" "this" {

  name       = var.name
  subnet_ids = var.subnet_ids
}
resource "aws_elasticache_replication_group" "this" {

  replication_group_id       = var.name
  description                = "Arbion ephemeral coordination cache"
  engine                     = "redis"
  node_type                  = var.node_type
  port                       = 6379
  num_cache_clusters         = 2
  automatic_failover_enabled = true
  multi_az_enabled           = true
  subnet_group_name          = aws_elasticache_subnet_group.this.name
  security_group_ids         = [var.security_group_id]
  transit_encryption_enabled = true
  at_rest_encryption_enabled = true
  kms_key_id                 = var.kms_key_arn
  auth_token                 = data.aws_secretsmanager_secret_version.auth.secret_string
  snapshot_retention_limit   = 1
  apply_immediately          = false
  lifecycle {

    ignore_changes = [auth_token]
  }
}
output "primary_endpoint" {
  value = aws_elasticache_replication_group.this.primary_endpoint_address
}
output "port" {
  value = aws_elasticache_replication_group.this.port
}
output "id" {
  value = aws_elasticache_replication_group.this.id
}
