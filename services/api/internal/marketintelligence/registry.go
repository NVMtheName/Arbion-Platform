package marketintelligence

// DefaultSources returns the production-approved source catalog. Sources are
// disabled and unhealthy until a real adapter, valid credentials where needed,
// and an initial health check are wired. Research-only tools such as yfinance
// and OpenInsider are deliberately absent.
func DefaultSources() []Source {
	sources := []Source{
		{ID: "schwab_broker_market_data", Label: "Schwab Market Data", Role: BrokerAuthority, Feed: "broker_entitled", Quality: Indicative, Capabilities: []Capability{EquityQuote, OptionData}},
		{ID: "alpaca_iex", Label: "Alpaca IEX", Role: MarketObservation, Feed: "iex", Quality: RealTimeSingleVenue, Capabilities: []Capability{EquityQuote, EquityBars}},
		{ID: "alpaca_sip", Label: "Alpaca SIP", Role: MarketObservation, Feed: "sip", Quality: RealTimeConsolidated, Capabilities: []Capability{EquityQuote, EquityBars}},
		{ID: "alpaca_options_indicative", Label: "Alpaca Options Indicative", Role: MarketObservation, Feed: "indicative", Quality: Indicative, Capabilities: []Capability{OptionData}},
		{ID: "alpaca_opra", Label: "Alpaca OPRA", Role: MarketObservation, Feed: "opra", Quality: RealTimeConsolidated, Capabilities: []Capability{OptionData}},
		{ID: "coingecko_rest", Label: "CoinGecko", Role: ReferenceData, Feed: "rest", Quality: AggregatedReference, Capabilities: []Capability{CryptoMarkets}},
		{ID: "coinbase_exchange", Label: "Coinbase Public Markets", Role: MarketObservation, Feed: "rest_ticker_candles_book_trades_and_stats", Quality: RealTimeSingleVenue, Capabilities: []Capability{CryptoMarkets, CryptoCandles, CryptoLiquidity, CryptoTrades, CryptoStats}},
		{ID: "sec_edgar", Label: "SEC EDGAR", Role: PrimaryFiling, Feed: "submissions", Quality: Filing, Capabilities: []Capability{InsiderFiling}},
	}
	for sourceIndex := range sources {
		for _, capability := range sources[sourceIndex].Capabilities {
			sources[sourceIndex].CapabilityStatus = append(sources[sourceIndex].CapabilityStatus, CapabilityStatus{
				Capability: capability,
				State:      NotConfigured,
			})
		}
	}
	return sources
}
