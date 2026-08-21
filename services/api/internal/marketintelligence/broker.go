package marketintelligence

import (
	"fmt"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

// NormalizeBrokerQuote makes delegated broker market data explicit instead of
// allowing it to masquerade as an independent public feed.
func NormalizeBrokerQuote(provider, currency string, quote financial.Quote, receivedAt time.Time, freshness FreshnessPolicy) (QuoteObservation, error) {
	observation := QuoteObservation{
		Symbol: strings.ToUpper(strings.TrimSpace(quote.Symbol)), AssetClass: Equity,
		Currency: strings.ToUpper(strings.TrimSpace(currency)), Bid: marketDecimal(quote.Bid),
		Ask: marketDecimal(quote.Ask), Mark: marketDecimal(quote.Mark), Last: marketDecimal(quote.Last),
		Provenance: Provenance{
			Provider: strings.ToLower(strings.TrimSpace(provider)), Role: BrokerAuthority,
			Feed: "broker_entitled", Quality: entitlementQuality(quote.Realtime, false),
			ProviderTimestamp: quote.ProviderTimestamp.UTC(), ReceivedAt: receivedAt.UTC(),
		},
	}
	if err := ValidateQuote(observation, receivedAt.UTC(), freshness); err != nil {
		return QuoteObservation{}, err
	}
	return observation, nil
}

func NormalizeBrokerOptionChain(provider string, chain financial.OptionChain, receivedAt time.Time, freshness FreshnessPolicy) (OptionChainObservation, error) {
	providerTimestamp := chain.ProviderTimestamp.UTC()
	if providerTimestamp.IsZero() {
		for _, contract := range chain.Contracts {
			if contract.ProviderTimestamp.After(providerTimestamp) {
				providerTimestamp = contract.ProviderTimestamp.UTC()
			}
		}
	}
	observation := OptionChainObservation{
		Symbol: strings.ToUpper(strings.TrimSpace(chain.Symbol)), UnderlyingPrice: marketDecimal(chain.UnderlyingPrice),
		Contracts: make([]OptionContractObservation, 0, len(chain.Contracts)),
		Provenance: Provenance{
			Provider: strings.ToLower(strings.TrimSpace(provider)), Role: BrokerAuthority,
			Feed: "broker_entitled", Quality: entitlementQuality(chain.Delayed, true),
			ProviderTimestamp: providerTimestamp, ReceivedAt: receivedAt.UTC(),
		},
	}
	for _, contract := range chain.Contracts {
		observation.Contracts = append(observation.Contracts, OptionContractObservation{
			Symbol: contract.Symbol, Underlying: contract.Underlying, PutCall: strings.ToUpper(contract.PutCall),
			Expiration: contract.Expiration, Strike: Decimal(contract.Strike), Bid: marketDecimal(contract.Bid),
			Ask: marketDecimal(contract.Ask), Mark: marketDecimal(contract.Mark), Delta: marketDecimal(contract.Delta),
			ImpliedVolatility: marketDecimal(contract.ImpliedVolatility), OpenInterest: contract.OpenInterest,
			Volume: contract.Volume, ProviderTimestamp: contract.ProviderTimestamp.UTC(),
		})
	}
	if err := validateOptionChain(observation, receivedAt.UTC(), freshness); err != nil {
		return OptionChainObservation{}, err
	}
	return observation, nil
}

func entitlementQuality(value *bool, delayedFlag bool) FeedQuality {
	if value == nil {
		return Indicative
	}
	isRealtime := *value
	if delayedFlag {
		isRealtime = !*value
	}
	if isRealtime {
		return RealTimeConsolidated
	}
	return Delayed
}

func marketDecimal(value *financial.Decimal) *Decimal {
	if value == nil {
		return nil
	}
	converted := Decimal(*value)
	return &converted
}

func validateOptionChain(chain OptionChainObservation, now time.Time, freshness FreshnessPolicy) error {
	if !boundedText(chain.Symbol, 32) || len(chain.Contracts) == 0 || len(chain.Contracts) > 100 || !validDecimal(chain.UnderlyingPrice) {
		return fmt.Errorf("%w: option-chain identity", ErrInvalidObservation)
	}
	if err := ValidateProvenance(chain.Provenance, now, freshness); err != nil {
		return err
	}
	for _, contract := range chain.Contracts {
		if !boundedText(contract.Symbol, 64) || !strings.EqualFold(contract.Underlying, chain.Symbol) || (contract.PutCall != "PUT" && contract.PutCall != "CALL") || !boundedText(contract.Expiration, 10) {
			return fmt.Errorf("%w: option contract identity", ErrInvalidObservation)
		}
		strike := contract.Strike
		if !validDecimal(&strike) || !validDecimal(contract.Bid) || !validDecimal(contract.Ask) || !validDecimal(contract.Mark) || !validSignedDecimal(contract.Delta) || !validDecimal(contract.ImpliedVolatility) {
			return fmt.Errorf("%w: option contract value", ErrInvalidObservation)
		}
		if contract.OpenInterest != nil && *contract.OpenInterest < 0 || contract.Volume != nil && *contract.Volume < 0 {
			return fmt.Errorf("%w: option contract activity", ErrInvalidObservation)
		}
	}
	return nil
}
