// Package executionsim is an offline fixture laboratory, not an execution
// adapter. It cannot access providers, production ledgers, AI, or credentials.
// Its fixture limits are NOT a substitute for Arbion's production risk gate.
package executionsim

import (
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"time"
)

const (
	OpenOrder     = "ORDER_OPENED"
	Send          = "SEND_RECORDED"
	Acknowledge   = "ACKNOWLEDGED"
	Fill          = "FILL_SETTLED"
	RequestCancel = "CANCEL_REQUESTED"
	ConfirmCancel = "CANCEL_CONFIRMED"
	Reject        = "REJECTED"
	Deposit       = "DEPOSIT_SETTLED"
	Withdrawal    = "WITHDRAWAL_SETTLED"
)

var (
	ErrInvalid     = errors.New("invalid simulation evidence")
	ErrConflict    = errors.New("conflicting simulation identity")
	ErrTransition  = errors.New("invalid simulation lifecycle transition")
	ErrLimits      = errors.New("simulation financial boundary exceeded")
	identifier     = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)
	symbolPattern  = regexp.MustCompile(`^[A-Z][A-Z0-9.-]{0,19}$`)
	decimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,19})(\.[0-9]{1,10})?$`)
)

// Scope deliberately uses synthetic identifiers, never connected account IDs.
type Scope struct {
	SimulationOnly bool   `json:"simulation_only"`
	Owner          string `json:"owner"`
	Account        string `json:"account"`
	Provider       string `json:"provider"`
	Run            string `json:"run"`
}

type Config struct {
	Scope            Scope     `json:"scope"`
	StartedAt        time.Time `json:"started_at"`
	StartingCash     string    `json:"starting_cash"`
	CashReserve      string    `json:"cash_reserve"`
	OrderCashCeiling string    `json:"order_cash_ceiling"`
	// A fixed cumulative BUY-spend ceiling; deposits and sale proceeds never
	// increase this fixture authority. This is not a portfolio exposure limit.
	BuySpendCeiling string   `json:"buy_spend_ceiling"`
	Symbols         []string `json:"symbols"`
}

// Events are synthetic normalized fixtures, NOT Coinbase/Schwab wire schemas.
// OrderVersion is a gapless per-order revision, including local lifecycle
// events. TransactionID identifies one economic fill or settled cash movement
// independently of its delivery ID. Amount is a settled USD cash movement;
// fill cash is calculated exactly from Quantity * Price plus/minus Fee.
type Event struct {
	Scope            Scope     `json:"scope"`
	ID               string    `json:"id"`
	Kind             string    `json:"kind"`
	At               time.Time `json:"at"`
	OrderID          string    `json:"order_id,omitempty"`
	OrderVersion     uint64    `json:"order_version,omitempty"`
	SimulatedOrderID string    `json:"simulated_order_id,omitempty"`
	TransactionID    string    `json:"transaction_id,omitempty"`
	Symbol           string    `json:"symbol,omitempty"`
	Side             string    `json:"side,omitempty"`
	Quantity         string    `json:"quantity,omitempty"`
	Price            string    `json:"price,omitempty"`
	Fee              string    `json:"fee,omitempty"`
	Amount           string    `json:"amount,omitempty"`
	CashCeiling      string    `json:"cash_ceiling,omitempty"`
}

type Order struct {
	ID               string    `json:"id"`
	State            string    `json:"state"`
	Version          uint64    `json:"version"`
	LastAt           time.Time `json:"last_at"`
	SimulatedOrderID string    `json:"simulated_order_id,omitempty"`
	Symbol           string    `json:"symbol"`
	Side             string    `json:"side"`
	Quantity         string    `json:"quantity"`
	LimitPrice       string    `json:"limit_price"`
	CashCeiling      string    `json:"cash_ceiling"`
	Filled           string    `json:"filled"`
	Spent            string    `json:"spent"`
	ReservedCash     string    `json:"reserved_cash"`
	ReservedQuantity string    `json:"reserved_quantity"`
}

type Snapshot struct {
	Scope                 Scope             `json:"scope"`
	Cash                  string            `json:"cash"`
	BuySpent              string            `json:"buy_spent"`
	ReservedCash          string            `json:"reserved_cash"`
	FundingReviewRequired bool              `json:"funding_review_required"`
	Positions             map[string]string `json:"positions"`
	Orders                map[string]Order  `json:"orders"`
	AppliedEvents         int               `json:"applied_events"`
	MatchedTransactions   int               `json:"matched_transactions"`
}

type Engine struct {
	config       Config
	state        Snapshot
	events       map[string]string
	transactions map[string]string
}

func New(config Config, now time.Time) (*Engine, error) {
	s := config.Scope
	if _, err := json.Marshal(config); err != nil {
		return nil, ErrInvalid
	}
	if !s.SimulationOnly || !identifier.MatchString(s.Owner) || !identifier.MatchString(s.Account) || !identifier.MatchString(s.Run) || (s.Provider != "coinbase" && s.Provider != "schwab") || config.StartedAt.IsZero() || now.IsZero() || config.StartedAt.After(now) || len(config.Symbols) == 0 || len(config.Symbols) > 20 {
		return nil, ErrInvalid
	}
	for _, raw := range []string{config.StartingCash, config.CashReserve, config.OrderCashCeiling, config.BuySpendCeiling} {
		if !validDecimal(raw) {
			return nil, ErrInvalid
		}
	}
	if number(config.StartingCash).Sign() <= 0 || number(config.OrderCashCeiling).Sign() <= 0 || number(config.BuySpendCeiling).Sign() <= 0 || number(config.CashReserve).Cmp(number(config.StartingCash)) > 0 {
		return nil, ErrInvalid
	}
	seen := map[string]bool{}
	for _, symbol := range config.Symbols {
		if !symbolPattern.MatchString(symbol) || seen[symbol] {
			return nil, ErrInvalid
		}
		seen[symbol] = true
	}
	config.Symbols = append([]string(nil), config.Symbols...)
	e := &Engine{config: config, events: map[string]string{}, transactions: map[string]string{}, state: Snapshot{Scope: s, Cash: fixed(number(config.StartingCash)), BuySpent: zero(), ReservedCash: zero(), Positions: map[string]string{}, Orders: map[string]Order{}}}
	return e, nil
}

// Apply is atomic in memory: invalid/conflicting/out-of-order evidence leaves
// every ledger and identity unchanged. false,nil is an exact duplicate.
func (e *Engine) Apply(event Event, now time.Time) (bool, error) {
	if event.Scope != e.config.Scope || !identifier.MatchString(event.ID) || event.At.IsZero() || now.IsZero() || event.At.Before(e.config.StartedAt) || event.At.After(now) {
		return false, ErrInvalid
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return false, ErrInvalid
	}
	if prior, ok := e.events[event.ID]; ok {
		if prior != string(payload) {
			return false, ErrConflict
		}
		return false, nil
	}
	if len(e.events) >= 10000 {
		return false, ErrInvalid
	}
	// A differently delivered copy of the same economic transaction may not
	// change anything except its delivery ID. Conflicting copies fail closed.
	economic := event
	economic.ID = ""
	transaction, _ := json.Marshal(economic)
	if event.TransactionID != "" {
		if !identifier.MatchString(event.TransactionID) {
			return false, ErrInvalid
		}
		if prior, ok := e.transactions[event.TransactionID]; ok {
			if prior != string(transaction) {
				return false, ErrConflict
			}
			e.events[event.ID] = string(payload)
			return false, nil
		}
	}
	state := e.Snapshot()
	if err := e.reduce(&state, event); err != nil {
		return false, err
	}
	state.AppliedEvents++
	if event.TransactionID != "" {
		state.MatchedTransactions++
	}
	state.ReservedCash = zero()
	for _, order := range state.Orders {
		state.ReservedCash = add(state.ReservedCash, order.ReservedCash)
	}
	state.FundingReviewRequired = number(state.Cash).Cmp(number(add(state.ReservedCash, e.config.CashReserve))) < 0
	e.state = state
	e.events[event.ID] = string(payload)
	if event.TransactionID != "" {
		e.transactions[event.TransactionID] = string(transaction)
	}
	return true, nil
}

func (e *Engine) reduce(s *Snapshot, v Event) error {
	// Reject irrelevant fields instead of silently interpreting mixed facts.
	expected := Event{Scope: v.Scope, ID: v.ID, Kind: v.Kind, At: v.At}
	if v.Kind == Deposit || v.Kind == Withdrawal {
		expected.Amount, expected.TransactionID = v.Amount, v.TransactionID
		if v != expected || v.TransactionID == "" || !positive(v.Amount) {
			return ErrInvalid
		}
		if v.Kind == Deposit {
			s.Cash = add(s.Cash, v.Amount)
		} else {
			if number(v.Amount).Cmp(number(s.Cash)) > 0 {
				return ErrLimits
			}
			s.Cash = subtract(s.Cash, v.Amount)
		}
		// Settled withdrawals can create a funding shortfall. Keep this fact
		// and existing reservations; do not silently free or expand authority.
		return nil
	}
	expected.OrderID, expected.OrderVersion = v.OrderID, v.OrderVersion
	if !identifier.MatchString(v.OrderID) {
		return ErrInvalid
	}
	order, exists := s.Orders[v.OrderID]
	if v.Kind == OpenOrder {
		expected.Symbol, expected.Side, expected.Quantity, expected.Price, expected.CashCeiling = v.Symbol, v.Side, v.Quantity, v.Price, v.CashCeiling
		if v != expected || exists || v.OrderVersion != 1 || !e.allowed(v.Symbol) || (v.Side != "BUY" && v.Side != "SELL") || !positive(v.Quantity) || !positive(v.Price) || !positive(v.CashCeiling) {
			return ErrInvalid
		}
		if s.FundingReviewRequired || number(v.CashCeiling).Cmp(number(e.config.OrderCashCeiling)) > 0 || new(big.Rat).Mul(number(v.Quantity), number(v.Price)).Cmp(number(v.CashCeiling)) > 0 {
			return ErrLimits
		}
		order = Order{ID: v.OrderID, State: "REGISTERED", Symbol: v.Symbol, Side: v.Side, Quantity: fixed(number(v.Quantity)), LimitPrice: fixed(number(v.Price)), CashCeiling: fixed(number(v.CashCeiling)), Filled: zero(), Spent: zero(), ReservedCash: zero(), ReservedQuantity: zero()}
		if v.Side == "BUY" {
			claims := add(s.ReservedCash, v.CashCeiling)
			if number(add(claims, e.config.CashReserve)).Cmp(number(s.Cash)) > 0 || number(add(claims, s.BuySpent)).Cmp(number(e.config.BuySpendCeiling)) > 0 {
				return ErrLimits
			}
			order.ReservedCash = fixed(number(v.CashCeiling))
		} else {
			reserved := zero()
			for _, o := range s.Orders {
				if o.Symbol == v.Symbol {
					reserved = add(reserved, o.ReservedQuantity)
				}
			}
			if number(add(reserved, v.Quantity)).Cmp(number(position(s, v.Symbol))) > 0 {
				return ErrLimits
			}
			order.ReservedQuantity = fixed(number(v.Quantity))
		}
	} else {
		if !exists || v.OrderVersion != order.Version+1 || v.At.Before(order.LastAt) {
			return ErrTransition
		}
		switch v.Kind {
		case Send:
			if order.State != "REGISTERED" || s.FundingReviewRequired {
				return ErrTransition
			}
			// Recording an attempt makes its result UNKNOWN immediately. No
			// resend transition exists, including after process restart.
			order.State = "OUTCOME_UNKNOWN"
		case Acknowledge:
			expected.SimulatedOrderID = v.SimulatedOrderID
			if order.State != "OUTCOME_UNKNOWN" || !identifier.MatchString(v.SimulatedOrderID) {
				return ErrTransition
			}
			for _, other := range s.Orders {
				if other.SimulatedOrderID == v.SimulatedOrderID {
					return ErrConflict
				}
			}
			order.SimulatedOrderID, order.State = v.SimulatedOrderID, "ACKNOWLEDGED"
		case Fill:
			expected.SimulatedOrderID, expected.TransactionID, expected.Quantity, expected.Price, expected.Fee = v.SimulatedOrderID, v.TransactionID, v.Quantity, v.Price, v.Fee
			if (order.State != "ACKNOWLEDGED" && order.State != "PARTIALLY_FILLED" && order.State != "CANCEL_PENDING") || v.SimulatedOrderID != order.SimulatedOrderID || v.TransactionID == "" || !positive(v.Quantity) || !positive(v.Price) || !validDecimal(v.Fee) {
				return ErrTransition
			}
			if err := settleFill(s, &order, v); err != nil {
				return err
			}
		case RequestCancel:
			if order.State != "ACKNOWLEDGED" && order.State != "PARTIALLY_FILLED" {
				return ErrTransition
			}
			order.State = "CANCEL_PENDING"
		case ConfirmCancel:
			expected.SimulatedOrderID = v.SimulatedOrderID
			if order.State != "CANCEL_PENDING" || v.SimulatedOrderID != order.SimulatedOrderID {
				return ErrTransition
			}
			order.State = "CANCELLED"
			order.ReservedCash, order.ReservedQuantity = zero(), zero()
		case Reject:
			if order.State != "OUTCOME_UNKNOWN" {
				return ErrTransition
			}
			order.State = "REJECTED"
			order.ReservedCash, order.ReservedQuantity = zero(), zero()
		default:
			return ErrInvalid
		}
		if v != expected {
			return ErrInvalid
		}
	}
	order.Version, order.LastAt = v.OrderVersion, v.At
	s.Orders[v.OrderID] = order
	return nil
}

func settleFill(s *Snapshot, o *Order, v Event) error {
	filled := add(o.Filled, v.Quantity)
	if number(filled).Cmp(number(o.Quantity)) > 0 {
		return ErrLimits
	}
	gross := new(big.Rat).Mul(number(v.Quantity), number(v.Price))
	// No implicit rounding: this laboratory accepts only exactly
	// representable 10-place amounts. Production rounding is a later adapter.
	if gross.Cmp(number(fixed(gross))) != 0 {
		return ErrInvalid
	}
	if o.Side == "BUY" {
		cost := add(fixed(gross), v.Fee)
		spent := add(o.Spent, cost)
		if number(v.Price).Cmp(number(o.LimitPrice)) > 0 || number(spent).Cmp(number(o.CashCeiling)) > 0 || number(cost).Cmp(number(s.Cash)) > 0 {
			return ErrLimits
		}
		o.Spent = spent
		o.ReservedCash = subtract(o.CashCeiling, spent)
		s.BuySpent, s.Cash = add(s.BuySpent, cost), subtract(s.Cash, cost)
		s.Positions[o.Symbol] = add(position(s, o.Symbol), v.Quantity)
	} else {
		if number(v.Price).Cmp(number(o.LimitPrice)) < 0 || number(v.Quantity).Cmp(number(position(s, o.Symbol))) > 0 || number(v.Fee).Cmp(gross) > 0 {
			return ErrLimits
		}
		s.Cash = add(s.Cash, subtract(fixed(gross), v.Fee))
		s.Positions[o.Symbol] = subtract(position(s, o.Symbol), v.Quantity)
		o.ReservedQuantity = subtract(o.Quantity, filled)
	}
	o.Filled = filled
	if filled == o.Quantity {
		o.State = "FILLED"
		o.ReservedCash, o.ReservedQuantity = zero(), zero()
	} else if o.State != "CANCEL_PENDING" {
		o.State = "PARTIALLY_FILLED"
	}
	return nil
}

func (e *Engine) Snapshot() Snapshot {
	s := e.state
	s.Orders = make(map[string]Order, len(e.state.Orders))
	s.Positions = make(map[string]string, len(e.state.Positions))
	for k, v := range e.state.Orders {
		s.Orders[k] = v
	}
	for k, v := range e.state.Positions {
		s.Positions[k] = v
	}
	return s
}

func (e *Engine) allowed(symbol string) bool {
	for _, s := range e.config.Symbols {
		if symbol == s {
			return true
		}
	}
	return false
}
func validDecimal(s string) bool { return decimalPattern.MatchString(s) }
func positive(s string) bool     { return validDecimal(s) && number(s).Sign() > 0 }
func number(s string) *big.Rat {
	n, ok := new(big.Rat).SetString(s)
	if !ok {
		return new(big.Rat)
	}
	return n
}
func fixed(n *big.Rat) string     { return n.FloatString(10) }
func zero() string                { return "0.0000000000" }
func add(a, b string) string      { return fixed(new(big.Rat).Add(number(a), number(b))) }
func subtract(a, b string) string { return fixed(new(big.Rat).Sub(number(a), number(b))) }
func position(s *Snapshot, symbol string) string {
	if q, ok := s.Positions[symbol]; ok {
		return q
	}
	return zero()
}
