resource "aws_kms_key" "state" {

  description             = "Arbion Terraform state"
  enable_key_rotation     = true
  deletion_window_in_days = 30
  lifecycle {

    prevent_destroy = true
  }
}
resource "aws_kms_alias" "state" {

  name          = "alias/arbion-terraform-state"
  target_key_id = aws_kms_key.state.key_id
}
resource "aws_s3_bucket" "state" {

  bucket = var.state_bucket_name
  lifecycle {

    prevent_destroy = true
  }
}
resource "aws_s3_bucket_versioning" "state" {

  bucket = aws_s3_bucket.state.id
  versioning_configuration {

    status = "Enabled"
  }
}
resource "aws_s3_bucket_server_side_encryption_configuration" "state" {

  bucket = aws_s3_bucket.state.id
  rule {

    apply_server_side_encryption_by_default {

      kms_master_key_id = aws_kms_key.state.arn
      sse_algorithm     = "aws:kms"
    }
    bucket_key_enabled = true
  }
}
resource "aws_s3_bucket_public_access_block" "state" {

  bucket                  = aws_s3_bucket.state.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}
resource "aws_s3_bucket_policy" "state" {

  bucket = aws_s3_bucket.state.id
  policy = jsonencode({
    Version = "2012-10-17", Statement = [{
      Sid = "DenyInsecureTransport", Effect = "Deny", Principal = "*", Action = "s3:*", Resource = [aws_s3_bucket.state.arn, "${aws_s3_bucket.state.arn}/*"], Condition = {
        Bool = {
          "aws:SecureTransport" = "false"
        }
      }
    }]
  })
}
resource "aws_iam_openid_connect_provider" "github" {

  url             = "https://token.actions.githubusercontent.com"
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = ["6938fd4d98bab03faadb97b34396831e3780aea1"]
}
locals {

  oidc_subjects = {
    plan = "repo:${var.github_repository}:pull_request", apply = "repo:${var.github_repository}:environment:production", deploy = "repo:${var.github_repository}:environment:production"
  }
}
data "aws_iam_policy_document" "github" {

  for_each = local.oidc_subjects
  statement {

    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {

      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github.arn]
    }
    condition {

      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }
    condition {

      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values   = [each.value]
    }
  }
}
resource "aws_iam_role" "github" {

  for_each             = local.oidc_subjects
  name                 = "Arbion${title(each.key)}Role"
  assume_role_policy   = data.aws_iam_policy_document.github[each.key].json
  max_session_duration = 3600
}
resource "aws_iam_role_policy" "plan_state" {

  name = "state-read"
  role = aws_iam_role.github["plan"].id
  policy = jsonencode({
    Version = "2012-10-17", Statement = [{
      Effect = "Allow", Action = ["s3:GetObject", "s3:ListBucket"], Resource = [aws_s3_bucket.state.arn, "${aws_s3_bucket.state.arn}/*"]
      }, {
      Effect = "Allow", Action = ["kms:Decrypt", "kms:DescribeKey"], Resource = aws_kms_key.state.arn
    }]
  })
}
# Some infrastructure service create/list APIs cannot be resource-scoped. IAM role mutation,
# managed-policy attachment, role passing, and Terraform state access are constrained below.
resource "aws_iam_role_policy" "apply" {

  name = "arbion-infrastructure"
  role = aws_iam_role.github["apply"].id
  policy = jsonencode({
    Version = "2012-10-17", Statement = [
      {
        Sid = "InfrastructureServices", Effect = "Allow", Action = ["ec2:*", "ecs:*", "ecr:*", "elasticloadbalancing:*", "rds:*", "elasticache:*", "servicediscovery:*", "logs:*", "cloudwatch:*", "secretsmanager:*", "kms:*", "acm:*", "route53:*"], Resource = "*"
      },
      {
        Sid = "IamRead", Effect = "Allow", Action = ["iam:Get*", "iam:List*"], Resource = "*"
      },
      {
        Sid = "ManageArbionRoles", Effect = "Allow", Action = ["iam:CreateRole", "iam:DeleteRole", "iam:UpdateAssumeRolePolicy", "iam:PutRolePolicy", "iam:DeleteRolePolicy", "iam:TagRole", "iam:UntagRole"], Resource = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:role/arbion-production-*"
      },
      {
        Sid = "ManageArbionExecutionPolicy", Effect = "Allow", Action = ["iam:AttachRolePolicy", "iam:DetachRolePolicy"], Resource = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:role/arbion-production-*", Condition = {
          ArnEquals = {
            "iam:PolicyARN" = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
          }
        }
      },
      {
        Sid = "PassArbionRolesToEcsTasks", Effect = "Allow", Action = "iam:PassRole", Resource = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:role/arbion-production-*", Condition = {
          StringEquals = {
            "iam:PassedToService" = "ecs-tasks.amazonaws.com"
          }
        }
      },
      {
        Sid = "TerraformStateBucket", Effect = "Allow", Action = "s3:ListBucket", Resource = aws_s3_bucket.state.arn
      },
      {
        Sid = "TerraformStateObjects", Effect = "Allow", Action = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"], Resource = "${aws_s3_bucket.state.arn}/*"
      }
    ]
  })
}
resource "aws_iam_role_policy" "deploy" {

  name = "arbion-application-deploy"
  role = aws_iam_role.github["deploy"].id
  policy = jsonencode({
    Version = "2012-10-17", Statement = [{
      Effect = "Allow", Action = ["ecr:GetAuthorizationToken"], Resource = "*"
      }, {
      Effect = "Allow", Action = ["ecr:BatchCheckLayerAvailability", "ecr:CompleteLayerUpload", "ecr:GetDownloadUrlForLayer", "ecr:InitiateLayerUpload", "ecr:PutImage", "ecr:UploadLayerPart"], Resource = "arn:aws:ecr:${var.aws_region}:${data.aws_caller_identity.current.account_id}:repository/arbion-*"
      }, {
      Effect = "Allow", Action = ["ecs:Describe*", "ecs:List*", "ecs:RegisterTaskDefinition", "ecs:RunTask", "ecs:UpdateService"], Resource = "*"
      }, {
      Effect = "Allow", Action = "iam:PassRole", Resource = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:role/arbion-production-*", Condition = {
        StringEquals = {
          "iam:PassedToService" = "ecs-tasks.amazonaws.com"
        }
      }
    }]
  })
}
output "state_bucket" {

  value = aws_s3_bucket.state.id
}
output "state_kms_key_arn" {

  value = aws_kms_key.state.arn
}
output "github_role_arns" {

  value = {
    for k, v in aws_iam_role.github : k => v.arn
  }
}
