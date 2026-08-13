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
variable "instance_class" {
  type = string
}
variable "backup_retention_days" {
  type = number
}
resource "aws_db_subnet_group" "this" {

  name       = var.name
  subnet_ids = var.subnet_ids
}
resource "aws_db_instance" "this" {

  identifier                      = var.name
  engine                          = "postgres"
  engine_version                  = "16.4"
  instance_class                  = var.instance_class
  allocated_storage               = 50
  max_allocated_storage           = 500
  storage_type                    = "gp3"
  storage_encrypted               = true
  kms_key_id                      = var.kms_key_arn
  db_name                         = "arbion"
  username                        = "arbion_admin"
  manage_master_user_password     = true
  master_user_secret_kms_key_id   = var.kms_key_arn
  db_subnet_group_name            = aws_db_subnet_group.this.name
  vpc_security_group_ids          = [var.security_group_id]
  publicly_accessible             = false
  multi_az                        = true
  backup_retention_period         = var.backup_retention_days
  backup_window                   = "03:00-04:00"
  maintenance_window              = "sun:04:30-sun:05:30"
  deletion_protection             = true
  skip_final_snapshot             = false
  final_snapshot_identifier       = "${var.name}-final"
  copy_tags_to_snapshot           = true
  enabled_cloudwatch_logs_exports = ["postgresql", "upgrade"]
  auto_minor_version_upgrade      = true
  lifecycle {

    prevent_destroy = true
  }
}
output "address" {
  value = aws_db_instance.this.address
}
output "port" {
  value = aws_db_instance.this.port
}
output "master_secret_arn" {
  value = aws_db_instance.this.master_user_secret[0].secret_arn
}
output "identifier" {
  value = aws_db_instance.this.id
}
