variable "names" {

  type = set(string)
}
variable "kms_key_arn" {

  type = string
}
resource "aws_ecr_repository" "this" {

  for_each             = var.names
  name                 = each.key
  image_tag_mutability = "IMMUTABLE"
  encryption_configuration {

    encryption_type = "KMS"
    kms_key         = var.kms_key_arn
  }
  image_scanning_configuration {

    scan_on_push = true
  }
}
resource "aws_ecr_lifecycle_policy" "this" {

  for_each   = aws_ecr_repository.this
  repository = each.value.name
  policy = jsonencode({
    rules = [{
      rulePriority = 1, description = "Expire untagged after 7 days", selection = {
        tagStatus = "untagged", countType = "sinceImagePushed", countUnit = "days", countNumber = 7
        }, action = {
        type = "expire"
      }
      }, {
      rulePriority = 2, description = "Keep 50 release images", selection = {
        tagStatus = "any", countType = "imageCountMoreThan", countNumber = 50
        }, action = {
        type = "expire"
      }
    }]
  })
}
output "repository_urls" {

  value = {
    for k, v in aws_ecr_repository.this : k => v.repository_url
  }
}
