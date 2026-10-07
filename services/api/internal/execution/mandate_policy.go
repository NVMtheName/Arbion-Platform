package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/arbion/platform/services/api/internal/risk"
	"github.com/jackc/pgx/v5"
)

// This explicit adapter is not a promotion of a research mandate. The existing
// automation API still refuses LIVE AI mandates, and no runtime calls this code.
// A future commissioning path must create this exact version and collect fresh
// consent before connecting a scheduler; neither MCP access nor an AI proposal
// is financial authority.
const coinbaseSpotPilotProfile = "COINBASE_SPOT_PILOT_V1"

type spotPilotParameters struct {
	Profile             string `json:"profile"`
	Objective           string `json:"objective"`
	MaxProposalNotional string `json:"max_proposal_notional"`
}

type mandatePolicySnapshot struct {
	FinancialAccountID             string                `json:"financial_account_id"`
	AutomationType                 string                `json:"automation_type"`
	StrategyIdentifier             *string               `json:"strategy_identifier"`
	AIProviderConnectionID         *string               `json:"ai_provider_connection_id"`
	AIModelID                      *string               `json:"ai_model_id"`
	CapitalBucketID                string                `json:"capital_bucket_id"`
	AutonomyLevel                  string                `json:"autonomy_level"`
	ExecutionMode                  string                `json:"execution_mode"`
	Status                         string                `json:"status"`
	StrategyParameters             json.RawMessage       `json:"strategy_parameters"`
	Risk                           automation.RiskPolicy `json:"risk_parameters"`
	AllowedUniverse                automation.Universe   `json:"allowed_universe"`
	ProhibitedUniverse             automation.Universe   `json:"prohibited_universe"`
	MarginAllowed                  bool                  `json:"margin_allowed"`
	OptionsAllowed                 bool                  `json:"options_allowed"`
	ScheduleConditions             json.RawMessage       `json:"schedule_conditions"`
	CapabilityUnverified           bool                  `json:"capability_unverified"`
	PaperOptionsSimulationAttested bool                  `json:"paper_options_simulation_attested"`
	EffectiveFrom                  time.Time             `json:"effective_from"`
	EffectiveUntil                 *time.Time            `json:"effective_until"`
	ExecutionCapable               bool                  `json:"execution_capable"`
}

var executionModelIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$`)

func decodeMandateJSON(body []byte, dst any) bool {
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(dst) != nil {
		return false
	}
	return d.Decode(new(any)) == io.EOF
}

// readCurrentMandate is called only AFTER the existing owner/account/provider/
// bucket locks. No connection lifecycle advisory lock may be acquired here:
// existing writers acquire those before the account. The independent SQL
// equality check binds every current policy field, not merely current_version.
func readCurrentMandate(ctx context.Context, tx pgx.Tx, ownerID, bucketID, mandateID string, version int) (risk.Mandate, string, error) {
	var pilot PilotAllocation
	err := tx.QueryRow(ctx, `SELECT owner_id::text,financial_account_id::text,provider_connection_id::text,
		capital_bucket_id::text,product_id,initial_cash_usd,maximum_order_usd,expires_at
		FROM execution_pilot_allocations WHERE owner_id=$1 AND capital_bucket_id=$2`, ownerID, bucketID).
		Scan(&pilot.OwnerID, &pilot.AccountID, &pilot.ConnectionID, &pilot.CapitalBucketID, &pilot.ProductID,
			&pilot.InitialCashUSD, &pilot.Limits.MaximumOrderUSD, &pilot.Limits.ExpiresAt)
	if err != nil {
		return risk.Mandate{}, "", ErrNotAuthorized
	}
	pilot.Limits.ExpiresAt = pilot.Limits.ExpiresAt.UTC()
	// Discover the immutable version's AI connection, lock it before the current
	// mandate row, and only then reread the current version and all policy fields.
	var aiConnection string
	err = tx.QueryRow(ctx, `SELECT v.snapshot->>'ai_provider_connection_id' FROM automation_mandate_versions v
		JOIN automation_mandates m ON m.id=v.mandate_id
		WHERE m.id=$1 AND m.user_id=$2 AND v.version_number=$3`, mandateID, ownerID, version).Scan(&aiConnection)
	if err != nil || !validUUID(aiConnection) {
		return risk.Mandate{}, "", ErrNotAuthorized
	}
	var provider, status string
	var aiExpiry *time.Time
	err = tx.QueryRow(ctx, `SELECT provider_name,status,authorization_expires_at FROM provider_connections
		WHERE id=$1 AND user_id=$2 AND provider_category='ai' FOR SHARE`, aiConnection, ownerID).Scan(&provider, &status, &aiExpiry)
	if err != nil || provider != "openai" || status != "active" {
		return risk.Mandate{}, "", ErrNotAuthorized
	}
	var body []byte
	var digest string
	var matches bool
	err = tx.QueryRow(ctx, `SELECT v.snapshot,encode(sha256(convert_to(v.snapshot::text,'UTF8')),'hex'),
		execution_mandate_snapshot_matches(m,v.snapshot)
		FROM automation_mandates m JOIN automation_mandate_versions v ON v.mandate_id=m.id AND v.version_number=m.current_version
		WHERE m.id=$1 AND m.user_id=$2 AND m.capital_bucket_id=$3 AND m.current_version=$4 FOR SHARE OF m`,
		mandateID, ownerID, bucketID, version).Scan(&body, &digest, &matches)
	if err != nil || !matches {
		return risk.Mandate{}, "", ErrNotAuthorized
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock_shared(risk_breaker_scope_lock_key('AUTOMATION',$1))`, mandateID); err != nil {
		return risk.Mandate{}, "", err
	}
	var stopped bool
	var now time.Time
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM risk_circuit_breakers WHERE scope='AUTOMATION' AND scope_id=$1 AND state='OPEN'),clock_timestamp()`, mandateID).Scan(&stopped, &now)
	if err != nil || stopped || (aiExpiry != nil && !aiExpiry.After(now)) {
		return risk.Mandate{}, "", ErrNotAuthorized
	}
	m, err := parseSpotPilotMandate(body, pilot, mandateID, version, now)
	if err != nil {
		return risk.Mandate{}, "", err
	}
	// Current AI connection expiry must bound the final send deadline too.
	if aiExpiry != nil && (m.EffectiveUntil == nil || aiExpiry.Before(*m.EffectiveUntil)) {
		m.EffectiveUntil = aiExpiry
	}
	return m, digest, nil
}

func parseSpotPilotMandate(body []byte, p PilotAllocation, id string, version int, now time.Time) (risk.Mandate, error) {
	var s mandatePolicySnapshot
	var params spotPilotParameters
	if !validPilotAllocation(p) || !validUUID(id) || version < 1 || !decodeMandateJSON(body, &s) ||
		!decodeMandateJSON(s.StrategyParameters, &params) || params.Profile != coinbaseSpotPilotProfile ||
		strings.TrimSpace(params.Objective) == "" || len(params.Objective) > 2000 ||
		!sameAmount(params.MaxProposalNotional, p.Limits.MaximumOrderUSD) ||
		s.FinancialAccountID != p.AccountID || s.CapitalBucketID != p.CapitalBucketID ||
		s.AutomationType != "AI_AUTONOMOUS" || s.AutonomyLevel != "FULL_AUTONOMOUS" || s.ExecutionMode != "LIVE" || s.Status != "READY" ||
		s.StrategyIdentifier != nil || s.MarginAllowed || s.OptionsAllowed || s.CapabilityUnverified || s.PaperOptionsSimulationAttested || s.ExecutionCapable ||
		s.AIProviderConnectionID == nil || !validUUID(*s.AIProviderConnectionID) || s.AIModelID == nil || !executionModelIDPattern.MatchString(*s.AIModelID) ||
		s.EffectiveFrom.IsZero() || s.EffectiveFrom.After(now) || (s.EffectiveUntil != nil && !s.EffectiveUntil.After(now)) || !p.Limits.ExpiresAt.After(now) {
		return risk.Mandate{}, ErrNotAuthorized
	}
	symbol := strings.TrimSuffix(p.ProductID, "-USD")
	if len(s.AllowedUniverse.Symbols) != 1 || s.AllowedUniverse.Symbols[0] != symbol ||
		len(s.AllowedUniverse.UniverseIDs) != 0 || len(s.ProhibitedUniverse.UniverseIDs) != 0 {
		return risk.Mandate{}, ErrNotAuthorized
	}
	for _, denied := range s.ProhibitedUniverse.Symbols {
		if strings.EqualFold(denied, symbol) {
			return risk.Mandate{}, ErrNotAuthorized
		}
	}
	// No invented daily P&L: settlement cash movements are not realized losses.
	// Percentage concentration requires post-trade denominator semantics not
	// supplied by this one-asset adapter; reject it rather than silently changing
	// or partially enforcing the saved policy, especially on SELL.
	if s.Risk.MaxDailyLoss != nil || s.Risk.MaxSinglePositionPercentage != nil || s.Risk.MaxTradesPerDay == nil || *s.Risk.MaxTradesPerDay < 1 || *s.Risk.MaxTradesPerDay > 48 {
		return risk.Mandate{}, ErrNotAuthorized
	}
	initial, _ := decimal(p.InitialCashUSD, true)
	for _, v := range []*string{s.Risk.MaxCapitalDeployed, s.Risk.MaxSinglePositionAmount, s.Risk.MinimumCashReserve} {
		if v == nil {
			continue
		}
		n, ok := decimal(*v, v != s.Risk.MinimumCashReserve)
		if !ok || n.Cmp(initial) > 0 {
			return risk.Mandate{}, ErrNotAuthorized
		}
	}
	return risk.Mandate{ID: id, UserID: p.OwnerID, AccountID: p.AccountID, BucketID: p.CapitalBucketID,
		Status: s.Status, AutomationType: s.AutomationType, AutonomyLevel: s.AutonomyLevel, ExecutionMode: s.ExecutionMode, Version: version,
		EffectiveFrom: s.EffectiveFrom, EffectiveUntil: s.EffectiveUntil, AllowedSymbols: s.AllowedUniverse.Symbols,
		ProhibitedSymbols: s.ProhibitedUniverse.Symbols, MaxCapitalDeployed: s.Risk.MaxCapitalDeployed,
		MaxSinglePositionAmount: s.Risk.MaxSinglePositionAmount, MaxSinglePositionPercentage: s.Risk.MaxSinglePositionPercentage,
		MinimumCashReserve: s.Risk.MinimumCashReserve, MaxTradesPerDay: s.Risk.MaxTradesPerDay}, nil
}

type checkedMandateConsent struct {
	approvalID string
	expiresAt  time.Time
	mandate    risk.Mandate
}

func loadMandateConsent(ctx context.Context, tx pgx.Tx, o Order, generation int64) (checkedMandateConsent, error) {
	var result checkedMandateConsent
	if err := tx.QueryRow(ctx, `SELECT check_execution_mandate_consent($1,$2,$3)`, o.Request.MandateApprovalID, o.Request.OwnerID, o.Request.CapitalBucketID).Scan(&result.expiresAt); err != nil {
		return result, mapError(err)
	}
	var id, digest string
	var version int
	err := tx.QueryRow(ctx, `SELECT mandate_id::text,mandate_version,snapshot_digest FROM execution_mandate_approvals
		WHERE id=$1 AND owner_id=$2 AND financial_account_id=$3 AND provider_connection_id=$4 AND capital_bucket_id=$5 AND credential_generation=$6`,
		o.Request.MandateApprovalID, o.Request.OwnerID, o.Request.AccountID, o.Request.ConnectionID, o.Request.CapitalBucketID, generation).Scan(&id, &version, &digest)
	if err != nil {
		return result, ErrNotAuthorized
	}
	m, currentDigest, err := readCurrentMandate(ctx, tx, o.Request.OwnerID, o.Request.CapitalBucketID, id, version)
	if err != nil || digest != currentDigest {
		return result, ErrNotAuthorized
	}
	result.approvalID, result.mandate = o.Request.MandateApprovalID, m
	return result, nil
}

func evaluateMandateFunding(ctx context.Context, tx pgx.Tx, o Order, proof VerifiedPreflight, bucket risk.CapitalBucket, mandate risk.Mandate, claimed bool, now time.Time) (risk.RiskEvaluation, error) {
	var cash, base, initial, modelCap string
	var count int
	var recent, future bool
	// One immutable settlement per order; all attempts count even if no-send,
	// rejected or unknown. Replacing consent/version never resets this history.
	// Exclude only this already-counted original claim during its final check.
	err := tx.QueryRow(ctx, `SELECT b.cash_usd::text,b.base_quantity::text,p.initial_cash_usd,
		v.snapshot->'strategy_parameters'->>'max_proposal_notional',
		(SELECT count(*) FROM execution_dispatch_attempts a WHERE a.financial_account_id=p.financial_account_id
		 AND a.claimed_at >= date_trunc('day',$3::timestamptz AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'
		 AND (NOT $4::boolean OR a.order_id<>$5)),
		EXISTS(SELECT 1 FROM execution_dispatch_attempts a JOIN execution_orders old ON old.id=a.order_id
		 WHERE a.financial_account_id=p.financial_account_id AND old.request->>'Side'=$6
		 AND a.claimed_at>$3::timestamptz-interval '1 hour' AND (NOT $4::boolean OR a.order_id<>$5)),
		EXISTS(SELECT 1 FROM execution_dispatch_attempts a WHERE a.financial_account_id=p.financial_account_id AND a.claimed_at>$3)
		FROM execution_pilot_allocations p CROSS JOIN LATERAL execution_pilot_balance(p.financial_account_id) b
		JOIN automation_mandate_versions v ON v.mandate_id=$7 AND v.version_number=$8
		WHERE p.owner_id=$1 AND p.financial_account_id=$2`, o.Request.OwnerID, o.Request.AccountID, now, claimed, o.ID, o.Request.Side, mandate.ID, mandate.Version).
		Scan(&cash, &base, &initial, &modelCap, &count, &recent, &future)
	if err != nil || recent || future {
		return risk.RiskEvaluation{}, ErrNotAuthorized
	}
	var body []byte
	var portfolio string
	if err = tx.QueryRow(ctx, `SELECT evidence,portfolio_id::text FROM execution_provider_preflights WHERE id=$1 AND order_id=$2 AND request_digest=$3`, proof.EvidenceID, o.ID, o.RequestDigest).Scan(&body, &portfolio); err != nil {
		return risk.RiskEvaluation{}, ErrNotAuthorized
	}
	var p ProviderPreflight
	if !decodeProviderPreflight(body, &p) || validateProviderPreflight(p, o, portfolio, now) != nil {
		return risk.RiskEvaluation{}, ErrNotAuthorized
	}
	return evaluateSpotPilotFunding(o, proof, bucket, mandate, cash, base, initial, modelCap, p.BestAsk, count, now)
}

func evaluateSpotPilotFunding(o Order, proof VerifiedPreflight, bucket risk.CapitalBucket, mandate risk.Mandate, cash, base, initial, modelCap, bestAsk string, count int, now time.Time) (risk.RiskEvaluation, error) {
	values := []string{cash, base, initial, modelCap, bestAsk, proof.AvailableCashUSD, proof.AvailableBase, o.Request.LimitPrice, o.Request.BaseSize, o.Request.FeeAllowanceUSD}
	n := make([]*big.Rat, len(values))
	for i, value := range values {
		var ok bool
		n[i], ok = amount(value)
		if !ok || n[i].Sign() < 0 {
			return risk.RiskEvaluation{}, ErrNotAuthorized
		}
	}
	if n[2].Sign() <= 0 || n[3].Sign() <= 0 || n[4].Sign() <= 0 || n[7].Sign() <= 0 || n[8].Sign() <= 0 || count < 0 {
		return risk.RiskEvaluation{}, ErrNotAuthorized
	}
	availableCash, availableBase := n[0], n[1]
	if n[5].Cmp(availableCash) < 0 {
		availableCash = n[5]
	}
	if n[6].Cmp(availableBase) < 0 {
		availableBase = n[6]
	}
	mark := n[4]
	if n[7].Cmp(mark) > 0 {
		mark = n[7]
	}
	exposure := canonical(new(big.Rat).Mul(n[1], mark))
	notional := new(big.Rat).Mul(n[8], n[7])
	allIn := new(big.Rat).Add(notional, n[9])
	action := risk.ActionSell
	if o.Request.Side == "BUY" {
		action = risk.ActionBuy
		var ok bool
		notional, ok = decimal(o.Request.MaximumDebitUSD, true)
		if !ok {
			return risk.RiskEvaluation{}, ErrNotAuthorized
		}
		allIn = notional
	}
	if allIn.Cmp(n[3]) > 0 {
		return risk.RiskEvaluation{}, ErrNotAuthorized
	}
	// Project only this immutable sub-allocation, never all brokerage cash or
	// pre-existing holdings. SQL already requires the current bucket's protected
	// capacity to cover the entire original allocation before every admission.
	bucket.AllocationType, bucket.AllocationValue, bucket.ProtectedAmount, bucket.AllocationLimit = "FIXED_AMOUNT", initial, "0", nil
	symbol := strings.TrimSuffix(o.Request.ProductID, "-USD")
	c := risk.EvaluationContext{UserID: o.Request.OwnerID, AccountOwned: true, FinancialEntitled: true, AutomationEntitled: true, ConnectionUsable: true,
		Mandate: &mandate, Bucket: &bucket, Now: now, MaxStaleness: 30 * time.Second,
		Account: &risk.AccountRiskSnapshot{AccountID: o.Request.AccountID, Currency: "USD", Timestamp: proof.ObservedAt,
			Cash: cash, AvailableCash: canonical(availableCash), BuyingPower: canonical(availableCash), CurrentExposure: exposure,
			Positions: []risk.Position{{Instrument: symbol, AvailableQuantity: canonical(availableBase), Exposure: exposure}}, Options: risk.CapabilityUnsupported, Margin: risk.CapabilityUnsupported},
		Activity:     &risk.RiskActivitySnapshot{Timestamp: now, ActionsToday: &count},
		Reservations: &risk.CapitalReservationSnapshot{Timestamp: now, AccountReservedCash: "0", BucketReservedCash: "0", TargetInstrument: symbol, TargetReservedQuantity: "0"}}
	return risk.NewEngine().Evaluate(c, risk.ProposedAction{ID: o.ID, CorrelationID: o.Request.ClientOrderID, FinancialAccountID: o.Request.AccountID,
		Source: risk.SourceAI, MandateID: &mandate.ID, MandateVersion: &mandate.Version, ActionType: action, Instrument: symbol,
		Side: o.Request.Side, Quantity: o.Request.BaseSize, Notional: canonical(notional), EstimatedPrice: &o.Request.LimitPrice, CreatedAt: o.CreatedAt}), nil
}
