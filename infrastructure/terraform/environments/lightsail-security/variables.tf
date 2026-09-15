variable "expected_account_id" {
  type        = string
  description = "Explicitly reviewed AWS account, verified against the authenticated identity. No default."
  validation {
    condition     = can(regex("^[0-9]{12}$", var.expected_account_id))
    error_message = "Provide the reviewed twelve-digit production account ID."
  }
}

variable "audit_evidence_retention_days" {
  type        = number
  description = "Owner-reviewed governance retention in days, not a legal/compliance retention determination. No default."
  validation {
    condition     = var.audit_evidence_retention_days >= 365 && var.audit_evidence_retention_days <= 3650 && floor(var.audit_evidence_retention_days) == var.audit_evidence_retention_days
    error_message = "Explicit retention must be an integer from 365 through 3650 days."
  }
}

variable "security_log_retention_days" {
  type        = number
  description = "Owner-reviewed CloudWatch log retention. No default."
  validation {
    condition     = contains([365, 400, 545, 731, 1096, 1827, 2192, 2557, 2922, 3288, 3653], var.security_log_retention_days)
    error_message = "Use a supported CloudWatch retention period of at least one year."
  }
}
