-- +goose Up
-- Exact owner confirmation for the initial single-order pilot. These records
-- are deliberately unrelated to proposal-only order_intent_reviews.
CREATE TABLE execution_owner_approvals (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id uuid NOT NULL UNIQUE,
  owner_id uuid NOT NULL,
  request_digest text NOT NULL CHECK(request_digest ~ '^[0-9a-f]{64}$'),
  credential_generation bigint NOT NULL CHECK(credential_generation>0),
  scope text NOT NULL DEFAULT 'EXACT_ORDER_CONFIRM_EACH' CHECK(scope='EXACT_ORDER_CONFIRM_EACH'),
  mfa_method text NOT NULL CHECK(mfa_method='totp'),
  mfa_verified_at timestamptz NOT NULL,
  mfa_enabled_at timestamptz NOT NULL,
  approved_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL CHECK(expires_at>approved_at AND expires_at<=approved_at+interval '5 minutes'),
  UNIQUE(id,order_id,owner_id),
  UNIQUE(owner_id,mfa_verified_at),
  FOREIGN KEY(order_id,owner_id) REFERENCES execution_orders(id,owner_id) ON DELETE RESTRICT
);
CREATE TABLE execution_approval_revocations (
  approval_id uuid PRIMARY KEY REFERENCES execution_owner_approvals(id) ON DELETE RESTRICT,
  revoked_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE execution_authorizations (
  id uuid PRIMARY KEY,
  order_id uuid NOT NULL UNIQUE,
  owner_id uuid NOT NULL,
  approval_id uuid NOT NULL,
  reconciliation_id uuid NOT NULL,
  financial_account_id uuid NOT NULL,
  credential_generation bigint NOT NULL CHECK(credential_generation>0),
  request_digest text NOT NULL CHECK(request_digest ~ '^[0-9a-f]{64}$'),
  preflight jsonb NOT NULL CHECK(jsonb_typeof(preflight)='object' AND octet_length(preflight::text)<=4096),
  risk_evaluation jsonb NOT NULL CHECK(jsonb_typeof(risk_evaluation)='object' AND octet_length(risk_evaluation::text)<=16384),
  checked_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL CHECK(expires_at>checked_at AND expires_at<=checked_at+interval '30 seconds'),
  FOREIGN KEY(approval_id,order_id,owner_id) REFERENCES execution_owner_approvals(id,order_id,owner_id) ON DELETE RESTRICT,
  FOREIGN KEY(order_id,owner_id,financial_account_id) REFERENCES execution_orders(id,owner_id,financial_account_id) ON DELETE RESTRICT,
  FOREIGN KEY(reconciliation_id,owner_id,financial_account_id) REFERENCES portfolio_reconciliations(id,user_id,financial_account_id) ON DELETE RESTRICT
);

-- +goose StatementBegin
CREATE FUNCTION guard_execution_owner_approval() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE generation bigint; factor auth_totp_factors%ROWTYPE; o execution_orders%ROWTYPE;
BEGIN
  generation=lock_execution_claim_controls(NEW.order_id);
  SELECT * INTO STRICT o FROM execution_orders WHERE id=NEW.order_id;
  SELECT * INTO factor FROM auth_totp_factors WHERE user_id=NEW.owner_id FOR SHARE;
  IF NEW.owner_id<>o.owner_id OR NEW.request_digest<>o.request_digest OR NEW.credential_generation<>generation
    OR factor.enabled_at IS NULL OR factor.enabled_at IS DISTINCT FROM NEW.mfa_enabled_at
    OR factor.enabled_at>NEW.mfa_verified_at
    OR factor.updated_at IS DISTINCT FROM NEW.mfa_verified_at
    OR factor.last_used_step NOT BETWEEN floor(extract(epoch FROM NEW.mfa_verified_at)/30)-1 AND floor(extract(epoch FROM NEW.mfa_verified_at)/30)+1
    OR NEW.mfa_verified_at<o.created_at OR NEW.mfa_verified_at>NEW.approved_at
    OR NEW.approved_at-NEW.mfa_verified_at>interval '10 seconds'
    OR NEW.approved_at>clock_timestamp() OR NEW.expires_at<=clock_timestamp()
    OR EXISTS(SELECT 1 FROM execution_dispatch_attempts WHERE order_id=NEW.order_id)
    OR NOT EXISTS(SELECT 1 FROM provider_connections WHERE id=o.provider_connection_id AND encrypted_credential_payload IS NOT NULL AND credential_reference IS NULL) THEN
    RAISE EXCEPTION 'exact execution approval unavailable' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_approval_guard BEFORE INSERT ON execution_owner_approvals FOR EACH ROW EXECUTE FUNCTION guard_execution_owner_approval();

-- An absent revocation is protected by the same approval row that the claimant
-- holds FOR SHARE. Even the first revocation serializes with claim commit.
-- +goose StatementBegin
CREATE FUNCTION guard_execution_approval_revocation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM 1 FROM execution_owner_approvals WHERE id=NEW.approval_id FOR UPDATE;
  NEW.revoked_at=clock_timestamp();
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_approval_revoke_guard BEFORE INSERT ON execution_approval_revocations FOR EACH ROW EXECUTE FUNCTION guard_execution_approval_revocation();
CREATE TRIGGER execution_approval_immutable BEFORE UPDATE OR DELETE ON execution_owner_approvals FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();
CREATE TRIGGER execution_approval_revocation_immutable BEFORE UPDATE OR DELETE ON execution_approval_revocations FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();
CREATE TRIGGER execution_authorization_immutable BEFORE UPDATE OR DELETE ON execution_authorizations FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();

-- Authorization is inseparable from the exact durable attempt. A caller cannot
-- save a reusable ALLOW receipt in one transaction and claim against it later.
-- +goose StatementBegin
CREATE FUNCTION guard_execution_authorization_commit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o execution_orders%ROWTYPE; approval execution_owner_approvals%ROWTYPE;
BEGIN
  SELECT * INTO STRICT o FROM execution_orders WHERE id=NEW.order_id;
  SELECT * INTO STRICT approval FROM execution_owner_approvals WHERE id=NEW.approval_id FOR SHARE;
  IF NEW.request_digest<>o.request_digest OR NEW.request_digest<>approval.request_digest
    OR NEW.credential_generation<>approval.credential_generation OR NEW.expires_at>approval.expires_at
    OR NEW.expires_at<=clock_timestamp()
    OR EXISTS(SELECT 1 FROM execution_approval_revocations WHERE approval_id=NEW.approval_id)
    OR NEW.preflight->>'RequestDigest' IS DISTINCT FROM NEW.request_digest
    OR NEW.preflight->>'AccountID' IS DISTINCT FROM NEW.financial_account_id::text
    OR NEW.preflight->>'ConnectionID' IS DISTINCT FROM o.provider_connection_id::text
    OR NEW.preflight->>'ReconciliationID' IS DISTINCT FROM NEW.reconciliation_id::text
    OR (NEW.preflight->>'CredentialGeneration')::bigint IS DISTINCT FROM NEW.credential_generation
    OR jsonb_typeof(NEW.preflight->'ExpiresAt') IS DISTINCT FROM 'string'
    OR (NEW.preflight->>'ExpiresAt')::timestamptz<NEW.expires_at
    OR NEW.risk_evaluation->>'ID' IS DISTINCT FROM NEW.id::text
    OR NEW.risk_evaluation->>'Decision' IS DISTINCT FROM 'ALLOW'
    OR NEW.risk_evaluation->>'Mode' IS DISTINCT FROM 'MANUAL_PROPOSAL'
    OR NEW.risk_evaluation->>'UserID' IS DISTINCT FROM NEW.owner_id::text
    OR NEW.risk_evaluation->>'AccountID' IS DISTINCT FROM NEW.financial_account_id::text
    OR NEW.risk_evaluation->>'ApprovalRequired' IS DISTINCT FROM 'true'
    OR NEW.risk_evaluation->>'PlatformExecutionAvailable' IS DISTINCT FROM 'false'
    OR (NEW.risk_evaluation->>'Timestamp')::timestamptz IS DISTINCT FROM NEW.checked_at
    OR NOT EXISTS(SELECT 1 FROM execution_dispatch_attempts a WHERE a.order_id=NEW.order_id AND a.owner_id=NEW.owner_id
      AND a.financial_account_id=NEW.financial_account_id AND a.authorization_id=NEW.id
      AND a.credential_generation=NEW.credential_generation AND a.expires_at=NEW.expires_at AND a.claimed_at>=NEW.checked_at) THEN
    RAISE EXCEPTION 'execution authorization does not bind exact claim' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER execution_authorization_commit_guard AFTER INSERT ON execution_authorizations
  DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_execution_authorization_commit();

-- Header insertion is already account-serialized (migration48). Freeze late
-- child INSERTs too while funding completeness and exact quantities are read.
-- +goose StatementBegin
CREATE FUNCTION serialize_execution_funding_position_insert() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM 1 FROM financial_accounts WHERE id=NEW.financial_account_id AND user_id=NEW.user_id FOR NO KEY UPDATE;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_funding_position_serialization BEFORE INSERT ON portfolio_reconciliation_positions
  FOR EACH ROW EXECUTE FUNCTION serialize_execution_funding_position_insert();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM execution_owner_approvals) THEN RAISE EXCEPTION 'cannot remove execution approval history'; END IF;
END $$;
-- +goose StatementEnd
DROP TABLE execution_authorizations;
DROP FUNCTION guard_execution_authorization_commit();
DROP TRIGGER execution_funding_position_serialization ON portfolio_reconciliation_positions;
DROP FUNCTION serialize_execution_funding_position_insert();
DROP TABLE execution_approval_revocations;
DROP TABLE execution_owner_approvals;
DROP FUNCTION guard_execution_approval_revocation();
DROP FUNCTION guard_execution_owner_approval();
