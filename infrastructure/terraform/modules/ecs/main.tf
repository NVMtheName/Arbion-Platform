variable "name" {
  type = string
}
variable "vpc_id" {
  type = string
}
variable "region" {
  type = string
}
variable "app_subnet_ids" {
  type = list(string)
}
variable "security_groups" {
  type = map(string)
}
variable "repository_urls" {
  type = map(string)
}
variable "image_tag" {
  type = string
}
variable "log_groups" {
  type = map(string)
}
variable "secret_arns" {
  type = map(string)
}
variable "db_secret_arn" {
  type = string
}
variable "db_host" {
  type = string
}
variable "db_port" {
  type = number
}
variable "redis_url_secret_arn" {
  type = string
}
variable "web_target_group_arn" {
  type = string
}
variable "api_target_group_arn" {
  type = string
}
variable "scaling" {
  type = map(object({
    min = number, max = number, cpu = number, memory = number
  }))
}
resource "aws_ecs_cluster" "this" {

  name = var.name
  setting {

    name  = "containerInsights"
    value = "enabled"
  }
}
resource "aws_service_discovery_private_dns_namespace" "this" {

  name = "arbion.internal"
  vpc  = var.vpc_id
}
resource "aws_service_discovery_service" "ai" {

  name = "ai"
  dns_config {

    namespace_id = aws_service_discovery_private_dns_namespace.this.id
    dns_records {

      ttl  = 10
      type = "A"
    }
    routing_policy = "MULTIVALUE"
  }
}
data "aws_iam_policy_document" "assume" {

  statement {

    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}
resource "aws_iam_role" "execution" {

  name               = "${var.name}-execution"
  assume_role_policy = data.aws_iam_policy_document.assume.json
}
resource "aws_iam_role_policy_attachment" "execution" {

  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}
resource "aws_iam_role_policy" "secrets" {

  name = "declared-secrets"
  role = aws_iam_role.execution.id
  policy = jsonencode({
    Version = "2012-10-17", Statement = [{
      Effect = "Allow", Action = ["secretsmanager:GetSecretValue"], Resource = concat(values(var.secret_arns), [var.db_secret_arn])
      }, {
      Effect = "Allow", Action = ["kms:Decrypt"], Resource = "*", Condition = {
        StringEquals = {
          "kms:ViaService" = "secretsmanager.${var.region}.amazonaws.com"
        }
      }
    }]
  })
}
resource "aws_iam_role" "task" {

  for_each           = toset(["web", "api", "ai", "migration"])
  name               = "${var.name}-${each.key}-task"
  assume_role_policy = data.aws_iam_policy_document.assume.json
}
locals {

  common_db_secrets = [{
    name = "DATABASE_USER", valueFrom = "${var.db_secret_arn}:username::"
    }, {
    name = "DATABASE_PASSWORD", valueFrom = "${var.db_secret_arn}:password::"
  }]
  definitions = {

    web = {
      repo = "arbion-web", port = 3000, cpu = 512, memory = 1024, command = null, env = [{
        name      = "NODE_ENV", value = "production"
      }], secrets = []
    },
    api = {
      repo = "arbion-api", port = 8080, cpu = 1024, memory = 2048, command = null, env = [{
        name = "ARBION_ENV", value = "production"
        }, {
        name = "PORT", value = "8080"
        }, {
        name = "DATABASE_HOST", value = var.db_host
        }, {
        name = "DATABASE_PORT", value = tostring(var.db_port)
        }, {
        name = "DATABASE_NAME", value = "arbion"
        }, {
        name = "DATABASE_SSLMODE", value = "require"
        }, {
        name = "REDIS_URL", value = ""
        }, {
        name = "AI_SERVICE_URL", value = "http://ai.arbion.internal:8000"
        }, {
        name = "AUTH_ALLOWED_ORIGINS", value = "https://www.arbion.ai"
        }, {
        name = "SCHWAB_REDIRECT_URI", value = "https://www.arbion.ai/api/connections/financial/schwab/callback"
        }], secrets = concat(local.common_db_secrets, [{
          name = "REDIS_URL", valueFrom = var.redis_url_secret_arn
          }, {
          name = "CREDENTIAL_ENCRYPTION_KEY", valueFrom = var.secret_arns["credential-encryption-key"]
          }, {
          name = "AI_INTERNAL_SERVICE_TOKEN", valueFrom = var.secret_arns["ai-internal-service-token"]
          }, {
          name = "SCHWAB_CLIENT_ID", valueFrom = var.secret_arns["schwab-client-id"]
          }, {
          name = "SCHWAB_CLIENT_SECRET", valueFrom = var.secret_arns["schwab-client-secret"]
      }])
    },
    ai = {
      repo = "arbion-ai", port = 8000, cpu = 1024, memory = 2048, command = null, env = [{
        name = "ARBION_ENV", value = "production"
        }], secrets = [{
        name = "AI_INTERNAL_SERVICE_TOKEN", valueFrom = var.secret_arns["ai-internal-service-token"]
      }]
    },
    migration = {
      repo = "arbion-api", port = 0, cpu = 512, memory = 1024, command = ["/migrate"], env = [{
        name = "ARBION_ENV", value = "production"
        }, {
        name = "DATABASE_HOST", value = var.db_host
        }, {
        name = "DATABASE_PORT", value = tostring(var.db_port)
        }, {
        name = "DATABASE_NAME", value = "arbion"
        }, {
        name      = "DATABASE_SSLMODE", value = "require"
      }], secrets = local.common_db_secrets
    }

  }

}
resource "aws_ecs_task_definition" "this" {

  for_each                 = local.definitions
  family                   = "${var.name}-${each.key}"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = each.value.cpu
  memory                   = each.value.memory
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task[each.key].arn
  runtime_platform {

    operating_system_family = "LINUX"
    cpu_architecture        = "X86_64"
  }
  container_definitions = jsonencode([{
    name = each.key, image = "${var.repository_urls[each.value.repo]}:${var.image_tag}", essential = true, command = each.value.command, portMappings = each.value.port == 0 ? [] : [{
      containerPort = each.value.port, protocol = "tcp"
      }], environment = [for e in each.value.env : e if e.value != ""], secrets = each.value.secrets, readonlyRootFilesystem = true, linuxParameters = {
      initProcessEnabled = true
      }, logConfiguration = {
      logDriver = "awslogs", options = {
        "awslogs-group" = var.log_groups[each.key == "migration" ? "migrations" : each.key], "awslogs-region" = var.region, "awslogs-stream-prefix" = "ecs"
      }
      }, healthCheck = each.key == "api" ? {
      command = ["CMD-SHELL", "/healthcheck"], interval = 30, timeout = 5, retries = 3, startPeriod = 20
    } : null
  }])
}
resource "aws_ecs_service" "this" {

  for_each                           = toset(["web", "api", "ai"])
  name                               = "arbion-${each.key}"
  cluster                            = aws_ecs_cluster.this.id
  task_definition                    = aws_ecs_task_definition.this[each.key].arn
  desired_count                      = var.scaling[each.key].min
  launch_type                        = "FARGATE"
  platform_version                   = "LATEST"
  enable_execute_command             = false
  health_check_grace_period_seconds  = each.key == "ai" ? 0 : 60
  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200
  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }
  network_configuration {
    subnets          = var.app_subnet_ids
    security_groups  = [var.security_groups[each.key]]
    assign_public_ip = false
  }
  dynamic "load_balancer" {

    for_each = each.key == "web" ? [var.web_target_group_arn] : each.key == "api" ? [var.api_target_group_arn] : []
    content {
      target_group_arn = load_balancer.value
      container_name   = each.key
      container_port   = each.key == "web" ? 3000 : 8080
    }
  }
  dynamic "service_registries" {
    for_each = each.key == "ai" ? [1] : []
    content {
      registry_arn = aws_service_discovery_service.ai.arn
    }
  }
  lifecycle {
    ignore_changes = [task_definition, desired_count]
  }
}
resource "aws_appautoscaling_target" "this" {

  for_each           = aws_ecs_service.this
  max_capacity       = var.scaling[each.key].max
  min_capacity       = var.scaling[each.key].min
  resource_id        = "service/${aws_ecs_cluster.this.name}/${each.value.name}"
  scalable_dimension = "ecs:service:DesiredCount"
  service_namespace  = "ecs"
}
resource "aws_appautoscaling_policy" "cpu" {

  for_each           = aws_ecs_service.this
  name               = "${each.key}-cpu"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.this[each.key].resource_id
  scalable_dimension = aws_appautoscaling_target.this[each.key].scalable_dimension
  service_namespace  = "ecs"
  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageCPUUtilization"
    }
    target_value       = var.scaling[each.key].cpu
    scale_in_cooldown  = 300
    scale_out_cooldown = 60
  }
}
resource "aws_appautoscaling_policy" "memory" {

  for_each           = aws_ecs_service.this
  name               = "${each.key}-memory"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.this[each.key].resource_id
  scalable_dimension = aws_appautoscaling_target.this[each.key].scalable_dimension
  service_namespace  = "ecs"
  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageMemoryUtilization"
    }
    target_value       = var.scaling[each.key].memory
    scale_in_cooldown  = 300
    scale_out_cooldown = 60
  }
}
output "cluster_name" {
  value = aws_ecs_cluster.this.name
}
output "service_names" {
  value = {
    for k, v in aws_ecs_service.this : k => v.name
  }
}
output "migration_task_definition_arn" {
  value = aws_ecs_task_definition.this["migration"].arn
}
output "execution_role_arn" {
  value = aws_iam_role.execution.arn
}
