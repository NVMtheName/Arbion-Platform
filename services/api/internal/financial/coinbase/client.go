package coinbase

import (
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
	keyNamePattern  = regexp.MustCompile(`^organizations/[A-Za-z0-9_-]+/apiKeys/[A-Za-z0-9_-]+$`)
	currencyPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._-]{0,31}$`)
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
	var value permissions
	if err := c.get(ctx, credentials, "/api/v3/brokerage/key_permissions", &value); err != nil {
		return err
	}
	portfolioID := strings.TrimSpace(value.PortfolioUUID)
	if !value.CanView || value.CanTrade || value.CanTransfer || portfolioID == "" || (credentials.PortfolioID != "" && credentials.PortfolioID != portfolioID) {
		return &financial.ProviderError{Code: financial.PermissionDenied}
	}
	credentials.PortfolioID = portfolioID
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

func portfolioAccount(credentials *financial.Credentials) (financial.FinancialAccount, error) {
	portfolioID := strings.TrimSpace(credentials.PortfolioID)
	if portfolioID == "" || len(portfolioID) > 200 {
		return financial.FinancialAccount{}, &financial.ProviderError{Code: financial.InvalidProviderResponse}
	}
	masked := "••••"
	if len(portfolioID) >= 4 {
		masked += portfolioID[len(portfolioID)-4:]
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
			"crypto_assets": financial.Supported,
			"balances":      financial.Supported,
			"positions":     financial.Supported,
			"trade_history": financial.Supported,
			"order_history": financial.Supported,
			"equities":      financial.Unsupported,
			"options":       financial.Unsupported,
			"margin":        financial.Unsupported,
			"orders":        financial.Unsupported,
			"transfers":     financial.Unsupported,
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
	accounts, err := c.providerAccounts(ctx, credentials)
	if err != nil {
		return nil, err
	}
	positions := make([]financial.Position, 0, len(accounts))
	for _, account := range accounts {
		currency := strings.ToUpper(strings.TrimSpace(account.Currency))
		if account.UUID == "" || !currencyPattern.MatchString(currency) {
			return nil, &financial.ProviderError{Code: financial.InvalidProviderResponse}
		}
		quantity, err := addDecimals(account.AvailableBalance.Value, account.Hold.Value)
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
		instrumentType := "CRYPTO"
		if strings.EqualFold(account.Type, "FIAT") {
			instrumentType = "CASH"
		}
		positions = append(positions, financial.Position{
			AccountID:            id,
			InstrumentType:       instrumentType,
			Symbol:               currency,
			Quantity:             quantity,
			Direction:            "long",
			ProviderInstrumentID: account.UUID,
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

// Arbion-side encrypted credential deletion is authoritative. Coinbase API-key
// deletion remains an explicit user action in the Coinbase portal.
func (c *Client) Disconnect(context.Context, *financial.Credentials) error { return nil }

func (c *Client) get(ctx context.Context, credentials *financial.Credentials, path string, output any) error {
	requestURL, err := c.base.Parse(path)
	if err != nil || requestURL.Host != c.base.Host || requestURL.Scheme != c.base.Scheme {
		return &financial.ProviderError{Code: financial.InternalError}
	}
	token, err := c.jwt(credentials, http.MethodGet, requestURL.Path)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return &financial.ProviderError{Code: financial.InternalError}
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
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
	keyName := strings.TrimSpace(credentials.APIKeyName)
	if !keyNamePattern.MatchString(keyName) || len(credentials.APIPrivateKey) > 4096 {
		return "", &financial.ProviderError{Code: financial.AuthorizationFailed}
	}
	key, err := parsePrivateKey(credentials.APIPrivateKey)
	if err != nil {
		return "", &financial.ProviderError{Code: financial.AuthorizationFailed}
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
