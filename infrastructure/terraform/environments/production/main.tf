locals {


  name = "arbion-${var.environment}"
  ports = {

    web = 3000, api = 8080, ai = 8000

  }

}
module "networking" {


  source             = "../../modules/networking"
  name               = local.name
  vpc_cidr           = var.vpc_cidr
  availability_zones = var.availability_zones
  nat_gateway_per_az = var.nat_gateway_per_az

}
module "secrets" {


  source = "../../modules/secrets"
  name   = local.name

}
module "ecr" {


  source      = "../../modules/ecr"
  names       = toset(["arbion-web", "arbion-api", "arbion-ai"])
  kms_key_arn = module.secrets.kms_key_arn

}
resource "aws_security_group" "alb" {
  name   = "${local.name}-alb"
  vpc_id = module.networking.vpc_id
}
resource "aws_security_group" "web" {
  name   = "${local.name}-web"
  vpc_id = module.networking.vpc_id
}
resource "aws_security_group" "api" {
  name   = "${local.name}-api"
  vpc_id = module.networking.vpc_id
}
resource "aws_security_group" "ai" {
  name   = "${local.name}-ai"
  vpc_id = module.networking.vpc_id
}
resource "aws_security_group" "migration" {
  name   = "${local.name}-migration"
  vpc_id = module.networking.vpc_id
}
resource "aws_security_group" "database" {
  name   = "${local.name}-database"
  vpc_id = module.networking.vpc_id
}
resource "aws_security_group" "cache" {
  name   = "${local.name}-cache"
  vpc_id = module.networking.vpc_id
}
resource "aws_vpc_security_group_ingress_rule" "alb_http" {
  security_group_id = aws_security_group.alb.id
  cidr_ipv4         = "0.0.0.0/0"
  from_port         = 80
  to_port           = 80
  ip_protocol       = "tcp"
}
resource "aws_vpc_security_group_ingress_rule" "alb_https" {
  security_group_id = aws_security_group.alb.id
  cidr_ipv4         = "0.0.0.0/0"
  from_port         = 443
  to_port           = 443
  ip_protocol       = "tcp"
}
resource "aws_vpc_security_group_ingress_rule" "web" {
  security_group_id            = aws_security_group.web.id
  referenced_security_group_id = aws_security_group.alb.id
  from_port                    = 3000
  to_port                      = 3000
  ip_protocol                  = "tcp"
}
resource "aws_vpc_security_group_ingress_rule" "api" {
  security_group_id            = aws_security_group.api.id
  referenced_security_group_id = aws_security_group.alb.id
  from_port                    = 8080
  to_port                      = 8080
  ip_protocol                  = "tcp"
}
resource "aws_vpc_security_group_ingress_rule" "ai" {
  security_group_id            = aws_security_group.ai.id
  referenced_security_group_id = aws_security_group.api.id
  from_port                    = 8000
  to_port                      = 8000
  ip_protocol                  = "tcp"
}
resource "aws_vpc_security_group_ingress_rule" "database_api" {
  security_group_id            = aws_security_group.database.id
  referenced_security_group_id = aws_security_group.api.id
  from_port                    = 5432
  to_port                      = 5432
  ip_protocol                  = "tcp"
}
resource "aws_vpc_security_group_ingress_rule" "database_migration" {
  security_group_id            = aws_security_group.database.id
  referenced_security_group_id = aws_security_group.migration.id
  from_port                    = 5432
  to_port                      = 5432
  ip_protocol                  = "tcp"
}
resource "aws_vpc_security_group_ingress_rule" "cache" {
  security_group_id            = aws_security_group.cache.id
  referenced_security_group_id = aws_security_group.api.id
  from_port                    = 6379
  to_port                      = 6379
  ip_protocol                  = "tcp"
}
locals {
  egress = {
    alb_web = [aws_security_group.alb.id, aws_security_group.web.id, 3000], alb_api = [aws_security_group.alb.id, aws_security_group.api.id, 8080], api_ai = [aws_security_group.api.id, aws_security_group.ai.id, 8000], api_db = [aws_security_group.api.id, aws_security_group.database.id, 5432], api_cache = [aws_security_group.api.id, aws_security_group.cache.id, 6379], migration_db = [aws_security_group.migration.id, aws_security_group.database.id, 5432]
  }
}
resource "aws_vpc_security_group_egress_rule" "scoped" {
  for_each                     = local.egress
  security_group_id            = each.value[0]
  referenced_security_group_id = each.value[1]
  from_port                    = each.value[2]
  to_port                      = each.value[2]
  ip_protocol                  = "tcp"
}
resource "aws_vpc_security_group_egress_rule" "https" {
  for_each = toset(["web", "api", "ai"])
  security_group_id = {
    web = aws_security_group.web.id, api = aws_security_group.api.id, ai = aws_security_group.ai.id
  }[each.key]
  cidr_ipv4   = "0.0.0.0/0"
  from_port   = 443
  to_port     = 443
  ip_protocol = "tcp"
}
module "database" {


