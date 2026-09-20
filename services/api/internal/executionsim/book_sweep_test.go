package executionsim

import (
	"errors"
	"math/big"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func bookSweepFixture() BookSweepInput {
	return BookSweepInput{
		Scope: fixtureConfig().Scope, Source: BookSweepSyntheticSource, Symbol: "XRP", Side: "BUY", LimitPrice: "41", RemainingQuantity: "2.5",
		Bids:       []BookLevel{{Price: "39", Quantity: "0.5"}, {Price: "38", Quantity: "0.5"}},
		Asks:       []BookLevel{{Price: "40", Quantity: "1"}, {Price: "41", Quantity: "0.5"}, {Price: "42", Quantity: "5"}},
		ObservedAt: fixtureTime.Add(-time.Minute), EvaluatedAt: fixtureTime, MaxAge: time.Minute, FeeBasisPoints: 50,
	}
}

func cloneBookSweepInput(input BookSweepInput) BookSweepInput {
	input.Bids = append([]BookLevel(nil), input.Bids...)
	input.Asks = append([]BookLevel(nil), input.Asks...)
	return input
}

func TestBookSweepExactMultiLevelAndPartialResults(t *testing.T) {
	tests := []struct {
		name, side, quantity, limit, filled, remaining, gross, fees, reason string
		fills                                                               []BookSweepFill
	}{
		{"buy stops at limit", "BUY", "2.5", "41", "1.5000000000", "1.0000000000", "60.5000000000", "0.3025000000", BookSweepLimitReached,
			[]BookSweepFill{{"40.0000000000", "1.0000000000", "40.0000000000", "0.2000000000"}, {"41.0000000000", "0.5000000000", "20.5000000000", "0.1025000000"}}},
		{"sell exhausts depth", "SELL", "1.5", "38", "1.0000000000", "0.5000000000", "38.5000000000", "0.1925000000", BookSweepDepthExhausted,
			[]BookSweepFill{{"39.0000000000", "0.5000000000", "19.5000000000", "0.0975000000"}, {"38.0000000000", "0.5000000000", "19.0000000000", "0.0950000000"}}},
		{"buy partly consumes final level", "BUY", "1.25", "41", "1.2500000000", "0.0000000000", "50.2500000000", "0.2512500000", BookSweepFilled,
			[]BookSweepFill{{"40.0000000000", "1.0000000000", "40.0000000000", "0.2000000000"}, {"41.0000000000", "0.2500000000", "10.2500000000", "0.0512500000"}}},
		{"sell partly consumes final level", "SELL", "0.75", "38", "0.7500000000", "0.0000000000", "29.0000000000", "0.1450000000", BookSweepFilled,
			[]BookSweepFill{{"39.0000000000", "0.5000000000", "19.5000000000", "0.0975000000"}, {"38.0000000000", "0.2500000000", "9.5000000000", "0.0475000000"}}},
		{"buy no marketable levels", "BUY", "1", "39", "0.0000000000", "1.0000000000", "0.0000000000", "0.0000000000", BookSweepLimitReached, []BookSweepFill{}},
		{"sell no marketable levels", "SELL", "1", "40", "0.0000000000", "1.0000000000", "0.0000000000", "0.0000000000", BookSweepLimitReached, []BookSweepFill{}},
		{"buy exhausts depth", "BUY", "7", "43", "6.5000000000", "0.5000000000", "270.5000000000", "1.3525000000", BookSweepDepthExhausted,
			[]BookSweepFill{{"40.0000000000", "1.0000000000", "40.0000000000", "0.2000000000"}, {"41.0000000000", "0.5000000000", "20.5000000000", "0.1025000000"}, {"42.0000000000", "5.0000000000", "210.0000000000", "1.0500000000"}}},
		{"sell stops at limit", "SELL", "1.5", "39", "0.5000000000", "1.0000000000", "19.5000000000", "0.0975000000", BookSweepLimitReached,
			[]BookSweepFill{{"39.0000000000", "0.5000000000", "19.5000000000", "0.0975000000"}}},
		{"filled wins over exhausted depth", "SELL", "1", "38", "1.0000000000", "0.0000000000", "38.5000000000", "0.1925000000", BookSweepFilled,
			[]BookSweepFill{{"39.0000000000", "0.5000000000", "19.5000000000", "0.0975000000"}, {"38.0000000000", "0.5000000000", "19.0000000000", "0.0950000000"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := bookSweepFixture()
			input.Side, input.RemainingQuantity, input.LimitPrice = tc.side, tc.quantity, tc.limit
			before := cloneBookSweepInput(input)
			result, err := SweepBook(input)
			if err != nil {
				t.Fatal(err)
			}
			if result.Scope != input.Scope || result.Source != BookSweepSyntheticSource || result.Symbol != input.Symbol || result.Side != input.Side || result.ObservedAt != input.ObservedAt || result.EvaluatedAt != input.EvaluatedAt || result.FilledQuantity != tc.filled || result.RemainingQuantity != tc.remaining || result.TotalGrossNotional != tc.gross || result.TotalFees != tc.fees || result.StopReason != tc.reason || !reflect.DeepEqual(result.Fills, tc.fills) {
				t.Fatalf("unexpected sweep: %#v", result)
			}
			if !reflect.DeepEqual(input, before) {
				t.Fatal("sweep changed its input book")
			}
			assertBookSweepConservation(t, input, result)
		})
	}
}

func TestBookSweepRejectsInvalidInputAtomically(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*BookSweepInput)
	}{
		{"not simulation", func(v *BookSweepInput) { v.Scope.SimulationOnly = false }},
		{"missing source", func(v *BookSweepInput) { v.Source = "" }},
		{"real market source", func(v *BookSweepInput) { v.Source = "RECORDED_PROVIDER_DATA" }},
		{"empty owner", func(v *BookSweepInput) { v.Scope.Owner = "" }},
		{"invalid owner", func(v *BookSweepInput) { v.Scope.Owner = "owner/name" }},
		{"empty account", func(v *BookSweepInput) { v.Scope.Account = "" }},
		{"invalid account", func(v *BookSweepInput) { v.Scope.Account = "account name" }},
		{"empty run", func(v *BookSweepInput) { v.Scope.Run = "" }},
		{"unsupported provider", func(v *BookSweepInput) { v.Scope.Provider = "other" }},
		{"missing symbol", func(v *BookSweepInput) { v.Symbol = "" }},
		{"lowercase symbol", func(v *BookSweepInput) { v.Symbol = "xrp" }},
		{"invalid side", func(v *BookSweepInput) { v.Side = "buy" }},
		{"zero limit", func(v *BookSweepInput) { v.LimitPrice = "0" }},
		{"zero quantity", func(v *BookSweepInput) { v.RemainingQuantity = "0" }},
		{"negative quantity", func(v *BookSweepInput) { v.RemainingQuantity = "-1" }},
		{"excessive quantity precision", func(v *BookSweepInput) { v.RemainingQuantity = "0.00000000001" }},
		{"overflow quantity", func(v *BookSweepInput) { v.RemainingQuantity = "100000000000000000000" }},
		{"exponent limit", func(v *BookSweepInput) { v.LimitPrice = "4e1" }},
		{"fraction limit", func(v *BookSweepInput) { v.LimitPrice = "80/2" }},
		{"leading zero limit", func(v *BookSweepInput) { v.LimitPrice = "040" }},
		{"NaN limit", func(v *BookSweepInput) { v.LimitPrice = "NaN" }},
		{"missing bids", func(v *BookSweepInput) { v.Bids = nil }},
		{"missing asks", func(v *BookSweepInput) { v.Asks = nil }},
		{"too many bids", func(v *BookSweepInput) { v.Bids = make([]BookLevel, 101) }},
		{"too many asks", func(v *BookSweepInput) { v.Asks = make([]BookLevel, 101) }},
		{"zero bid price", func(v *BookSweepInput) { v.Bids[0].Price = "0" }},
		{"zero ask quantity", func(v *BookSweepInput) { v.Asks[0].Quantity = "0" }},
		{"negative ask price", func(v *BookSweepInput) { v.Asks[0].Price = "-1" }},
		{"malformed bid quantity", func(v *BookSweepInput) { v.Bids[0].Quantity = " 1" }},
		{"malformed ignored ask", func(v *BookSweepInput) { v.Asks[2].Quantity = "1e2" }},
		{"overflow bid price", func(v *BookSweepInput) { v.Bids[0].Price = "100000000000000000000" }},
		{"excessive bid precision", func(v *BookSweepInput) { v.Bids[0].Quantity = "0.00000000001" }},
		{"duplicate asks", func(v *BookSweepInput) { v.Asks[1].Price = "40" }},
		{"numerically duplicate bids", func(v *BookSweepInput) { v.Bids[1].Price = "39.0000000000" }},
		{"unsorted asks", func(v *BookSweepInput) { v.Asks[1], v.Asks[2] = v.Asks[2], v.Asks[1] }},
		{"unsorted bids", func(v *BookSweepInput) { v.Bids[0], v.Bids[1] = v.Bids[1], v.Bids[0] }},
		{"crossed book", func(v *BookSweepInput) { v.Bids[0].Price = "41" }},
		{"locked book", func(v *BookSweepInput) { v.Bids[0].Price = "40.00" }},
		{"invalid unused side", func(v *BookSweepInput) { v.Side = "SELL"; v.Asks[2].Price = "0" }},
		{"zero observation", func(v *BookSweepInput) { v.ObservedAt = time.Time{} }},
		{"zero evaluation", func(v *BookSweepInput) { v.EvaluatedAt = time.Time{} }},
		{"future observation", func(v *BookSweepInput) { v.ObservedAt = v.EvaluatedAt.Add(time.Nanosecond) }},
		{"stale observation", func(v *BookSweepInput) { v.ObservedAt = v.EvaluatedAt.Add(-v.MaxAge - time.Nanosecond) }},
		{"zero age", func(v *BookSweepInput) { v.MaxAge = 0 }},
		{"negative age", func(v *BookSweepInput) { v.MaxAge = -time.Second }},
		{"excessive age", func(v *BookSweepInput) { v.MaxAge = 24*time.Hour + time.Nanosecond }},
		{"excessive fee", func(v *BookSweepInput) { v.FeeBasisPoints = 10_001 }},
		{"overflow fee", func(v *BookSweepInput) { v.FeeBasisPoints = ^uint32(0) }},
		{"gross needs rounding", func(v *BookSweepInput) {
			v.Bids = []BookLevel{{"0.0000000001", "1"}}
			v.Asks = []BookLevel{{"0.0000000002", "1"}}
			v.RemainingQuantity, v.FeeBasisPoints = "0.1", 0
		}},
		{"fee needs rounding", func(v *BookSweepInput) {
			v.Bids = []BookLevel{{"0.5", "1"}}
			v.Asks = []BookLevel{{"1", "1"}}
			v.RemainingQuantity, v.FeeBasisPoints = "0.0000000001", 1
		}},
		{"later fee needs rounding", func(v *BookSweepInput) {
			v.Asks[1].Quantity, v.RemainingQuantity = "0.0000000001", "1.0000000001"
		}},
		{"fill gross overflows", func(v *BookSweepInput) {
			v.Asks = []BookLevel{{"99999999999999999999", "2"}}
			v.LimitPrice, v.RemainingQuantity = "99999999999999999999", "2"
		}},
		{"total gross overflows", func(v *BookSweepInput) {
			v.Asks = []BookLevel{{"60000000000000000000", "1"}, {"60000000000000000001", "1"}}
			v.LimitPrice, v.RemainingQuantity, v.FeeBasisPoints = "99999999999999999999", "2", 0
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := bookSweepFixture()
			tc.mutate(&input)
			before := cloneBookSweepInput(input)
			result, err := SweepBook(input)
			if !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(result, BookSweepResult{}) {
				t.Fatalf("invalid input returned partial evidence: %#v, %v", result, err)
			}
			if !reflect.DeepEqual(input, before) {
				t.Fatal("rejected sweep changed input")
			}
		})
	}
}

