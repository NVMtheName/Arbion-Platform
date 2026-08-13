variable "name" {
  type = string
}
variable "vpc_id" {
  type = string
}
variable "subnet_ids" {
  type = list(string)
}
variable "certificate_arn" {
  type = string
}
variable "alb_security_group_id" {
  type = string
}
variable "web_port" {
  type = number
}
variable "api_port" {
  type = number
}
variable "domain_name" {
  type = string
}
resource "aws_lb" "this" {

  name                       = var.name
  internal                   = false
  load_balancer_type         = "application"
  security_groups            = [var.alb_security_group_id]
  subnets                    = var.subnet_ids
  enable_deletion_protection = true
  drop_invalid_header_fields = true
}
resource "aws_lb_target_group" "web" {

  name                 = "${var.name}-web"
  port                 = var.web_port
  protocol             = "HTTP"
  vpc_id               = var.vpc_id
  target_type          = "ip"
  deregistration_delay = 30
  health_check {

    path                = "/api/health"
    healthy_threshold   = 2
    unhealthy_threshold = 3
    timeout             = 5
    interval            = 30
    matcher             = "200"
  }
}
resource "aws_lb_target_group" "api" {

  name                 = "${var.name}-api"
  port                 = var.api_port
  protocol             = "HTTP"
  vpc_id               = var.vpc_id
  target_type          = "ip"
  deregistration_delay = 30
  health_check {

    path                = "/readyz"
    healthy_threshold   = 2
    unhealthy_threshold = 3
    timeout             = 5
    interval            = 30
    matcher             = "200"
  }
}
resource "aws_lb_listener" "http" {

  load_balancer_arn = aws_lb.this.arn
  port              = 80
  protocol          = "HTTP"
  default_action {

    type = "redirect"
    redirect {

      protocol    = "HTTPS"
      port        = "443"
      status_code = "HTTP_301"
    }
  }
}
resource "aws_lb_listener" "https" {

  load_balancer_arn = aws_lb.this.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = var.certificate_arn
  default_action {

    type             = "forward"
    target_group_arn = aws_lb_target_group.web.arn
  }
}
resource "aws_lb_listener_rule" "apex" {

  listener_arn = aws_lb_listener.https.arn
  priority     = 10
  action {

    type = "redirect"
    redirect {

      host        = "www.${var.domain_name}"
      protocol    = "HTTPS"
      port        = "443"
      path        = "/#{path}"
      query       = "#{query}"
      status_code = "HTTP_301"
    }
  }
  condition {

    host_header {

      values = [var.domain_name]
    }
  }
}
resource "aws_lb_listener_rule" "api" {

  listener_arn = aws_lb_listener.https.arn
  priority     = 20
  action {

    type             = "forward"
    target_group_arn = aws_lb_target_group.api.arn
  }
  condition {

    path_pattern {

      values = ["/api/*", "/healthz", "/readyz"]
    }
  }
}
output "arn" {
  value = aws_lb.this.arn
}
output "dns_name" {
  value = aws_lb.this.dns_name
}
output "zone_id" {
  value = aws_lb.this.zone_id
}
output "web_target_group_arn" {
  value = aws_lb_target_group.web.arn
}
output "api_target_group_arn" {
  value = aws_lb_target_group.api.arn
}
output "web_target_group_name" {
  value = aws_lb_target_group.web.name
}
output "api_target_group_name" {
  value = aws_lb_target_group.api.name
}
