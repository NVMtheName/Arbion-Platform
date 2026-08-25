package strategy

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/arbion/platform/services/api/internal/risk"
)

var (
	ErrInvalid                         = errors.New("invalid strategy input")
	ErrEvaluationInactive              = fmt.Errorf("%w: strategy instance is not active", ErrInvalid)
	ErrEvaluationConfigurationChanged  = fmt.Errorf("%w: bound configuration changed", ErrInvalid)
	ErrEvaluationParametersInvalid     = fmt.Errorf("%w: strategy parameters are invalid", ErrInvalid)
	ErrEvaluationPaperStateUnavailable = fmt.Errorf("%w: paper state is unavailable", ErrInvalid)
	ErrEvaluationMarketDataStale       = fmt.Errorf("%w: market data is stale", ErrInvalid)
	ErrEvaluationNoEligibleContracts   = fmt.Errorf("%w: no eligible option contracts", ErrInvalid)
)

func number(s string) (*big.Rat, bool)   { r, ok := new(big.Rat).SetString(s); return r, ok }
func positive(s string) (*big.Rat, bool) { r, ok := number(s); return r, ok && r.Sign() > 0 }
func decimal(r *big.Rat) string          { return r.FloatString(10) }
func abs(r *big.Rat) *big.Rat {
	if r.Sign() < 0 {
		return new(big.Rat).Neg(r)
	}
	return new(big.Rat).Set(r)
}

func ValidateParameters(p Parameters) error {
	if err := automation.ValidateStrategyParameters(p); err != nil {
		return ErrInvalid
	}
	return nil
}

func ParseParameters(raw json.RawMessage) (Parameters, error) {
	p, err := automation.ParseStrategyParameters(raw)
	if err != nil {
		return Parameters{}, ErrInvalid
	}
	return p, nil
}

// SelectCandidate filters required fields, then ranks by distance to target delta,
// earliest expiration, and lowest strike. No missing market value is fabricated.
func SelectCandidate(now time.Time, p Parameters, kind string, candidates []OptionCandidate) (*OptionCandidate, int, error) {
	if err := ValidateParameters(p); err != nil {
		return nil, 0, err
	}
	target, _ := number(p.TargetDelta)
	lo, _ := number(p.TargetDeltaMin)
	hi, _ := number(p.TargetDeltaMax)
	allowed := map[string]bool{}
	for _, s := range p.Symbols {
		allowed[strings.ToUpper(s)] = true
	}
	type ranked struct {
		c        OptionCandidate
		distance *big.Rat
		expiry   time.Time
		strike   *big.Rat
	}
	var eligible []ranked
	for _, c := range candidates {
		if !allowed[strings.ToUpper(c.Underlying)] || strings.ToUpper(c.OptionType) != kind || c.Delta == nil || c.Bid == nil {
			continue
		}
		d, ok := number(*c.Delta)
		if !ok {
			continue
		}
		d = abs(d)
		bid, bok := number(*c.Bid)
		strike, sok := positive(c.Strike)
		expiry, parseErr := time.Parse("2006-01-02", c.Expiration)
		if !bok || bid.Sign() < 0 || !sok || parseErr != nil || d.Cmp(lo) < 0 || d.Cmp(hi) > 0 {
			continue
		}
		today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
		days := int(expiry.Sub(today).Hours() / 24)
		if days < p.MinimumDTE || days > p.MaximumDTE || (p.MinimumPremium != nil && func() bool { x, _ := number(*p.MinimumPremium); return bid.Cmp(x) < 0 }()) {
			continue
		}
		eligible = append(eligible, ranked{c, abs(new(big.Rat).Sub(d, target)), expiry, strike})
	}
	if len(eligible) == 0 {
		return nil, 0, ErrEvaluationNoEligibleContracts
	}
	sort.Slice(eligible, func(i, j int) bool {
		if x := eligible[i].distance.Cmp(eligible[j].distance); x != 0 {
			return x < 0
		}
		if !eligible[i].expiry.Equal(eligible[j].expiry) {
			return eligible[i].expiry.Before(eligible[j].expiry)
		}
		return eligible[i].strike.Cmp(eligible[j].strike) < 0
	})
	return &eligible[0].c, len(eligible), nil
}

type Engine struct{}