func TestBookSweepBoundariesAndStatelessDeterminism(t *testing.T) {
	for _, provider := range []string{"coinbase", "schwab"} {
		for _, fee := range []uint32{0, 10_000} {
			input := bookSweepFixture()
			input.Scope.Provider, input.FeeBasisPoints = provider, fee
			input.ObservedAt = input.EvaluatedAt
			input.MaxAge = 24 * time.Hour
			first, err := SweepBook(input)
			if err != nil {
				t.Fatal(err)
			}
			repeated, err := SweepBook(input)
			if err != nil || !reflect.DeepEqual(first, repeated) {
				t.Fatal("same book and synthetic replay clock must produce the same hypothetical plan")
			}
			if fee == 0 && first.TotalFees != zero() || fee == 10_000 && first.TotalFees != first.TotalGrossNotional {
				t.Fatalf("fee boundary changed exact totals: %#v", first)
			}
			first.Fills[0].Quantity = "999"
			if input.Asks[0].Quantity != "1" || repeated.Fills[0].Quantity != "1.0000000000" {
				t.Fatal("result aliases input or another result")
			}
		}
	}
	input := bookSweepFixture()
	input.Bids, input.Asks = nil, nil
	for index := 0; index < 100; index++ {
		input.Bids = append(input.Bids, BookLevel{strconv.Itoa(100 - index), "1"})
		input.Asks = append(input.Asks, BookLevel{strconv.Itoa(101 + index), "1"})
	}
	input.LimitPrice, input.RemainingQuantity, input.FeeBasisPoints = "200", "100", 0
	result, err := SweepBook(input)
	if err != nil || len(result.Fills) != 100 || result.FilledQuantity != "100.0000000000" || result.TotalGrossNotional != "15050.0000000000" || result.StopReason != BookSweepFilled {
		t.Fatalf("100-level boundary: %#v, %v", result, err)
	}
	input = bookSweepFixture()
	input.Asks = []BookLevel{{"40.0000000000", "1.0000000000"}}
	input.RemainingQuantity, input.FeeBasisPoints = "0.0000000001", 0
	result, err = SweepBook(input)
	if err != nil || result.FilledQuantity != "0.0000000001" || result.TotalGrossNotional != "0.0000000040" {
		t.Fatalf("smallest quantity boundary: %#v, %v", result, err)
	}
	input = bookSweepFixture()
	input.Asks = []BookLevel{{"99999999999999999999.9999999999", "1"}}
	input.LimitPrice, input.RemainingQuantity, input.FeeBasisPoints = input.Asks[0].Price, "1", 0
	result, err = SweepBook(input)
	if err != nil || result.TotalGrossNotional != input.Asks[0].Price {
		t.Fatalf("largest exact amount boundary: %#v, %v", result, err)
	}
	// The synthetic replay clock is explicit; wall-clock time must not change
	// whether an otherwise identical historical or future fixture can run.
	for _, year := range []int{2000, 2099} {
		input = bookSweepFixture()
		input.EvaluatedAt = time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		input.ObservedAt = input.EvaluatedAt.Add(-24 * time.Hour)
		input.MaxAge = 24 * time.Hour
		if _, err := SweepBook(input); err != nil {
			t.Fatalf("explicit replay clock for year %d was rejected: %v", year, err)
		}
	}
}

