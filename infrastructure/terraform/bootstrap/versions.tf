terraform {

  required_version = ">= 1.10.0, < 2.0.0"
  required_providers {

    aws = {
      source = "hashicorp/aws", version = "~> 6.0"
    }
  }

}
provider "aws" {

  region = var.aws_region
  default_tags {

    tags = {
      Project = "Arbion", Environment = "production", ManagedBy = "Terraform"
    }
  }
}
variable "aws_region" {

  type    = string
  default = "us-east-1"
}
variable "state_bucket_name" {

  type = string
}
variable "github_repository" {

  type    = string
  default = "NVMtheName/Arbion-Platform"
}
variable "github_owner_id" {

  type        = string
  description = "Immutable GitHub owner ID used in the repository OIDC subject claim"
  default     = "155460335"
}
variable "github_repository_id" {

  type        = string
  description = "Immutable GitHub repository ID used in the OIDC subject claim"
  default     = "1332347278"
}
data "aws_caller_identity" "current" {

}
