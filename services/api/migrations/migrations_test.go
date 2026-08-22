package migrations

import (
	"io/fs"
	"strings"
	"testing"
)

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