// Exhaustive bounded cases compare the sweep with a separate integer-quarter
// reference model and check quantity, level, limit, gross, and fee conservation.
func TestBookSweepConservationAcrossBoundedBooks(t *testing.T) {
	for _, side := range []string{"BUY", "SELL"} {
		for quarterQuantity := int64(1); quarterQuantity <= 16; quarterQuantity++ {
			for limit := int64(36); limit <= 43; limit++ {
				for _, fee := range []uint32{0, 50, 10_000} {
					input := bookSweepFixture()
					input.Side, input.LimitPrice, input.FeeBasisPoints = side, strconv.FormatInt(limit, 10), fee
					input.RemainingQuantity = new(big.Rat).SetFrac64(quarterQuantity, 4).FloatString(2)
					input.Bids = []BookLevel{{"39", "1"}, {"38", "1"}, {"37", "1"}}
					input.Asks = []BookLevel{{"40", "1"}, {"41", "1"}, {"42", "1"}}
					result, err := SweepBook(input)
					if err != nil {
						t.Fatalf("side=%s quantity=%s limit=%d fee=%d: %v", side, input.RemainingQuantity, limit, fee, err)
					}
					remaining, grossQuarters := quarterQuantity, int64(0)
					reason := BookSweepDepthExhausted
					for level := int64(0); level < 3; level++ {
						price := 40 + level
						if side == "SELL" {
							price = 39 - level
						}
						if side == "BUY" && price > limit || side == "SELL" && price < limit {
							reason = BookSweepLimitReached
							break
						}
						quantity := min(remaining, 4)
						grossQuarters += quantity * price
						remaining -= quantity
						if remaining == 0 {
							reason = BookSweepFilled
							break
						}
					}
					wantGross := new(big.Rat).SetFrac64(grossQuarters, 4)
					wantFees := new(big.Rat).Mul(wantGross, new(big.Rat).SetFrac64(int64(fee), 10_000))
					if result.RemainingQuantity != new(big.Rat).SetFrac64(remaining, 4).FloatString(10) || result.TotalGrossNotional != wantGross.FloatString(10) || result.TotalFees != wantFees.FloatString(10) || result.StopReason != reason {
						t.Fatalf("integer reference mismatch: input=%#v result=%#v", input, result)
					}
					assertBookSweepConservation(t, input, result)
				}
			}
		}
	}
}

