package coinbase

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

const (
	defaultBaseURL = "https://api.coinbase.com"
	maxBodyBytes   = 1 << 20
	maxPages       = 10
)

var (
	keyNamePattern         = regexp.MustCompile(`^organizations/[A-Za-z0-9_-]+/apiKeys/[A-Za-z0-9_-]+$`)
	currencyPattern        = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._-]{0,31}$`)
	previewSymbolPattern   = regexp.MustCompile(`^[A-Z][A-Z0-9]{0,15}$`)
	previewDecimalPattern  = regexp.MustCompile(`^(0|[1-9][0-9]{0,17})(\.[0-9]{1,18})?$`)
	productStatusPattern   = regexp.MustCompile(`^[A-Z][A-Z0-9_-]{0,31}$`)
	providerDecimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)
)

type Config struct {
	BaseURL string
	Timeout time.Duration
}

type Client struct {
	base *url.URL
	http *http.Client
	now  func() time.Time
}

var _ financial.TradeHistoryProvider = (*Client)(nil)
var _ financial.OrderHistoryProvider = (*Client)(nil)
var _ financial.TradingCostProvider = (*Client)(nil)
var _ financial.OrderPreviewProvider = (*Client)(nil)

func New(cfg Config, client *http.Client) (*Client, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Host == "" || (base.Scheme != "https" && base.Scheme != "http") || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("coinbase base URL is invalid")
	}
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	return &Client{base: base, http: client, now: time.Now}, nil
}

type permissions struct {
	CanView       bool   `json:"can_view"`
	CanTrade      bool   `json:"can_trade"`
	CanTransfer   bool   `json:"can_transfer"`
	PortfolioUUID string `json:"portfolio_uuid"`
	PortfolioType string `json:"portfolio_type"`
}

func (c *Client) VerifyConnection(ctx context.Context, credentials *financial.Credentials) error {
	if err := normalizeCredentials(credentials); err != nil {
		return err
	}
	var value permissions
	if err := c.get(ctx, credentials, "/api/v3/brokerage/key_permissions", &value); err != nil {
		return err
	}
	portfolioID := strings.TrimSpace(value.PortfolioUUID)
	if !value.CanView || value.CanTransfer || portfolioID == "" || (credentials.PortfolioID != "" && credentials.PortfolioID != portfolioID) {
		return &financial.ProviderError{Code: financial.PermissionDenied}
	}
	credentials.PortfolioID = portfolioID
	credentials.ProviderCanTrade = value.CanTrade
	return nil
}

// Coinbase API-key credentials do not expire or refresh. Re-verification is the
// safe equivalent used by the provider-neutral connection boundary.
func (c *Client) RefreshAuthorization(ctx context.Context, credentials *financial.Credentials) error {
	return c.VerifyConnection(ctx, credentials)
}

type providerMoney struct {
	Value    json.Number `json:"value"`
	Currency string      `json:"currency"`
}

type providerAccount struct {
	UUID              string        `json:"uuid"`
	Name              string        `json:"name"`
	Currency          string        `json:"currency"`
	AvailableBalance  providerMoney `json:"available_balance"`
	Hold              providerMoney `json:"hold"`
	Active            bool          `json:"active"`
	Ready             bool          `json:"ready"`
	Type              string        `json:"type"`
	RetailPortfolioID string        `json:"retail_portfolio_id"`
}

type accountPage struct {
	Accounts []providerAccount `json:"accounts"`
	HasNext  bool              `json:"has_next"`
	Cursor   string            `json:"cursor"`
}

type providerSpotPosition struct {
	Asset                  string      `json:"asset"`
	AccountUUID            string      `json:"account_uuid"`
	TotalBalanceCrypto     json.Number `json:"total_balance_crypto"`
	AvailableToTradeCrypto json.Number `json:"available_to_trade_crypto"`
}

type portfolioBreakdownResponse struct {
	Breakdown struct {
		Portfolio struct {
			UUID string `json:"uuid"`
		} `json:"portfolio"`
		SpotPositions []providerSpotPosition `json:"spot_positions"`
	} `json:"breakdown"`
}

type providerFill struct {
	TradeTime          time.Time   `json:"trade_time"`
	TradeType          string      `json:"trade_type"`
	Price              json.Number `json:"price"`
	Size               json.Number `json:"size"`
	Commission         json.Number `json:"commission"`
	ProductID          string      `json:"product_id"`
	SequenceTimestamp  time.Time   `json:"sequence_timestamp"`
	LiquidityIndicator string      `json:"liquidity_indicator"`
	SizeInQuote        bool        `json:"size_in_quote"`
	Side               string      `json:"side"`
}

type fillPage struct {
	Fills              []providerFill `json:"fills"`
	Cursor             string         `json:"cursor"`
	ProofTokenRequired bool           `json:"proof_token_required"`
}

type providerOrder struct {
	ProductID            string      `json:"product_id"`
	Side                 string      `json:"side"`
	Status               string      `json:"status"`
	CreatedTime          time.Time   `json:"created_time"`
	CompletionPercentage json.Number `json:"completion_percentage"`
	AverageFilledPrice   json.Number `json:"average_filled_price"`
	NumberOfFills        json.Number `json:"number_of_fills"`
	PendingCancel        bool        `json:"pending_cancel"`
	TotalFees            json.Number `json:"total_fees"`
	TimeInForce          string      `json:"time_in_force"`
	FilledSize           json.Number `json:"filled_size"`
	FilledValue          json.Number `json:"filled_value"`
	OrderType            string      `json:"order_type"`
	RejectReason         string      `json:"reject_reason"`
	Settled              bool        `json:"settled"`
	ProductType          string      `json:"product_type"`
	IsLiquidation        bool        `json:"is_liquidation"`
	LastFillTime         string      `json:"last_fill_time"`
}

type orderPage struct {
	Orders             []providerOrder `json:"orders"`
	HasNext            bool            `json:"has_next"`
	Cursor             string          `json:"cursor"`
	ProofTokenRequired bool            `json:"proof_token_required"`
}

type providerFeeTier struct {
	PricingTier  string `json:"pricing_tier"`
	TakerFeeRate string `json:"taker_fee_rate"`
	MakerFeeRate string `json:"maker_fee_rate"`
}

type transactionSummary struct {
	TotalFees               json.Number     `json:"total_fees"`
	FeeTier                 providerFeeTier `json:"fee_tier"`
	AdvancedTradeOnlyVolume json.Number     `json:"advanced_trade_only_volume"`
	AdvancedTradeOnlyFees   json.Number     `json:"advanced_trade_only_fees"`
	HasCostPlusCommission   bool            `json:"has_cost_plus_commission"`
}

type providerDecimal string

func (value *providerDecimal) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || len(data) > 132 {
		return errors.New("invalid provider decimal")
	}
	text := string(data)
	if data[0] == '"' {
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
	}
	text = strings.TrimSpace(text)
	if text != "" && (!providerDecimalPattern.MatchString(text) || len(text) > 128) {
		return errors.New("invalid provider decimal")
	}
	*value = providerDecimal(text)
	return nil
}

type providerPreview struct {
	OrderTotal                  providerDecimal `json:"order_total"`
	CommissionTotal             providerDecimal `json:"commission_total"`
	Errors                      []string        `json:"errs"`
	Warning                     []string        `json:"warning"`
	QuoteSize                   providerDecimal `json:"quote_size"`
	BaseSize                    providerDecimal `json:"base_size"`
	BestBid                     providerDecimal `json:"best_bid"`
	BestAsk                     providerDecimal `json:"best_ask"`
	Slippage                    providerDecimal `json:"slippage"`
	PreviewID                   string          `json:"preview_id"`
	EstimatedAverageFilledPrice providerDecimal `json:"est_average_filled_price"`
}

type providerProduct struct {
	ProductID       string          `json:"product_id"`
	ProductType     string          `json:"product_type"`
	BaseCurrencyID  string          `json:"base_currency_id"`
	QuoteCurrencyID string          `json:"quote_currency_id"`
	BaseIncrement   providerDecimal `json:"base_increment"`
	QuoteIncrement  providerDecimal `json:"quote_increment"`
	BaseMinSize     providerDecimal `json:"base_min_size"`
	BaseMaxSize     providerDecimal `json:"base_max_size"`
	QuoteMinSize    providerDecimal `json:"quote_min_size"`
	QuoteMaxSize    providerDecimal `json:"quote_max_size"`
	Status          string          `json:"status"`
	IsDisabled      bool            `json:"is_disabled"`
	TradingDisabled bool            `json:"trading_disabled"`
	CancelOnly      bool            `json:"cancel_only"`
	LimitOnly       bool            `json:"limit_only"`
	PostOnly        bool            `json:"post_only"`
	AuctionMode     bool            `json:"auction_mode"`
}

type marketPreviewConfiguration struct {
	QuoteSize   string `json:"quote_size,omitempty"`
	BaseSize    string `json:"base_size,omitempty"`
	RFQDisabled bool   `json:"rfq_disabled"`
}

type providerPreviewRequest struct {
	ProductID          string `json:"product_id"`
	Side               string `json:"side"`
	OrderConfiguration struct {
		MarketIOC marketPreviewConfiguration `json:"market_market_ioc"`
	} `json:"order_configuration"`
}

func (c *Client) providerAccounts(ctx context.Context, credentials *financial.Credentials) ([]providerAccount, error) {
	if err := c.VerifyConnection(ctx, credentials); err != nil {
		return nil, err
	}
	accounts := make([]providerAccount, 0, 32)
	cursor := ""
	for page := 0; page < maxPages; page++ {
		query := url.Values{"limit": {"250"}}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		var response accountPage
		if err := c.get(ctx, credentials, "/api/v3/brokerage/accounts?"+query.Encode(), &response); err != nil {
			return nil, err
		}
		if len(response.Accounts) > 250 {
			return nil, &financial.ProviderError{Code: financial.InvalidProviderResponse}
		}
		accounts = append(accounts, response.Accounts...)
		if !response.HasNext {
			return accounts, nil
		}
		next := strings.TrimSpace(response.Cursor)
		if next == "" || next == cursor || len(next) > 1024 {
			return nil, &financial.ProviderError{Code: financial.InvalidProviderResponse}
		}
		cursor = next
	}
	return nil, &financial.ProviderError{Code: financial.InvalidProviderResponse}
}

func (c *Client) providerPortfolioPositions(ctx context.Context, credentials *financial.Credentials) ([]providerSpotPosition, error) {
	if err := c.VerifyConnection(ctx, credentials); err != nil {
		return nil, err
	}
	portfolioID := strings.TrimSpace(credentials.PortfolioID)
	if portfolioID == "" || len(portfolioID) > 200 {
		return nil, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	path := "/api/v3/brokerage/portfolios/" + url.PathEscape(portfolioID) + "?currency=USD"
	var response portfolioBreakdownResponse
	if err := c.get(ctx, credentials, path, &response); err != nil {
		return nil, err
	}
	if response.Breakdown.Portfolio.UUID != portfolioID || len(response.Breakdown.SpotPositions) > 250 {
		return nil, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	return response.Breakdown.SpotPositions, nil
}

func portfolioAccount(credentials *financial.Credentials) (financial.FinancialAccount, error) {
	portfolioID := strings.TrimSpace(credentials.PortfolioID)
	if portfolioID == "" || len(portfolioID) > 200 {
		return financial.FinancialAccount{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	masked := "••••"
	if len(portfolioID) >= 4 {
		masked += portfolioID[len(portfolioID)-4:]
	}
	tradeAuthorization := financial.Unsupported
	if credentials.ProviderCanTrade {
		tradeAuthorization = financial.Supported
	}
	return financial.FinancialAccount{
		Provider:          "coinbase",
		ProviderAccountID: "portfolio:" + portfolioID,
		DisplayName:       "Coinbase Portfolio",
		MaskedIdentifier:  masked,
		AccountType:       "digital_asset_portfolio",
		BaseCurrency:      "USD",
		Status:            "active",
		Capabilities: financial.Capabilities{
			"crypto_assets":                financial.Supported,
			"balances":                     financial.Supported,
			"positions":                    financial.Supported,
			"trade_history":                financial.Supported,
			"order_history":                financial.Supported,
			"trading_costs":                financial.Supported,
			"order_preview":                financial.Supported,
			"provider_trade_authorization": tradeAuthorization,
			"equities":                     financial.Unsupported,
			"options":                      financial.Unsupported,
			"margin":                       financial.Unsupported,
			"orders":                       financial.Unsupported,
			"transfers":                    financial.Unsupported,
		},
	}, nil
}

func (c *Client) ListAccounts(ctx context.Context, credentials *financial.Credentials) ([]financial.FinancialAccount, error) {
	if credentials.PortfolioID == "" {
		if err := c.VerifyConnection(ctx, credentials); err != nil {
			return nil, err
		}
	}
	if _, err := c.providerAccounts(ctx, credentials); err != nil {
		return nil, err
	}
	account, err := portfolioAccount(credentials)
	if err != nil {
		return nil, err
	}
	return []financial.FinancialAccount{account}, nil
}

func (c *Client) GetAccount(_ context.Context, credentials *financial.Credentials, id string) (financial.FinancialAccount, error) {
	account, err := portfolioAccount(credentials)
	if err != nil {
		return financial.FinancialAccount{}, err
	}
	if id != account.ProviderAccountID {
		return financial.FinancialAccount{}, &financial.ProviderError{Code: financial.AccountNotFound}
	}
	return account, nil
}

func validateAccountID(credentials *financial.Credentials, id string) error {
	account, err := portfolioAccount(credentials)
	if err != nil {
		return err
	}
	if id != account.ProviderAccountID {
		return &financial.ProviderError{Code: financial.AccountNotFound}
	}
	return nil
}

func (c *Client) GetBalances(ctx context.Context, credentials *financial.Credentials, id string) (financial.Balances, error) {
	if err := validateAccountID(credentials, id); err != nil {
		return financial.Balances{}, err
	}
	accounts, err := c.providerAccounts(ctx, credentials)
	if err != nil {
		return financial.Balances{}, err
	}
	for _, account := range accounts {
		if strings.EqualFold(account.Currency, "USD") {
			available, err := normalizedDecimal(account.AvailableBalance.Value)
			if err != nil {
				return financial.Balances{}, err
			}
			total, err := addDecimals(account.AvailableBalance.Value, account.Hold.Value)
			if err != nil {
				return financial.Balances{}, err
			}
			return financial.Balances{
				Cash:          &financial.Money{Amount: total, Currency: "USD"},
				AvailableCash: &financial.Money{Amount: available, Currency: "USD"},
			}, nil
		}
	}
	return financial.Balances{}, nil
}

func (c *Client) GetPositions(ctx context.Context, credentials *financial.Credentials, id string) ([]financial.Position, error) {
	if err := validateAccountID(credentials, id); err != nil {
		return nil, err
	}
	providerPositions, err := c.providerPortfolioPositions(ctx, credentials)
	if err != nil {
		return nil, err
	}
	positions := make([]financial.Position, 0, len(providerPositions))
	positionIndex := make(map[string]int, len(providerPositions))
	for _, providerPosition := range providerPositions {
		currency := strings.ToUpper(strings.TrimSpace(providerPosition.Asset))
		if providerPosition.AccountUUID == "" || len(providerPosition.AccountUUID) > 200 || !currencyPattern.MatchString(currency) {
			return nil, &financial.ProviderError{Code: financial.InvalidProviderResponse}
		}
		quantity, quantityOK := requiredNonnegativeProviderDecimal(providerPosition.TotalBalanceCrypto)
		availableQuantity, availableOK := requiredNonnegativeProviderDecimal(providerPosition.AvailableToTradeCrypto)
		if !quantityOK || !availableOK {
			return nil, &financial.ProviderError{Code: financial.InvalidProviderResponse}
		}
		unavailableQuantity, err := subtractDecimals(quantity, availableQuantity)
		if err != nil {
			return nil, err
		}
		nonzero, err := decimalNonzero(quantity)
		if err != nil {
			return nil, err
		}
		if !nonzero || currency == "USD" {
			continue
		}
		if index, exists := positionIndex[currency]; exists {
			combinedQuantity, combineErr := addDecimals(json.Number(positions[index].Quantity), json.Number(quantity))
			if combineErr != nil || positions[index].AvailableQuantity == nil {
				return nil, &financial.ProviderError{Code: financial.InvalidProviderResponse}
			}
			combinedAvailable, combineErr := addDecimals(json.Number(*positions[index].AvailableQuantity), json.Number(availableQuantity))
			if combineErr != nil {
				return nil, combineErr
			}
			combinedUnavailable, combineErr := subtractDecimals(combinedQuantity, combinedAvailable)
			if combineErr != nil {
				return nil, combineErr
			}
			positions[index].Quantity = combinedQuantity
			positions[index].AvailableQuantity = &combinedAvailable
			positions[index].UnavailableToTradeQuantity = &combinedUnavailable
			continue
		}
		positionIndex[currency] = len(positions)
		positions = append(positions, financial.Position{
			AccountID:                  id,
			InstrumentType:             "CRYPTO",
			Symbol:                     currency,
			Quantity:                   quantity,
			AvailableQuantity:          &availableQuantity,
			UnavailableToTradeQuantity: &unavailableQuantity,
			Direction:                  "long",
			ProviderInstrumentID:       providerPosition.AccountUUID,
		})
	}
	return positions, nil
}

func (c *Client) GetCapabilities(_ context.Context, credentials *financial.Credentials, id string) (financial.Capabilities, error) {
	if err := validateAccountID(credentials, id); err != nil {
		return nil, err
	}
	account, _ := portfolioAccount(credentials)
	return account.Capabilities, nil
}

func (c *Client) GetTradeFills(ctx context.Context, credentials *financial.Credentials, id string, limit int) (financial.TradeFillPage, error) {
	if err := validateAccountID(credentials, id); err != nil {
		return financial.TradeFillPage{}, err
	}
	if limit != 50 {
		return financial.TradeFillPage{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	if err := c.VerifyConnection(ctx, credentials); err != nil {
		return financial.TradeFillPage{}, err
	}
	query := url.Values{
		"limit":         {"50"},
		"product_types": {"SPOT"},
		"sort_by":       {"TRADE_TIME"},
	}
	var response fillPage
	if err := c.get(ctx, credentials, "/api/v3/brokerage/orders/historical/fills?"+query.Encode(), &response); err != nil {
		return financial.TradeFillPage{}, err
	}
	if response.ProofTokenRequired {
		return financial.TradeFillPage{}, &financial.ProviderError{Code: financial.PermissionDenied}
	}
	if len(response.Fills) > limit || len(response.Cursor) > 1024 || !safeCursor(response.Cursor) {
		return financial.TradeFillPage{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	now := c.now().UTC()
	fills := make([]financial.TradeFill, 0, len(response.Fills))
	for _, raw := range response.Fills {
		productID := strings.ToUpper(strings.TrimSpace(raw.ProductID))
		separator := strings.LastIndexByte(productID, '-')
		if separator < 1 || separator == len(productID)-1 {
			return financial.TradeFillPage{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
		}
		baseAsset, quoteCurrency := productID[:separator], productID[separator+1:]
		side := strings.ToUpper(strings.TrimSpace(raw.Side))
		liquidity, ok := normalizedLiquidity(raw.LiquidityIndicator)
		price, priceOK := positiveProviderDecimal(raw.Price)
		size, sizeOK := positiveProviderDecimal(raw.Size)
		commission, commissionOK := nonnegativeProviderDecimal(raw.Commission)
		if !currencyPattern.MatchString(baseAsset) || !currencyPattern.MatchString(quoteCurrency) || (side != "BUY" && side != "SELL") || strings.ToUpper(strings.TrimSpace(raw.TradeType)) != "FILL" || !ok || !priceOK || !sizeOK || !commissionOK || raw.TradeTime.IsZero() || raw.TradeTime.After(now.Add(2*time.Minute)) || raw.SequenceTimestamp.IsZero() || raw.SequenceTimestamp.After(now.Add(2*time.Minute)) {
			return financial.TradeFillPage{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
		}
		sizeUnit := baseAsset
		if raw.SizeInQuote {
			sizeUnit = quoteCurrency
		}
		fills = append(fills, financial.TradeFill{
			ProductID: productID, BaseAsset: baseAsset, QuoteCurrency: quoteCurrency,
			Side: side, Price: price, Size: size, SizeUnit: sizeUnit,
			Commission: financial.Money{Amount: commission, Currency: quoteCurrency},
			TradeTime:  raw.TradeTime.UTC(), Liquidity: liquidity,
		})
	}
	sort.SliceStable(fills, func(left, right int) bool {
		return fills[left].TradeTime.After(fills[right].TradeTime)
	})
	return financial.TradeFillPage{
		Provider: "coinbase", Feed: "advanced_trade_fills", Fills: fills,
		HasMore: strings.TrimSpace(response.Cursor) != "", RetrievedAt: now,
	}, nil
}

func (c *Client) GetOrderHistory(ctx context.Context, credentials *financial.Credentials, id string, limit int) (financial.OrderHistoryPage, error) {
	if err := validateAccountID(credentials, id); err != nil {
		return financial.OrderHistoryPage{}, err
	}
	if limit != 50 {
		return financial.OrderHistoryPage{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	if err := c.VerifyConnection(ctx, credentials); err != nil {
		return financial.OrderHistoryPage{}, err
	}
	query := url.Values{
		"limit":                                  {"50"},
		"product_type":                           {"SPOT"},
		"order_placement_source":                 {"RETAIL_ADVANCED"},
		"use_simplified_total_value_calculation": {"true"},
	}
	var response orderPage
	if err := c.get(ctx, credentials, "/api/v3/brokerage/orders/historical/batch?"+query.Encode(), &response); err != nil {
		return financial.OrderHistoryPage{}, err
	}
	if response.ProofTokenRequired {
		return financial.OrderHistoryPage{}, &financial.ProviderError{Code: financial.PermissionDenied}
	}
	if len(response.Orders) > limit || len(response.Cursor) > 1024 || !safeCursor(response.Cursor) || (response.HasNext && strings.TrimSpace(response.Cursor) == "") {
		return financial.OrderHistoryPage{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	now := c.now().UTC()
	orders := make([]financial.OrderObservation, 0, len(response.Orders))
	for _, raw := range response.Orders {
		order, ok := normalizedOrderObservation(raw, now)
		if !ok {
			return financial.OrderHistoryPage{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
		}
		orders = append(orders, order)
	}
	sort.SliceStable(orders, func(left, right int) bool {
		return orders[left].CreatedAt.After(orders[right].CreatedAt)
	})
	return financial.OrderHistoryPage{
		Provider: "coinbase", Feed: "advanced_trade_orders", Orders: orders,
		HasMore: response.HasNext, RetrievedAt: now,
	}, nil
}

func (c *Client) GetTradingCostSummary(ctx context.Context, credentials *financial.Credentials, id string) (financial.TradingCostSummary, error) {
	if err := validateAccountID(credentials, id); err != nil {
		return financial.TradingCostSummary{}, err
	}
	if err := c.VerifyConnection(ctx, credentials); err != nil {
		return financial.TradingCostSummary{}, err
	}
	query := url.Values{"product_type": {"SPOT"}}
	var response transactionSummary
	if err := c.get(ctx, credentials, "/api/v3/brokerage/transaction_summary?"+query.Encode(), &response); err != nil {
		return financial.TradingCostSummary{}, err
	}
	pricingTier, tierOK := safeProviderLabel(response.FeeTier.PricingTier)
	makerRate, makerOK := providerRate(response.FeeTier.MakerFeeRate)
	takerRate, takerOK := providerRate(response.FeeTier.TakerFeeRate)
	totalFees, totalOK := requiredNonnegativeProviderDecimal(response.TotalFees)
	advancedVolume, volumeOK := requiredNonnegativeProviderDecimal(response.AdvancedTradeOnlyVolume)
	advancedFees, feesOK := requiredNonnegativeProviderDecimal(response.AdvancedTradeOnlyFees)
	if !tierOK || !makerOK || !takerOK || !totalOK || !volumeOK || !feesOK {
		return financial.TradingCostSummary{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	return financial.TradingCostSummary{
		Provider: "coinbase", Feed: "advanced_trade_transaction_summary", ProductType: "SPOT",
		PricingTier: pricingTier, MakerFeeRate: makerRate, TakerFeeRate: takerRate,
		AdvancedTradeVolume: financial.Money{Amount: advancedVolume, Currency: "USD"},
		AdvancedTradeFees:   financial.Money{Amount: advancedFees, Currency: "USD"},
		TotalFees:           financial.Money{Amount: totalFees, Currency: "USD"},
		CostPlusCommission:  response.HasCostPlusCommission, RetrievedAt: c.now().UTC(),
	}, nil
}

func (c *Client) PreviewSpotOrder(ctx context.Context, credentials *financial.Credentials, id string, input financial.SpotOrderPreviewRequest) (financial.SpotOrderPreview, error) {
	if err := validateAccountID(credentials, id); err != nil {
		return financial.SpotOrderPreview{}, err
	}
	symbol := strings.ToUpper(strings.TrimSpace(input.Symbol))
	side := strings.ToUpper(strings.TrimSpace(input.Side))
	size := strings.TrimSpace(string(input.Size))
	quantity, quantityOK := new(big.Rat).SetString(size)
	if !previewSymbolPattern.MatchString(symbol) || symbol == "USD" || (side != "BUY" && side != "SELL") || !previewDecimalPattern.MatchString(size) || !quantityOK || quantity.Sign() <= 0 {
		return financial.SpotOrderPreview{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	if err := c.VerifyConnection(ctx, credentials); err != nil {
		return financial.SpotOrderPreview{}, err
	}

	productID := symbol + "-USD"
	var product providerProduct
	if err := c.get(ctx, credentials, "/api/v3/brokerage/products/"+url.PathEscape(productID), &product); err != nil {
		return financial.SpotOrderPreview{}, err
	}
	observedAt := c.now().UTC()
	rules, ok := normalizeSpotProductRules(product, productID, symbol, side, financial.Decimal(size), observedAt)
	if !ok {
		return financial.SpotOrderPreview{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	request := providerPreviewRequest{ProductID: productID, Side: side}
	request.OrderConfiguration.MarketIOC.RFQDisabled = true
	requestedCurrency := symbol
	if side == "BUY" {
		request.OrderConfiguration.MarketIOC.QuoteSize = size
		requestedCurrency = "USD"
	} else {
		request.OrderConfiguration.MarketIOC.BaseSize = size
	}
	var response providerPreview
	if err := c.post(ctx, credentials, "/api/v3/brokerage/orders/preview", request, &response); err != nil {
		return financial.SpotOrderPreview{}, err
	}
	preview, ok := normalizeSpotPreview(response, productID, symbol, side, financial.Money{Amount: financial.Decimal(size), Currency: requestedCurrency}, credentials.ProviderCanTrade, observedAt)
	if !ok {
		return financial.SpotOrderPreview{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	preview.ProductRules = &rules
	for _, reason := range rules.BlockReasons {
		preview.BlockReasons = appendNormalizedReason(preview.BlockReasons, reason)
	}
	if !rules.MarketIOCEnabled {
		preview.PreviewState = "BLOCKED"
	}
	return preview, nil
}

func normalizeSpotProductRules(raw providerProduct, productID, symbol, side string, size financial.Decimal, observedAt time.Time) (financial.SpotProductRules, bool) {
	status := strings.ToUpper(strings.TrimSpace(raw.Status))
	baseIncrement, baseIncrementOK := positiveProductDecimal(raw.BaseIncrement)
	quoteIncrement, quoteIncrementOK := positiveProductDecimal(raw.QuoteIncrement)
	baseMin, baseMinOK := positiveProductDecimal(raw.BaseMinSize)
	baseMax, baseMaxOK := positiveProductDecimal(raw.BaseMaxSize)
	quoteMin, quoteMinOK := positiveProductDecimal(raw.QuoteMinSize)
	quoteMax, quoteMaxOK := positiveProductDecimal(raw.QuoteMaxSize)
	if raw.ProductID != productID || strings.ToUpper(strings.TrimSpace(raw.ProductType)) != "SPOT" || strings.ToUpper(strings.TrimSpace(raw.BaseCurrencyID)) != symbol || strings.ToUpper(strings.TrimSpace(raw.QuoteCurrencyID)) != "USD" || !productStatusPattern.MatchString(status) || !baseIncrementOK || !quoteIncrementOK || !baseMinOK || !baseMaxOK || !quoteMinOK || !quoteMaxOK || compareDecimal(baseMin, baseMax) > 0 || compareDecimal(quoteMin, quoteMax) > 0 || !decimalMultiple(baseMin, baseIncrement) || !decimalMultiple(baseMax, baseIncrement) || !decimalMultiple(quoteMin, quoteIncrement) || !decimalMultiple(quoteMax, quoteIncrement) || observedAt.IsZero() {
		return financial.SpotProductRules{}, false
	}
	blocks := make([]string, 0, 8)
	if status != "ONLINE" || raw.IsDisabled || raw.TradingDisabled {
		blocks = append(blocks, "PRODUCT_DISABLED")
	}
	if raw.CancelOnly {
		blocks = append(blocks, "PRODUCT_CANCEL_ONLY")
	}
	if raw.LimitOnly {
		blocks = append(blocks, "PRODUCT_LIMIT_ONLY")
	}
	if raw.PostOnly {
		blocks = append(blocks, "PRODUCT_POST_ONLY")
	}
	if raw.AuctionMode {
		blocks = append(blocks, "PRODUCT_AUCTION_MODE")
	}
	minimum, maximum, increment := baseMin, baseMax, baseIncrement
	if side == "BUY" {
		minimum, maximum, increment = quoteMin, quoteMax, quoteIncrement
	}
	if compareDecimal(size, minimum) < 0 {
		blocks = append(blocks, "SIZE_BELOW_MINIMUM")
	}
	if compareDecimal(size, maximum) > 0 {
		blocks = append(blocks, "SIZE_ABOVE_MAXIMUM")
	}
	if !decimalMultiple(size, increment) {
		blocks = append(blocks, "SIZE_INCREMENT_MISMATCH")
	}
	return financial.SpotProductRules{
		Provider: "coinbase", Feed: "advanced_trade_product", ProductID: productID, ProductType: "SPOT", BaseAsset: symbol, QuoteCurrency: "USD",
		BaseIncrement: baseIncrement, QuoteIncrement: quoteIncrement, BaseMinSize: baseMin, BaseMaxSize: baseMax, QuoteMinSize: quoteMin, QuoteMaxSize: quoteMax,
		Status: status, MarketIOCEnabled: len(blocks) == 0, BlockReasons: blocks, ObservedAt: observedAt,
	}, true
}

func positiveProductDecimal(value providerDecimal) (financial.Decimal, bool) {
	text := strings.TrimSpace(string(value))
	if !previewDecimalPattern.MatchString(text) {
		return "", false
	}
	number, ok := new(big.Rat).SetString(text)
	return financial.Decimal(text), ok && number.Sign() > 0
}

func compareDecimal(left, right financial.Decimal) int {
	leftNumber, leftOK := new(big.Rat).SetString(string(left))
	rightNumber, rightOK := new(big.Rat).SetString(string(right))
	if !leftOK || !rightOK {
		return 0
	}
	return leftNumber.Cmp(rightNumber)
}

func decimalMultiple(value, increment financial.Decimal) bool {
	valueNumber, valueOK := new(big.Rat).SetString(string(value))
	incrementNumber, incrementOK := new(big.Rat).SetString(string(increment))
	return valueOK && incrementOK && incrementNumber.Sign() > 0 && new(big.Rat).Quo(valueNumber, incrementNumber).IsInt()
}

func appendNormalizedReason(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func normalizeSpotPreview(raw providerPreview, productID, symbol, side string, requested financial.Money, tradingAuthorized bool, now time.Time) (financial.SpotOrderPreview, bool) {
	if len(raw.Errors) > 20 || len(raw.Warning) > 20 || len(raw.PreviewID) > 256 {
		return financial.SpotOrderPreview{}, false
	}
	orderTotal, orderTotalOK := normalizedPreviewDecimal(raw.OrderTotal, true)
	commission, commissionOK := normalizedPreviewDecimal(raw.CommissionTotal, true)
	baseSize, baseSizeOK := normalizedPreviewDecimal(raw.BaseSize, true)
	quoteSize, quoteSizeOK := normalizedPreviewDecimal(raw.QuoteSize, true)
	if !orderTotalOK || !commissionOK || !baseSizeOK || !quoteSizeOK {
		return financial.SpotOrderPreview{}, false
	}
	blocks, blocksOK := normalizedPreviewMessages(raw.Errors, previewBlockReason)
	warnings, warningsOK := normalizedPreviewMessages(raw.Warning, previewWarning)
	if !blocksOK || !warningsOK {
		return financial.SpotOrderPreview{}, false
	}
	state := "READY"
	if len(blocks) > 0 {
		state = "BLOCKED"
	}
	if state == "READY" && (raw.PreviewID == "" || !positiveDecimal(orderTotal) || (!positiveDecimal(baseSize) && !positiveDecimal(quoteSize))) {
		return financial.SpotOrderPreview{}, false
	}
	bestBid, bidOK := optionalPreviewMoney(raw.BestBid, "USD")
	bestAsk, askOK := optionalPreviewMoney(raw.BestAsk, "USD")
	estimatedPrice, estimatedOK := optionalPreviewMoney(raw.EstimatedAverageFilledPrice, "USD")
	slippage, slippageOK := optionalPreviewDecimal(raw.Slippage)
	if !bidOK || !askOK || !estimatedOK || !slippageOK {
		return financial.SpotOrderPreview{}, false
	}
	return financial.SpotOrderPreview{
		Provider: "coinbase", Feed: "advanced_trade_order_preview", ProductID: productID,
		BaseAsset: symbol, QuoteCurrency: "USD", Side: side, OrderType: "MARKET_IOC",
		RequestedSize: requested, BaseSize: baseSize, QuoteSize: quoteSize,
		OrderTotal:      financial.Money{Amount: orderTotal, Currency: "USD"},
		CommissionTotal: financial.Money{Amount: commission, Currency: "USD"},
		BestBid:         bestBid, BestAsk: bestAsk, EstimatedAverageFilledPrice: estimatedPrice,
		Slippage: slippage, PreviewState: state, BlockReasons: blocks, Warnings: warnings,
		ProviderTradingAuthorized: tradingAuthorized, PreviewedAt: now,
	}, true
}

func normalizedPreviewDecimal(value providerDecimal, required bool) (financial.Decimal, bool) {
	text := strings.TrimSpace(string(value))
	if text == "" {
		return "", !required
	}
	normalized, ok := nonnegativeProviderDecimal(json.Number(text))
	return normalized, ok
}

func positiveDecimal(value financial.Decimal) bool {
	rational, ok := new(big.Rat).SetString(string(value))
	return ok && rational.Sign() > 0
}

func optionalPreviewMoney(value providerDecimal, currency string) (*financial.Money, bool) {
	normalized, ok := normalizedPreviewDecimal(value, false)
	if !ok {
		return nil, false
	}
	if normalized == "" {
		return nil, true
	}
	if !positiveDecimal(normalized) {
		return nil, false
	}
	return &financial.Money{Amount: normalized, Currency: currency}, true
}

func optionalPreviewDecimal(value providerDecimal) (*financial.Decimal, bool) {
	normalized, ok := normalizedPreviewDecimal(value, false)
	if !ok {
		return nil, false
	}
	if normalized == "" {
		return nil, true
	}
	return &normalized, true
}

func normalizedPreviewMessages(values []string, normalize func(string) string) ([]string, bool) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value == "" || len(value) > 128 {
			return nil, false
		}
		for _, character := range value {
			if (character < 'A' || character > 'Z') && character != '_' && (character < '0' || character > '9') {
				return nil, false
			}
		}
		normalized := normalize(value)
		if _, exists := seen[normalized]; !exists {
			seen[normalized] = struct{}{}
			result = append(result, normalized)
		}
	}
	return result, true
}

func previewBlockReason(value string) string {
	switch {
	case strings.Contains(value, "INSUFFICIENT"):
		return "INSUFFICIENT_FUNDS"
	case strings.Contains(value, "SIZE"), strings.Contains(value, "PRECISION"), strings.Contains(value, "NOTIONAL"):
		return "INVALID_SIZE"
	case strings.Contains(value, "LIQUIDITY"), strings.Contains(value, "PRICE_BOOK"), strings.Contains(value, "MARKET"), strings.Contains(value, "HALTED"):
		return "MARKET_UNAVAILABLE"
	case strings.Contains(value, "GEOFENCING"), strings.Contains(value, "COMPLIANCE"), strings.Contains(value, "NOT_ALLOWED"), strings.Contains(value, "TRADING_DISABLED"), strings.Contains(value, "UNTRADABLE"):
		return "ACCOUNT_RESTRICTED"
	default:
		return "PROVIDER_REJECTED"
	}
}

func previewWarning(value string) string {
	switch value {
	case "BIG_ORDER":
		return "LARGE_ORDER"
	case "SMALL_ORDER":
		return "SMALL_ORDER"
	default:
		return "PROVIDER_WARNING"
	}
}

func normalizedOrderObservation(raw providerOrder, now time.Time) (financial.OrderObservation, bool) {
	productID := strings.ToUpper(strings.TrimSpace(raw.ProductID))
	separator := strings.LastIndexByte(productID, '-')
	if separator < 1 || separator == len(productID)-1 {
		return financial.OrderObservation{}, false
	}
	baseAsset, quoteCurrency := productID[:separator], productID[separator+1:]
	side := strings.ToUpper(strings.TrimSpace(raw.Side))
	status, statusOK := normalizedOrderStatus(raw.Status)
	orderType, orderTypeOK := normalizedOrderType(raw.OrderType)
	timeInForce, timeInForceOK := normalizedTimeInForce(raw.TimeInForce)
	completion, completionOK := percentageProviderDecimal(raw.CompletionPercentage)
	filledSize, filledSizeOK := nonnegativeProviderDecimal(raw.FilledSize)
	filledValue, filledValueOK := nonnegativeProviderDecimal(raw.FilledValue)
	totalFees, totalFeesOK := nonnegativeProviderDecimal(raw.TotalFees)
	numberOfFills, fillsOK := providerCount(raw.NumberOfFills)
	outcomeReason, reasonOK := normalizedOutcomeReason(raw.RejectReason)
	lastFillAt, lastFillOK := optionalProviderTime(raw.LastFillTime, now)
	if !currencyPattern.MatchString(baseAsset) || !currencyPattern.MatchString(quoteCurrency) || (side != "BUY" && side != "SELL") || strings.ToUpper(strings.TrimSpace(raw.ProductType)) != "SPOT" || !statusOK || !orderTypeOK || !timeInForceOK || !completionOK || !filledSizeOK || !filledValueOK || !totalFeesOK || !fillsOK || !reasonOK || !lastFillOK || raw.CreatedTime.IsZero() || raw.CreatedTime.After(now.Add(2*time.Minute)) || (lastFillAt != nil && lastFillAt.Before(raw.CreatedTime.Add(-2*time.Minute))) {
		return financial.OrderObservation{}, false
	}
	var averageFilledPrice *financial.Money
	if strings.TrimSpace(raw.AverageFilledPrice.String()) != "" {
		average, averageOK := nonnegativeProviderDecimal(raw.AverageFilledPrice)
		if !averageOK {
			return financial.OrderObservation{}, false
		}
		if rational, _ := new(big.Rat).SetString(string(average)); rational.Sign() > 0 {
			averageFilledPrice = &financial.Money{Amount: average, Currency: quoteCurrency}
		}
	}
	return financial.OrderObservation{
		ProductID: productID, BaseAsset: baseAsset, QuoteCurrency: quoteCurrency,
		Side: side, Status: status, OrderType: orderType, TimeInForce: timeInForce,
		CompletionPercentage: completion, FilledSize: filledSize, FilledSizeUnit: baseAsset,
		FilledValue: financial.Money{Amount: filledValue, Currency: quoteCurrency}, AverageFilledPrice: averageFilledPrice,
		TotalFees: financial.Money{Amount: totalFees, Currency: quoteCurrency}, NumberOfFills: numberOfFills,
		PendingCancel: raw.PendingCancel, Settled: raw.Settled, IsLiquidation: raw.IsLiquidation,
		OutcomeReason: outcomeReason, CreatedAt: raw.CreatedTime.UTC(), LastFillAt: lastFillAt,
	}, true
}

func safeCursor(value string) bool {
	for _, character := range value {
		if character < 0x21 || character > 0x7e {
			return strings.TrimSpace(value) == ""
		}
	}
	return true
}

func normalizedLiquidity(value string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "MAKER":
		return "MAKER", true
	case "TAKER":
		return "TAKER", true
	case "", "UNKNOWN_LIQUIDITY_INDICATOR":
		return "UNKNOWN", true
	default:
		return "", false
	}
}

func normalizedOrderStatus(value string) (string, bool) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch normalized {
	case "PENDING", "OPEN", "FILLED", "CANCELLED", "EXPIRED", "FAILED", "QUEUED", "CANCEL_QUEUED", "EDIT_QUEUED":
		return normalized, true
	case "UNKNOWN_ORDER_STATUS":
		return "UNKNOWN", true
	default:
		return "", false
	}
}

func normalizedOrderType(value string) (string, bool) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch normalized {
	case "MARKET", "LIMIT", "STOP", "STOP_LIMIT", "BRACKET", "TWAP", "SCALED", "LIQUIDATION", "ROLL_OPEN", "ROLL_CLOSE":
		return normalized, true
	case "UNKNOWN_ORDER_TYPE":
		return "UNKNOWN", true
	default:
		return "", false
	}
}

func normalizedTimeInForce(value string) (string, bool) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch normalized {
	case "GOOD_UNTIL_DATE_TIME", "GOOD_UNTIL_CANCELLED", "IMMEDIATE_OR_CANCEL", "FILL_OR_KILL":
		return normalized, true
	case "", "UNKNOWN_TIME_IN_FORCE":
		return "UNKNOWN", true
	default:
		return "", false
	}
}

func percentageProviderDecimal(value json.Number) (financial.Decimal, bool) {
	result, ok := nonnegativeProviderDecimal(value)
	if !ok {
		return "", false
	}
	rational, valid := new(big.Rat).SetString(string(result))
	return result, valid && rational.Cmp(big.NewRat(100, 1)) <= 0
}

func providerCount(value json.Number) (int, bool) {
	text := strings.TrimSpace(value.String())
	if text == "" {
		return 0, true
	}
	if len(text) > 7 {
		return 0, false
	}
	for _, character := range text {
		if character < '0' || character > '9' {
			return 0, false
		}
	}
	integer, ok := new(big.Int).SetString(text, 10)
	if !ok || integer.Sign() < 0 || integer.Cmp(big.NewInt(1_000_000)) > 0 {
		return 0, false
	}
	return int(integer.Int64()), true
}

func normalizedOutcomeReason(value string) (string, bool) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "" || normalized == "REJECT_REASON_UNSPECIFIED" {
		return "NONE", true
	}
	if len(normalized) > 64 {
		return "", false
	}
	for _, character := range normalized {
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' {
			return "", false
		}
	}
	return normalized, true
}

func optionalProviderTime(value string, now time.Time) (*time.Time, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return nil, true
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil || parsed.After(now.Add(2*time.Minute)) {
		return nil, false
	}
	utc := parsed.UTC()
	return &utc, true
}

func positiveProviderDecimal(value json.Number) (financial.Decimal, bool) {
	result, err := normalizedDecimal(value)
	if err != nil {
		return "", false
	}
	rational, ok := new(big.Rat).SetString(string(result))
	return result, ok && rational.Sign() > 0
}

func nonnegativeProviderDecimal(value json.Number) (financial.Decimal, bool) {
	result, err := normalizedDecimal(value)
	if err != nil {
		return "", false
	}
	rational, ok := new(big.Rat).SetString(string(result))
	return result, ok && rational.Sign() >= 0
}

func requiredNonnegativeProviderDecimal(value json.Number) (financial.Decimal, bool) {
	if strings.TrimSpace(value.String()) == "" {
		return "", false
	}
	return nonnegativeProviderDecimal(value)
}

func providerRate(value string) (financial.Decimal, bool) {
	if strings.TrimSpace(value) == "" {
		return "", false
	}
	rate, ok := nonnegativeProviderDecimal(json.Number(value))
	if !ok {
		return "", false
	}
	rational, valid := new(big.Rat).SetString(string(rate))
	return rate, valid && rational.Cmp(big.NewRat(1, 1)) <= 0
}

func safeProviderLabel(value string) (string, bool) {
	normalized := strings.TrimSpace(value)
	if normalized == "" || len(normalized) > 64 {
		return "", false
	}
	for _, character := range normalized {
		if character < 0x20 || character > 0x7e {
			return "", false
		}
	}
	return normalized, true
}

// Arbion-side encrypted credential deletion is authoritative. Coinbase API-key
// deletion remains an explicit user action in the Coinbase portal.
func (c *Client) Disconnect(context.Context, *financial.Credentials) error { return nil }

func (c *Client) get(ctx context.Context, credentials *financial.Credentials, path string, output any) error {
	return c.request(ctx, credentials, http.MethodGet, path, nil, output)
}

func (c *Client) post(ctx context.Context, credentials *financial.Credentials, path string, input, output any) error {
	return c.request(ctx, credentials, http.MethodPost, path, input, output)
}

func (c *Client) request(ctx context.Context, credentials *financial.Credentials, method, path string, input, output any) error {
	requestURL, err := c.base.Parse(path)
	if err != nil || requestURL.Host != c.base.Host || requestURL.Scheme != c.base.Scheme {
		return &financial.ProviderError{Code: financial.InternalError}
	}
	var body io.Reader
	if input != nil {
		encoded, encodeErr := json.Marshal(input)
		if encodeErr != nil || len(encoded) > 16*1024 {
			return &financial.ProviderError{Code: financial.InternalError}
		}
		body = bytes.NewReader(encoded)
	}
	token, err := c.jwt(credentials, method, requestURL.Path)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return &financial.ProviderError{Code: financial.InternalError}
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		var networkError net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
			return &financial.ProviderError{Code: financial.Timeout}
		}
		return &financial.ProviderError{Code: financial.ProviderUnavailable}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		switch response.StatusCode {
		case http.StatusUnauthorized:
			return &financial.ProviderError{Code: financial.AuthorizationFailed}
		case http.StatusForbidden:
			return &financial.ProviderError{Code: financial.PermissionDenied}
		case http.StatusNotFound:
			return &financial.ProviderError{Code: financial.AccountNotFound}
		case http.StatusTooManyRequests:
			return &financial.ProviderError{Code: financial.RateLimited}
		default:
			if response.StatusCode >= 500 {
				return &financial.ProviderError{Code: financial.ProviderUnavailable}
			}
			return &financial.ProviderError{Code: financial.InvalidProviderResponse}
		}
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxBodyBytes+1))
	decoder.UseNumber()
	if err := decoder.Decode(output); err != nil {
		return &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	return nil
}

func (c *Client) jwt(credentials *financial.Credentials, method, path string) (string, error) {
	if err := normalizeCredentials(credentials); err != nil {
		return "", err
	}
	keyName := credentials.APIKeyName
	key, err := parsePrivateKey(credentials.APIPrivateKey)
	if err != nil {
		return "", &financial.ProviderError{Code: financial.InvalidCredentialFormat}
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", &financial.ProviderError{Code: financial.InternalError}
	}
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": keyName, "nonce": hex.EncodeToString(nonce), "typ": "JWT"})
	now := c.now().Unix()
	claims, _ := json.Marshal(map[string]any{
		"sub": keyName,
		"iss": "cdp",
		"nbf": now,
		"exp": now + 120,
		"uri": strings.ToUpper(method) + " " + c.base.Host + path,
	})
	encodedHeader := base64.RawURLEncoding.EncodeToString(header)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claims)
	unsigned := encodedHeader + "." + encodedClaims
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", &financial.ProviderError{Code: financial.InternalError}
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// Coinbase presents the same ECDSA key in several copy formats: literal PEM,
// a quoted JSON value, or one line with escaped newlines. Normalize those
// documented representations before parsing and before encrypted storage.
func normalizeCredentials(credentials *financial.Credentials) error {
	if credentials == nil {
		return &financial.ProviderError{Code: financial.InvalidCredentialFormat}
	}
	keyName := strings.TrimSpace(credentials.APIKeyName)
	if unquoted, ok := unquoteCredentialValue(keyName); ok {
		keyName = strings.TrimSpace(unquoted)
	}
	privateKey := strings.TrimSpace(credentials.APIPrivateKey)
	if unquoted, ok := unquoteCredentialValue(privateKey); ok {
		privateKey = strings.TrimSpace(unquoted)
	}
	privateKey = strings.ReplaceAll(privateKey, "\\r\\n", "\n")
	privateKey = strings.ReplaceAll(privateKey, "\\n", "\n")
	privateKey = strings.ReplaceAll(privateKey, "\\r", "\n")
	privateKey = strings.ReplaceAll(privateKey, "\r\n", "\n")
	privateKey = strings.TrimSpace(privateKey)
	if !keyNamePattern.MatchString(keyName) || privateKey == "" || len(privateKey) > 4096 {
		return &financial.ProviderError{Code: financial.InvalidCredentialFormat}
	}
	if _, err := parsePrivateKey(privateKey); err != nil {
		return &financial.ProviderError{Code: financial.InvalidCredentialFormat}
	}
	credentials.APIKeyName = keyName
	credentials.APIPrivateKey = privateKey
	return nil
}

func unquoteCredentialValue(value string) (string, bool) {
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return "", false
	}
	unquoted, err := strconv.Unquote(value)
	return unquoted, err == nil
}

func parsePrivateKey(value string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(value)))
	if block == nil {
		return nil, errors.New("private key is not PEM")
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		if key.Curve != elliptic.P256() {
			return nil, errors.New("private key must use P-256")
		}
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, errors.New("private key must use P-256")
	}
	return key, nil
}

func normalizedDecimal(value json.Number) (financial.Decimal, error) {
	text := strings.TrimSpace(value.String())
	if text == "" {
		text = "0"
	}
	if _, ok := new(big.Rat).SetString(text); !ok || len(text) > 128 {
		return "", &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	return financial.Decimal(text), nil
}

func addDecimals(left, right json.Number) (financial.Decimal, error) {
	l, err := normalizedDecimal(left)
	if err != nil {
		return "", err
	}
	r, err := normalizedDecimal(right)
	if err != nil {
		return "", err
	}
	leftScale := decimalScale(string(l))
	rightScale := decimalScale(string(r))
	scale := leftScale
	if rightScale > scale {
		scale = rightScale
	}
	if scale > 32 {
		return "", &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	leftRat, _ := new(big.Rat).SetString(string(l))
	rightRat, _ := new(big.Rat).SetString(string(r))
	return financial.Decimal(new(big.Rat).Add(leftRat, rightRat).FloatString(scale)), nil
}

func subtractDecimals(total, available financial.Decimal) (financial.Decimal, error) {
	totalScale := decimalScale(string(total))
	availableScale := decimalScale(string(available))
	scale := totalScale
	if availableScale > scale {
		scale = availableScale
	}
	if scale > 32 {
		return "", &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	totalRat, totalOK := new(big.Rat).SetString(string(total))
	availableRat, availableOK := new(big.Rat).SetString(string(available))
	if !totalOK || !availableOK || totalRat.Sign() < 0 || availableRat.Sign() < 0 || availableRat.Cmp(totalRat) > 0 {
		return "", &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	return financial.Decimal(new(big.Rat).Sub(totalRat, availableRat).FloatString(scale)), nil
}

func decimalScale(value string) int {
	if index := strings.IndexByte(value, '.'); index >= 0 {
		return len(value) - index - 1
	}
	return 0
}

func decimalNonzero(value financial.Decimal) (bool, error) {
	rational, ok := new(big.Rat).SetString(string(value))
	if !ok {
		return false, fmt.Errorf("invalid normalized decimal")
	}
	return rational.Sign() != 0, nil
}
