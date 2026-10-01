terraform {
  required_version = ">= 1.10.0, < 2.0.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.59"
    }
  }
}

provider "aws" {
  region              = "us-east-1"
  allowed_account_ids = [var.expected_account_id]

  default_tags {
    tags = {
      Project     = "Arbion"
      Environment = "production"
      ManagedBy   = "Terraform"
      Scope       = "lightsail-security"
    }
  }
}