func assertBookSweepConservation(t *testing.T, input BookSweepInput, result BookSweepResult) {
	t.Helper()
	quantity, gross, fees := new(big.Rat), new(big.Rat), new(big.Rat)
	levels := input.Asks
	if input.Side == "SELL" {
		levels = input.Bids
	}
	if len(result.Fills) > len(levels) {
		t.Fatal("invented liquidity levels")
	}
	for index, fill := range result.Fills {
		for _, value := range []string{fill.Price, fill.Quantity, fill.GrossNotional, fill.Fee} {
			if !validDecimal(value) {
				t.Fatalf("noncanonical or out-of-bounds fill: %#v", fill)
			}
		}
		price, units := number(fill.Price), number(fill.Quantity)
		wantGross := new(big.Rat).Mul(price, units)
		wantFee := new(big.Rat).Mul(wantGross, new(big.Rat).SetFrac64(int64(input.FeeBasisPoints), 10_000))
		if units.Sign() <= 0 || units.Cmp(number(levels[index].Quantity)) > 0 || price.Cmp(number(levels[index].Price)) != 0 || wantGross.Cmp(number(fill.GrossNotional)) != 0 || wantFee.Cmp(number(fill.Fee)) != 0 {
			t.Fatalf("level conservation failed: %#v", fill)
		}
		comparison := price.Cmp(number(input.LimitPrice))
		if input.Side == "BUY" && comparison > 0 || input.Side == "SELL" && comparison < 0 {
			t.Fatal("fill crossed its limit")
		}
		quantity.Add(quantity, units)
		gross.Add(gross, wantGross)
		fees.Add(fees, wantFee)
	}
	if quantity.Cmp(number(result.FilledQuantity)) != 0 || new(big.Rat).Add(quantity, number(result.RemainingQuantity)).Cmp(number(input.RemainingQuantity)) != 0 || gross.Cmp(number(result.TotalGrossNotional)) != 0 || fees.Cmp(number(result.TotalFees)) != 0 || number(result.RemainingQuantity).Sign() < 0 {
		t.Fatalf("sweep conservation failed: %#v", result)
	}
}

func FuzzBookSweepExactInputs(f *testing.F) {
	f.Add("BUY", "40", "0.25", uint32(50))
	f.Add("SELL", "38", "1", uint32(0))
	f.Add("BUY", "42", "0.0000000001", uint32(1))
	f.Add("", "NaN", "-1", ^uint32(0))
	f.Fuzz(func(t *testing.T, side, limit, quantity string, fee uint32) {
		input := bookSweepFixture()
		input.Side, input.LimitPrice, input.RemainingQuantity, input.FeeBasisPoints = side, limit, quantity, fee
		before := cloneBookSweepInput(input)
		result, err := SweepBook(input)
		if !reflect.DeepEqual(input, before) {
			t.Fatal("sweep mutated fixture")
		}
		if err != nil {
			if !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(result, BookSweepResult{}) {
				t.Fatal("invalid sweep exposed partial result")
			}
			return
		}
		assertBookSweepConservation(t, input, result)
		repeated, err := SweepBook(input)
		if err != nil || !reflect.DeepEqual(result, repeated) {
			t.Fatalf("nondeterministic result for %#v", input)
		}
	})
}
