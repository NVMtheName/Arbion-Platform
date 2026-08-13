output "alb_dns_name" {

  value = module.load_balancer.dns_name
}
output "acm_dns_validation_records" {

  value = module.certificate.dns_validation_records
}
output "ecr_repository_urls" {

  value = module.ecr.repository_urls
}
output "ecs_cluster_name" {

  value = module.ecs.cluster_name
}
output "ecs_service_names" {

  value = module.ecs.service_names
}
output "migration_task_definition_arn" {

  value = module.ecs.migration_task_definition_arn
}
output "application_secret_arns" {

  value = module.secrets.secret_arns
}
output "private_application_subnets" {

  value = module.networking.app_subnet_ids
}
output "migration_security_group" {

  value = aws_security_group.migration.id
}
output "dns_cutover_enabled" {

  value = var.manage_dns_records
}
