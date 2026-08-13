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
data "aws_caller_identity" "current" {

}
