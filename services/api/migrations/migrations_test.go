package migrations

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

func TestEmbeddedMigrationSetContainsOnlyCanonicalVersionedSQL(t *testing.T) {
	entries, err := fs.ReadDir(Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	canonical := regexp.MustCompile(`^[0-9]{5}_[a-z0-9_]+\.sql$`)
	if len(entries) == 0 {
		t.Fatal("embedded migration set is empty")
	}
	for _, entry := range entries {
		if entry.IsDir() || !canonical.MatchString(entry.Name()) {
			t.Fatalf("embedded migration is not canonical: %q", entry.Name())
		}
	}
}

func TestInitialMigrationDefinesRequiredSchema(t *testing.T) {
	body, err := fs.ReadFile(Files, "00001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"-- +goose Up", "CREATE TABLE users", "CREATE TABLE provider_connections", "CREATE TABLE automation_configs", "CREATE TABLE audit_events", "-- +goose Down"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("migration missing %q", required)
		}
	}
}

func TestAuthenticationMigrationIsVersionedAndConstrained(t *testing.T) {
	body, err := fs.ReadFile(Files, "00002_user_authentication.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"ALTER TABLE users", "normalized_email", "password_hash", "email_verified_at", "users_normalized_email_unique_idx"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("authentication migration missing %q", required)
		}
	}
}

func TestFinancialConnectionIdentityMigrationPreventsDuplicateProviderAccounts(t *testing.T) {
	body, err := fs.ReadFile(Files, "00024_financial_connection_identity.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"CREATE UNIQUE INDEX", "financial_accounts_owner_provider_identity_idx", "user_id,provider_name,provider_account_id"} {
		if !strings.Contains(strings.ReplaceAll(string(body), " ", ""), strings.ReplaceAll(required, " ", "")) {
			t.Errorf("financial identity migration missing %q", required)
		}
	}
}

func TestNonLiveStrategyMigrationSeparatesSimulationAndHistory(t *testing.T) {
	body, err := fs.ReadFile(Files, "00008_nonlive_strategy.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"strategy_instances", "strategy_state_transitions", "paper_portfolios", "paper_positions", "nonlive_execution_records", "decision_journal_entries", "WOULD_HAVE_SUBMITTED", "reject_nonlive_history_mutation"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("non-live strategy migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"broker_orders", "SchwabExecutionAdapter", "LIVE')"} {
		if strings.Contains(string(body), prohibited) {
			t.Errorf("non-live strategy migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestAuthEmailTokenMigrationStoresOnlyHashedSingleUseTokens(t *testing.T) {
	body, err := fs.ReadFile(Files, "00009_auth_email_tokens.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"auth_email_tokens", "token_hash bytea", "octet_length(token_hash) = 32", "consumed_at", "auth_email_tokens_one_active_idx", "verify_email", "reset_password"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("email token migration missing %q", required)
		}
	}
	if strings.Contains(string(body), " token text") {
		t.Fatal("email token migration appears to store raw tokens")
	}
}

func TestTOTPMFAMigrationEncryptsSecretsAndHashesRecoveryCodes(t *testing.T) {
	body, err := fs.ReadFile(Files, "00010_totp_mfa.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"auth_totp_factors", "secret_ciphertext bytea", "pending_expires_at", "last_used_step", "auth_mfa_recovery_codes", "code_hash bytea", "used_at"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("MFA migration missing %q", required)
		}
	}
	for _, prohibited := range []string{" secret text", " code text"} {
		if strings.Contains(string(body), prohibited) {
			t.Fatalf("MFA migration appears to store a raw secret: %q", prohibited)
		}
	}
}

func TestDecisionJournalFeedIndexIsOwnerScopedAndStable(t *testing.T) {
	body, err := fs.ReadFile(Files, "00011_decision_journal_feed.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"decision_journal_owner_feed_idx", "user_id,created_at DESC,id DESC"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("decision journal feed migration missing %q", required)
		}
	}
}

