terraform {
  backend "s3" {
    key          = "lightsail-security/terraform.tfstate"
    encrypt      = true
    use_lockfile = true
    # Supply the independently verified existing state bucket, region, and KMS key.
    # Never reuse production/terraform.tfstate or run the target-design apply workflow.
  }
}
