terraform {

  backend "s3" {

    key          = "production/terraform.tfstate"
    encrypt      = true
    use_lockfile = true
    # Supply bucket, region, and kms_key_id using backend.hcl created from bootstrap outputs.

  }

}