  source                = "../../modules/database"
  name                  = "${local.name}-postgres"
  subnet_ids            = module.networking.data_subnet_ids
  security_group_id     = aws_security_group.database.id
  kms_key_arn           = module.secrets.kms_key_arn
  instance_class        = var.db_instance_class
  backup_retention_days = var.db_backup_retention_days

}
module "cache" {


  source                = "../../modules/cache"
  name                  = "${local.name}-redis"
  subnet_ids            = module.networking.data_subnet_ids
  security_group_id     = aws_security_group.cache.id
  kms_key_arn           = module.secrets.kms_key_arn
  node_type             = var.cache_node_type
  auth_token_secret_arn = module.secrets.secret_arns["redis-auth-token"]

}
module "certificate" {


  source             = "../../modules/dns"
  domain_name        = var.domain_name
  zone_id            = var.route53_zone_id
  manage_dns_records = var.manage_dns_records

}
module "load_balancer" {


  source                = "../../modules/load-balancer"
  name                  = local.name
  vpc_id                = module.networking.vpc_id
  subnet_ids            = module.networking.public_subnet_ids
  certificate_arn       = module.certificate.certificate_arn
  alb_security_group_id = aws_security_group.alb.id
  web_port              = local.ports.web
  api_port              = local.ports.api
  domain_name           = var.domain_name

}
resource "aws_route53_record" "site" {


  for_each = var.manage_dns_records && var.route53_zone_id != null ? toset([var.domain_name, "www.${var.domain_name}"]) : toset([])
  zone_id  = var.route53_zone_id
  name     = each.key
  type     = "A"
  alias {

    name                   = module.load_balancer.dns_name
    zone_id                = module.load_balancer.zone_id
    evaluate_target_health = true

  }
  allow_overwrite = false

}
module "observability" {


  source        = "../../modules/observability"
  name          = local.name
  alarm_email   = var.alarm_email
  cluster_name  = local.name
  service_names = toset(["arbion-web", "arbion-api", "arbion-ai"])
  db_identifier = module.database.identifier
  cache_id      = module.cache.id

}
module "ecs" {


  source         = "../../modules/ecs"
  name           = local.name
  vpc_id         = module.networking.vpc_id
  region         = var.aws_region
  app_subnet_ids = module.networking.app_subnet_ids
  security_groups = {

    web = aws_security_group.web.id, api = aws_security_group.api.id, ai = aws_security_group.ai.id, migration = aws_security_group.migration.id

  }
  repository_urls      = module.ecr.repository_urls
  image_tag            = var.image_tag
  log_groups           = module.observability.log_group_names
  secret_arns          = module.secrets.secret_arns
  db_secret_arn        = module.database.master_secret_arn
  db_host              = module.database.address
  db_port              = module.database.port
  redis_url_secret_arn = module.secrets.secret_arns["redis-url"]
  web_target_group_arn = module.load_balancer.web_target_group_arn
  api_target_group_arn = module.load_balancer.api_target_group_arn
  scaling              = var.service_scaling

}