func TestNonLiveSchedulerMigrationUsesDurableLeasesAndExactMandateVersions(t *testing.T) {
	body, err := fs.ReadFile(Files, "00012_nonlive_strategy_scheduler.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"nonlive_strategy_schedules", "lease_token", "lease_expires_at", "mandate_id,mandate_version", "US_EQUITIES_REGULAR", "BETWEEN 30 AND 1440"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("non-live scheduler migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"LIVE", "broker_orders"} {
		if strings.Contains(string(body), prohibited) {
			t.Errorf("non-live scheduler migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestAIShadowMigrationExtendsOnlyNonLiveStrategyHistory(t *testing.T) {
	body, err := fs.ReadFile(Files, "00025_ai_shadow_engine.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"ai_shadow", "source IN ('STRATEGY','LIFECYCLE','AI')", "'US_EQUITIES_REGULAR','CONTINUOUS'", "cannot remove AI shadow schema while AI shadow history exists"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("AI shadow migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"broker_orders", "provider_order", "LIVE_EXECUTION", "PlaceOrder"} {
		if strings.Contains(string(body), prohibited) {
			t.Errorf("AI shadow migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestAIShadowOutcomeMigrationIsImmutableAndNonExecuting(t *testing.T) {
	body, err := fs.ReadFile(Files, "00026_ai_shadow_outcomes.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"shadow_execution_outcomes", "ONE_HOUR", "TWENTY_FOUR_HOURS", "directional_change_usd", "pricing_basis", "WOULD_HAVE_SUBMITTED", "enforce_ai_shadow_outcome_source", "shadow_execution_outcomes_immutable", "cannot remove immutable AI shadow outcome history"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("AI shadow outcome migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"provider_order", "broker_order", "LIVE_EXECUTION"} {
		if strings.Contains(string(body), prohibited) {
			t.Errorf("AI shadow outcome migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestPortfolioReconciliationMigrationIsImmutableOwnerScopedAndNonExecuting(t *testing.T) {
	body, err := fs.ReadFile(Files, "00027_portfolio_reconciliation.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"portfolio_reconciliations", "portfolio_reconciliation_positions", "financial_account_id,user_id", "previous_reconciliation_id,user_id,financial_account_id", "BASELINE", "DRIFT_DETECTED", "realized_performance_status", "blocks_new_actions = false", "evidence_hash bytea", "enforce_portfolio_reconciliation_source", "reject_portfolio_reconciliation_mutation", "cannot remove immutable portfolio reconciliation history"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("portfolio reconciliation migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"provider_account_id", "provider_order_id", "private_key", "access_token", "LIVE_EXECUTION"} {
		if strings.Contains(string(body), prohibited) {
			t.Errorf("portfolio reconciliation migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestReconciliationAutonomyGatePreservesLegacyEvidenceAndFailsClosed(t *testing.T) {
	body, err := fs.ReadFile(Files, "00028_reconciliation_autonomy_gate.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"autonomy_enforcement_active", "DEFAULT false", "SET DEFAULT true", "comparison_status <> 'MATCHED'", "cannot remove enforced portfolio reconciliation history"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("reconciliation autonomy migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"provider_order", "broker_order", "LIVE_EXECUTION"} {
		if strings.Contains(string(body), prohibited) {
			t.Errorf("reconciliation autonomy migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestReconciliationChangeImpactPreservesEvidenceAndFailsClosed(t *testing.T) {
	body, err := fs.ReadFile(Files, "00029_reconciliation_change_impact.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"blocking_change_count", "DRIFT_DETECTED", "portfolio_reconciliations_immutable", "enforce_reconciliation_change_impact", "TRADABLE_INVENTORY", "NON_TRADABLE_QUANTITY_ONLY", "cannot remove classified non-tradable reconciliation history"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("reconciliation change-impact migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"provider_order", "broker_order", "LIVE_EXECUTION"} {
		if strings.Contains(string(body), prohibited) {
			t.Errorf("reconciliation change-impact migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestReconciliationNotificationMarkerIsOwnerAccountScopedAndNonExecuting(t *testing.T) {
	body, err := fs.ReadFile(Files, "00030_reconciliation_notification_marker.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"last_reconciliation_notification_id", "last_reconciliation_notification_at", "portfolio_reconciliations", "strategy_instances", "financial_account_id=r.financial_account_id", "DRIFT_DETECTED", "blocking_change_count > 0", "blocks_new_actions=true"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("reconciliation notification migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"provider_order", "broker_order", "LIVE_EXECUTION", "acknowledge_current_drift"} {
		if strings.Contains(string(body), prohibited) {
			t.Errorf("reconciliation notification migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestNonLiveScheduleRunHistoryIsImmutableOwnerScopedAndNonExecuting(t *testing.T) {
	body, err := fs.ReadFile(Files, "00031_nonlive_schedule_run_history.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"nonlive_schedule_runs", "strategy_instance_id,user_id", "mandate_id,mandate_version", "PAPER", "SHADOW", "scheduled_for", "consecutive_failures", "duplicate_recovered", "enforce_nonlive_schedule_run_source", "nonlive_schedule_runs_immutable", "cannot remove immutable non-live schedule run history"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("non-live schedule run migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"provider_order", "broker_order", "submission", "credential", "private_key", "access_token"} {
		if strings.Contains(strings.ToLower(string(body)), prohibited) {
			t.Errorf("non-live schedule run migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestSecurityActivityAuditHistoryIsAppendOnlyAndCredentialFree(t *testing.T) {
	body, err := fs.ReadFile(Files, "00032_immutable_security_activity.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"audit_events_owner_activity_time_idx", "user_id,occurred_at DESC,id DESC", "reject_audit_event_mutation", "audit_events_append_only", "BEFORE UPDATE OR DELETE", "platform audit history is append-only"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("security activity migration missing %q", required)
		}
	}
}

func TestShadowEvidenceReviewIsImmutableOwnerScopedAndNonExecuting(t *testing.T) {
	body, err := fs.ReadFile(Files, "00033_shadow_evidence_reviews.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"shadow_evidence_reviews", "strategy_instance_id,user_id", "mandate_id,mandate_version", "evidence_hash bytea", "octet_length(evidence_hash) = 32", "EVIDENCE_REVIEWABLE", "one_hour_sample_size >= 20", "twenty_four_hour_sample_size >= 20", "evidence_window_hours >= 168", "SHADOW_ONLY", "NON_LIVE_EVIDENCE_ONLY", "mfa_method = 'totp'", "enforce_shadow_evidence_review_source", "shadow_evidence_reviews_immutable", "reject_nonlive_history_mutation", "cannot remove immutable Shadow evidence review history"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("Shadow evidence review migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"provider_order", "broker_order", "submission", "credential", "private_key", "access_token", "execution_authority"} {
		if strings.Contains(strings.ToLower(string(body)), prohibited) {
			t.Errorf("Shadow evidence review migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestPaperLifecycleMigrationIsImmutableOwnerScopedAndNonLive(t *testing.T) {
	body, err := fs.ReadFile(Files, "00013_paper_lifecycle_events.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"strategy_lifecycle_events", "strategy_instance_id,user_id", "EXPIRE_WORTHLESS", "ASSIGNED", "CALLED_AWAY", "strategy_lifecycle_event_immutable", "reject_nonlive_history_mutation"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("paper lifecycle migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"broker_orders", "provider_order", "LIVE_EXECUTION"} {
		if strings.Contains(string(body), prohibited) {
			t.Errorf("paper lifecycle migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestStrategyCapitalBucketBindingIsOwnerScopedAndExclusive(t *testing.T) {
	body, err := fs.ReadFile(Files, "00014_strategy_capital_bucket_binding.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"capital_bucket_id", "strategy_instances_capital_bucket_owner_fkey", "strategy_one_active_bucket_idx", "status IN ('ACTIVE','PAUSED')", "immutable capital bucket binding"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("strategy capital binding migration missing %q", required)
		}
	}
}

func TestNonLiveAccountExclusivityFailsClosedWithoutReservationLedger(t *testing.T) {
	body, err := fs.ReadFile(Files, "00015_nonlive_account_exclusivity.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"strategy_one_active_account_idx", "user_id,financial_account_id", "status IN ('ACTIVE','PAUSED')", "aggregate reservation accounting"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("non-live account exclusivity migration missing %q", required)
		}
	}
}

func TestPaperOptionsSimulationAttestationIsDatabaseConstrained(t *testing.T) {
	body, err := fs.ReadFile(Files, "00016_paper_options_simulation_attestation.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"paper_options_simulation_attested", "automation_type = 'STRATEGY'", "execution_mode = 'PAPER'", "options_allowed"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("PAPER options attestation migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"SHADOW", "LIVE", "broker_order"} {
		if strings.Contains(string(body), prohibited) {
			t.Errorf("PAPER options attestation migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestMarketSourceHealthHistoryIsBoundedAndContainsNoSubjectDimensions(t *testing.T) {
	body, err := fs.ReadFile(Files, "00017_market_source_health_history.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"market_source_health_buckets", "completed_attempts = successes + failures", "date_bin('5 minutes'", "last_observed_at >= bucket_started_at", "last_state IN ('VERIFIED','DEGRADED')", "bucket_started_at DESC,source_id,capability"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("market source health migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"user_id", "account_id", "instrument", "symbol", "provider_request", "raw_error", "request_url"} {
		if strings.Contains(string(body), prohibited) {
			t.Errorf("market source health history contains a prohibited subject dimension: %q", prohibited)
		}
	}
}

func TestFinancialAuthorizationDeadlineTracksWeeklySchwabReauthorization(t *testing.T) {
	body, err := fs.ReadFile(Files, "00023_financial_authorization_deadline.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"authorization_expires_at", "financial.authorization_completed", "interval '7 days'", "provider_name = 'schwab'"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("financial authorization deadline migration missing %q", required)
		}
	}
}

func TestStagedAICredentialRotationIsEncryptedAndConcurrencyBound(t *testing.T) {
	body, err := fs.ReadFile(Files, "00034_staged_ai_credential_rotation.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"credential_generation",
		"pending_encrypted_credential_payload",
		"pending_credential_token",
		"provider_pending_credential_pair_check",
		"octet_length(pending_encrypted_credential_payload) >= 28",
		"^[0-9a-f]{64}$",
	} {
		if !strings.Contains(string(body), required) {
			t.Errorf("staged AI credential rotation migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"pending_plaintext", "pending_api_key", "pending_secret"} {
		if strings.Contains(strings.ToLower(string(body)), prohibited) {
			t.Errorf("staged AI credential rotation migration contains prohibited plaintext field %q", prohibited)
		}
	}
}

func TestMarketWatchlistIsOwnerScopedBoundedAndNonExecutable(t *testing.T) {
	body, err := fs.ReadFile(Files, "00018_market_watchlist.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"market_watchlist_items", "user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE", "asset_class = 'CRYPTO'", "quote_currency = 'USD'", "UNIQUE (user_id,asset_class,symbol,quote_currency)", "market_watchlist_owner_order_idx"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("market watchlist migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"provider_order", "execution", "credential", "private_key", "account_id"} {
		if strings.Contains(strings.ToLower(string(body)), prohibited) {
			t.Errorf("market watchlist migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestOrderIntentFoundationIsOwnerScopedImmutableAndNonExecuting(t *testing.T) {
	body, err := fs.ReadFile(Files, "00019_nonexecuting_order_intents.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"order_intents", "order_intent_previews", "order_intent_reviews", "order_intent_events", "FOREIGN KEY (financial_account_id,user_id)", "USER_APPROVED_NONEXECUTABLE", "PROPOSAL_REVIEW_ONLY", "evidence_hash bytea", "reject_order_intent_evidence_mutation", "enforce_order_intent_transition", "must begin non-approved", "expires_at = previewed_at + interval '1 minute'", "PROVIDER_TRADE_PERMISSION_REQUIRED"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("order intent migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"provider_order_id", "client_order_id", "preview_id", "EXECUTION_APPROVAL", "SUBMITTED", "FILLED"} {
		if strings.Contains(string(body), prohibited) {
			t.Errorf("non-executing order intent migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestOrderIntentProductRulesAreBoundedAndNonExecuting(t *testing.T) {
	body, err := fs.ReadFile(Files, "00020_order_intent_product_rules.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"order_intent_previews", "advanced_trade_product", "base_increment", "quote_increment", "base_min_size", "base_max_size", "quote_min_size", "quote_max_size", "product_market_ioc_enabled", "product_block_reasons", "product_status='ONLINE' OR product_block_reasons", "product_rules_observed_at=previewed_at", "order_intent_product_rules_all_or_none"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("order intent product-rule migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"provider_order_id", "client_order_id", "preview_id", "SUBMITTED", "FILLED"} {
		if strings.Contains(string(body), prohibited) {
			t.Errorf("order intent product-rule migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestManualOrderIntentRiskIsOwnerBoundImmutableAndNonExecuting(t *testing.T) {
	body, err := fs.ReadFile(Files, "00021_manual_order_intent_risk.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"capital_buckets_id_user_account_unique", "order_intents_capital_bucket_owner_account_fkey", "order_intents_id_owner_account_capital_bucket_unique", "risk_evaluations_id_owner_account_unique", "order_intent_risk_evaluations", "manual_coinbase_spot.v1", "risk_evaluations_immutable", "order_intent_risk_evaluations_immutable", "order_intent_capital_policy_guard", "order_intent_previews_block_reasons_check", "proposed_notional", "account_available_cash", "target_available_quantity"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("manual order-intent risk migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"provider_order_id", "client_order_id", "preview_id", "SUBMITTED", "FILLED", "create_order"} {
		if strings.Contains(strings.ToLower(string(body)), strings.ToLower(prohibited)) {
			t.Errorf("manual order-intent risk migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestOrderIntentReservationsAreOwnerBoundImmutableAndNonExecuting(t *testing.T) {
	body, err := fs.ReadFile(Files, "00022_order_intent_capital_reservations.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"capital_reservations", "account_reserved_cash", "bucket_reserved_cash", "target_reserved_quantity", "ORDER_INTENT", "resource_type IN ('CASH','ASSET')", "pg_advisory_xact_lock", "capital reservation snapshot is stale", "capital_reservation_immutable", "reviewable_order_intent_reservation_guard", "DEFERRABLE INITIALLY DEFERRED"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("capital-reservation migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"provider_order_id", "client_order_id", "preview_id", "SUBMITTED", "FILLED", "create_order"} {
		if strings.Contains(strings.ToLower(string(body)), strings.ToLower(prohibited)) {
			t.Errorf("capital-reservation migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestNonliveStrategyReservationsAreDurableAggregateAndNonExecuting(t *testing.T) {
	body, err := fs.ReadFile(Files, "00035_nonlive_strategy_capital_reservations.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"strategy_capital_reservations",
		"strategy_one_active_reservation_bucket_idx",
		"strategy_capital_reservation_account_guard",
		"pg_advisory_xact_lock",
		"account_allocation_limit",
		"UNRESOLVED_LEGACY",
		"strategy_capital_reservation_immutable",
		"strategy_instance_capital_reservation_guard",
		"reserved_capital_bucket_policy_guard",
		"DEFERRABLE INITIALLY DEFERRED",
	} {
		if !strings.Contains(string(body), required) {
			t.Errorf("non-live strategy reservation migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"provider_order_id", "client_order_id", "SUBMITTED", "FILLED", "create_order"} {
		if strings.Contains(strings.ToLower(string(body)), strings.ToLower(prohibited)) {
			t.Errorf("non-live strategy reservation migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestPaperReservationsRemainExactButDoNotConsumeShadowCapitalAuthority(t *testing.T) {
	body, err := fs.ReadFile(Files, "00038_paper_reservation_isolation.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"CREATE OR REPLACE FUNCTION enforce_strategy_capital_reservation_insert()",
		"IF NEW.execution_mode='PAPER' THEN",
		"AND execution_mode='SHADOW' AND released_at IS NULL",
		"AND r.execution_mode='SHADOW' AND r.released_at IS NULL",
		"active Shadow strategy reservations do not fit one explicit account ceiling",
		"cannot restore the legacy aggregate while an active Paper reservation shares an account",
	} {
		if !strings.Contains(string(body), required) {
			t.Errorf("Paper reservation isolation migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"provider_order_id", "client_order_id", "SUBMITTED", "FILLED", "create_order"} {
		if strings.Contains(strings.ToLower(string(body)), strings.ToLower(prohibited)) {
			t.Errorf("Paper reservation isolation migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestAIPaperSpotLedgerIsOwnerBoundImmutableAndNonExecuting(t *testing.T) {
	body, err := fs.ReadFile(Files, "00036_ai_paper_spot_ledger.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"ai_paper_spot_fills",
		"simulation_only boolean NOT NULL DEFAULT true CHECK (simulation_only)",
		"paper_portfolios_ai_paper_owner_unique",
		"FOREIGN KEY (paper_portfolio_id,user_id,strategy_instance_id)",
		"FOREIGN KEY (execution_record_id,user_id,strategy_instance_id)",
		"enforce_ai_paper_spot_fill_source",
		"x.mode='PAPER'",
		"x.status='SIMULATED_FILLED'",
		"i.current_state='AI_MONITORING'",
		"j.source='AI'",
		"ai_paper_spot_fills_immutable",
		"reject_nonlive_history_mutation",
		"cannot remove immutable AI paper spot fill history",
	} {
		if !strings.Contains(string(body), required) {
			t.Errorf("AI PAPER spot ledger migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"provider_order_id", "client_order_id", "'LIVE'", "create_order", "submit_order"} {
		if strings.Contains(strings.ToLower(string(body)), strings.ToLower(prohibited)) {
			t.Errorf("AI PAPER spot ledger migration unexpectedly contains %q", prohibited)
		}
	}
}

func TestAIPaperSpotPositionsAreDistinctAndProviderIndependent(t *testing.T) {
	body, err := fs.ReadFile(Files, "00037_ai_paper_spot_positions.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"instrument IN ('EQUITY','OPTION','CRYPTO')",
		"paper_positions_one_spot_symbol_idx",
		"paper_portfolio_id,symbol,instrument",
		"option_type IS NULL AND strike IS NULL AND expiration IS NULL",
		"cannot remove AI paper crypto position projection",
	} {
		if !strings.Contains(string(body), required) {
			t.Errorf("AI PAPER spot-position migration missing %q", required)
		}
	}
	for _, prohibited := range []string{"provider_order_id", "client_order_id", "'LIVE'", "create_order", "submit_order", "coinbase", "schwab"} {
		if strings.Contains(strings.ToLower(string(body)), strings.ToLower(prohibited)) {
			t.Errorf("AI PAPER spot-position migration unexpectedly contains %q", prohibited)
		}
	}
}