func NewEngine() *Engine { return &Engine{} }
func (e *Engine) Evaluate(instance Instance, in EvaluationInput) (Decision, error) {
	if in.EventID == "" || in.Timestamp.IsZero() || in.Mandate.ID != instance.AutomationMandateID || in.Mandate.Version != instance.MandateVersion || in.Mandate.StrategyIdentifier != instance.StrategyIdentifier || in.PriorState != instance.CurrentState {
		return Decision{}, ErrInvalid
	}
	kind := ""
	proposed := State("")
	switch instance.StrategyIdentifier {
	case "cash_secured_put":
		if instance.CurrentState != ReadyForPut {
			return Decision{}, ErrInvalid
		}
		kind = "PUT"
		proposed = PutProposed
	case "covered_call":
		if instance.CurrentState != ReadyForCall {
			return Decision{}, ErrInvalid
		}
		kind = "CALL"
		proposed = CallProposed
	case "wheel":
		if instance.CurrentState == Cash {
			instance.CurrentState = ReadyForPut
		}
		if instance.CurrentState == ReadyForPut {
			kind = "PUT"
			proposed = PutProposed
		} else if instance.CurrentState == ReadyForCall || instance.CurrentState == LongShares {
			kind = "CALL"
			proposed = CallProposed
		} else {
			return Decision{}, ErrInvalid
		}
	default:
		return Decision{}, ErrInvalid
	}
	c, count, err := SelectCandidate(in.Timestamp, in.Parameters, kind, in.Market.Options)
	if err != nil {
		return Decision{}, err
	}
	contracts := in.Parameters.MaximumContracts
	if contracts > 1 {
		contracts = 1
	} // conservative initial action size
	if kind == "CALL" {
		shares := big.NewRat(0, 1)
		for _, p := range in.Account.Positions {
			if strings.EqualFold(p.Symbol, c.Underlying) && p.Instrument == "EQUITY" {
				q, ok := number(p.Quantity)
				if ok {
					shares.Add(shares, q)
				}
			}
		}
		if shares.Cmp(big.NewRat(int64(contracts*100), 1)) < 0 {
			return Decision{}, ErrInvalid
		}
	}
	strike, _ := number(c.Strike)
	notional := new(big.Rat).Mul(strike, big.NewRat(int64(contracts*100), 1))
	qty := strconv.Itoa(contracts)
	id := instance.ID + ":" + in.EventID
	a := risk.ProposedAction{ID: id, CorrelationID: in.EventID, FinancialAccountID: instance.FinancialAccountID, Source: risk.SourceStrategy, ActionType: risk.ActionOpenOption, MandateID: &instance.AutomationMandateID, MandateVersion: &instance.MandateVersion, Instrument: strings.ToUpper(c.Underlying), Side: "SELL_TO_OPEN", Quantity: qty, Notional: decimal(notional), EstimatedPrice: c.Bid, Option: &risk.OptionContract{Underlying: strings.ToUpper(c.Underlying), Expiration: c.Expiration, PutCall: kind, Strike: c.Strike, ContractMultiplier: 100}, StrategyIdentifier: &instance.StrategyIdentifier, StrategyInstanceID: &instance.ID}
	state := string(instance.CurrentState)
	a.StrategyState = &state
	rationale, _ := json.Marshal(map[string]any{"strategy": instance.StrategyIdentifier, "state": instance.CurrentState, "symbol": c.Underlying, "candidate_count": count, "selected_reason": "closest_to_target_delta_then_expiration_then_strike", "expiration": c.Expiration, "strike": c.Strike, "delta": c.Delta})
	return Decision{ProposedAction: &a, Source: "STRATEGY", InstrumentType: "OPTION", ProposedState: proposed, CandidateCount: count, Selected: c, Reason: "closest_to_target_delta_then_expiration_then_strike", Rationale: rationale}, nil
}

func ApplyLifecycle(identifier string, state State, event LifecycleEvent) (State, error) {
	switch {
	case state == ShortPutOpen && event == ExpireWorthless:
		return ReadyForPut, nil
	case state == ShortPutOpen && event == Assignment:
		if identifier == "wheel" {
			return LongShares, nil
		}
		return Assigned, nil
	case state == ShortCallOpen && event == ExpireWorthless:
		return ReadyForCall, nil
	case state == ShortCallOpen && event == CallAway:
		if identifier == "wheel" {
			return Cash, nil
		}
		return CalledAway, nil
	}
	return state, fmt.Errorf("%w: lifecycle %s from %s", ErrInvalid, event, state)
}
