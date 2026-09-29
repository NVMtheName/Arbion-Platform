package coinbase

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/execution"
	"github.com/arbion/platform/services/api/internal/financial"
)

// ExecutionAdapter is deliberately separate from Client: constructing the
// existing read/preview client must never grant it the sender capability.
// There is no runtime registration or activation in this implementation.
type ExecutionAdapter struct{ client *Client }

func NewExecutionAdapter(c *Client) *ExecutionAdapter { return &ExecutionAdapter{client: c} }

var _ execution.ConfirmedOrderSender = (*ExecutionAdapter)(nil)
var _ execution.SubmissionLookup = (*ExecutionAdapter)(nil)

const executionHistoryPath = "/api/v3/brokerage/orders/historical/"

var executionRejectionCode = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`)

type executionSubmitRequest struct {
	ClientOrderID string `json:"client_order_id"`
	PreviewID     string `json:"preview_id"`
	executionPreviewRequest
}

type executionOrderIdentity struct {
	OrderID       *string `json:"order_id"`
	ClientOrderID *string `json:"client_order_id"`
	ProductID     *string `json:"product_id"`
	Side          *string `json:"side"`
}

type executionSubmitResponse struct {
	Success            *bool                   `json:"success"`
	SuccessResponse    *executionOrderIdentity `json:"success_response"`
	ErrorResponse      json.RawMessage         `json:"error_response"`
	OrderConfiguration json.RawMessage         `json:"order_configuration"`
}

type executionHistoryPage struct {
	Orders             *[]executionOrderIdentity `json:"orders"`
	HasNext            *bool                     `json:"has_next"`
	Cursor             *string                   `json:"cursor"`
	ProofTokenRequired *bool                     `json:"proof_token_required"`
}

type executionOrderDetail struct {
	Order *struct {
		executionOrderIdentity
		RetailPortfolioID          *string         `json:"retail_portfolio_id"`
		ProductType                *string         `json:"product_type"`
		OrderType                  *string         `json:"order_type"`
		TimeInForce                *string         `json:"time_in_force"`
		CreatedTime                *time.Time      `json:"created_time"`
		SizeInQuote                *bool           `json:"size_in_quote"`
		SizeInclusiveOfFees        *bool           `json:"size_inclusive_of_fees"`
		IsLiquidation              *bool           `json:"is_liquidation"`
		OrderConfiguration         json.RawMessage `json:"order_configuration"`
		AttachedOrderConfiguration json.RawMessage `json:"attached_order_configuration"`
		Leverage                   json.RawMessage `json:"leverage"`
		MarginType                 *string         `json:"margin_type"`
	} `json:"order"`
}

// isolatedClient gives each operation a private, standard transport. Every
// request uses a fresh HTTP/1 connection: net/http's reused-connection retry and
// HTTP/2 stream replay paths are therefore unavailable. POST additionally has
// no replayable GetBody or idempotency headers. Caller transports cannot wrap
// a hidden retry. Existing read/preview clients remain unchanged.
func (a *ExecutionAdapter) isolatedClient() (*Client, func(), error) {
	if a == nil || a.client == nil || a.client.http == nil || a.client.base == nil || a.client.now == nil {
		return nil, nil, invalidExecutionResponse()
	}
	transport := a.client.http.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	standard, ok := transport.(*http.Transport)
	if !ok || standard == nil {
		return nil, nil, invalidExecutionResponse()
	}
	t := standard.Clone()
	t.DisableKeepAlives = true
	t.ForceAttemptHTTP2 = false
	t.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	t.Protocols = new(http.Protocols)
	t.Protocols.SetHTTP1(true)
	if t.TLSClientConfig != nil {
		t.TLSClientConfig = t.TLSClientConfig.Clone()
		t.TLSClientConfig.NextProtos = []string{"http/1.1"}
	}
	base := *a.client.base
	c := &Client{base: &base, now: a.client.now, http: &http.Client{
		Transport: t, Timeout: a.client.http.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
	return c, t.CloseIdleConnections, nil
}

func executionSubmissionKey(credentials *financial.Credentials, portfolio string) (*financial.Credentials, error) {
	if credentials == nil || credentials.PortfolioID != portfolio || !executionUUID(portfolio) {
		return nil, invalidExecutionResponse()
	}
	key := *credentials
	if err := normalizeCredentials(&key); err != nil {
		return nil, err
	}
	return &key, nil
}

func executionCheckPermissions(ctx context.Context, c *Client, key *financial.Credentials, trade bool) error {
	var p executionPermissions
	if err := c.executionRequest(ctx, key, http.MethodGet, "/api/v3/brokerage/key_permissions", nil, &p); err != nil {
		return err
	}
	if !requiredExecutionBools(p.CanView, p.CanTrade, p.CanTransfer) || p.PortfolioUUID == nil ||
		!*p.CanView || *p.CanTransfer || (trade && !*p.CanTrade) || *p.PortfolioUUID != key.PortfolioID {
		return &financial.ProviderError{Code: financial.PermissionDenied}
	}
	return nil
}

// SubmitOnce is synchronous and has exactly one POST call site. Its caller must
// be the durable winning control-plane claim; this adapter never claims, retries,
// recovers, changes order terms or treats an acknowledgement as a fill.
func (a *ExecutionAdapter) SubmitOnce(ctx context.Context, credentials *financial.Credentials, s execution.ConfirmedSubmission) (execution.SubmissionAcknowledgement, error) {
	fail := func(err error) (execution.SubmissionAcknowledgement, error) {
		return execution.SubmissionAcknowledgement{}, err
	}
	deadline, ok := ctx.Deadline()
	remaining := time.Until(deadline)
	if !ok || remaining <= 0 || remaining > 5*time.Second || ctx.Err() != nil {
		return fail(invalidExecutionResponse())
	}
	c, closeTransport, err := a.isolatedClient()
	if err != nil {
		return fail(err)
	}
	defer closeTransport()
	if err := execution.ValidateSubmission(s, c.now().UTC()); err != nil {
		return fail(err)
	}
	key, err := executionSubmissionKey(credentials, s.PortfolioID)
	if err != nil {
		return fail(err)
	}
	if err := executionCheckPermissions(ctx, c, key, true); err != nil {
		return fail(err)
	}
	current := execution.ProviderPreflight{PortfolioID: s.PortfolioID}
	if err := c.executionAccounts(ctx, key, strings.TrimSuffix(s.Order.Request.ProductID, "-USD"), &current); err != nil {
		return fail(err)
	}
	for _, amounts := range [][2]string{{current.CashUSD, s.Preflight.CashUSD}, {current.AvailableCashUSD, s.Preflight.AvailableCashUSD},
		{current.TotalBase, s.Preflight.TotalBase}, {current.AvailableBase, s.Preflight.AvailableBase}} {
		if compareDecimal(financial.Decimal(amounts[0]), financial.Decimal(amounts[1])) != 0 {
			return fail(invalidExecutionResponse())
		}
	}
	query := url.Values{"retail_portfolio_id": {s.PortfolioID}, "limit": {"100"}, "order_status": {
		"PENDING", "OPEN", "QUEUED", "CANCEL_QUEUED", "EDIT_QUEUED", "UNKNOWN_ORDER_STATUS",
	}}
	var orders executionHistoryPage
	if err := c.submissionRequest(ctx, key, http.MethodGet, executionHistoryPath+"batch?"+query.Encode(), nil, &orders); err != nil {
		return fail(err)
	}
	if !validExecutionHistoryPage(orders) || *orders.HasNext || len(*orders.Orders) != 0 {
		return fail(invalidExecutionResponse())
	}
	// Read time consumes the original grant. Never renew it after provider I/O.
	if ctx.Err() != nil || execution.ValidateSubmission(s, c.now().UTC()) != nil {
		return fail(invalidExecutionResponse())
	}
	r := s.Order.Request
	input := executionSubmitRequest{ClientOrderID: r.ClientOrderID, PreviewID: s.PreviewID,
		executionPreviewRequest: executionPreviewRequest{ProductID: r.ProductID, Side: r.Side, RetailPortfolioID: s.PortfolioID}}
	input.OrderConfiguration.LimitIOC.BaseSize, input.OrderConfiguration.LimitIOC.LimitPrice = r.BaseSize, r.LimitPrice
	var response executionSubmitResponse
	if err := c.submissionRequest(ctx, key, http.MethodPost, "/api/v3/brokerage/orders", input, &response); err != nil {
		return fail(err)
	}
	if response.Success == nil || ctx.Err() != nil {
		return fail(invalidExecutionResponse())
	}
	if !*response.Success {
		var rejection struct {
			Code *string `json:"error"`
		}
		if response.SuccessResponse != nil || !emptyExecutionField(response.OrderConfiguration) ||
			json.Unmarshal(response.ErrorResponse, &rejection) != nil || rejection.Code == nil || !executionRejectionCode.MatchString(*rejection.Code) {
			return fail(invalidExecutionResponse())
		}
		return fail(&execution.SubmissionRejectedError{Code: *rejection.Code})
	}
	if response.SuccessResponse == nil || !emptyExecutionField(response.ErrorResponse) ||
		(!emptyExecutionField(response.OrderConfiguration) && !exactExecutionConfiguration(response.OrderConfiguration, r)) {
		return fail(invalidExecutionResponse())
	}
	ack, ok := exactExecutionIdentity(*response.SuccessResponse, r)
	if !ok {
		return fail(invalidExecutionResponse())
	}
	return ack, nil
}

// LookupSubmission cannot send. Absence in eventually-consistent or bounded
// history is never proof of non-submission and never permits another POST.
// Positive correlation requires the exact immutable terms from Get Order.
func (a *ExecutionAdapter) LookupSubmission(ctx context.Context, credentials *financial.Credentials, s execution.ConfirmedSubmission, attempt execution.Attempt) (execution.SubmissionAcknowledgement, error) {
	fail := func(err error) (execution.SubmissionAcknowledgement, error) {
		return execution.SubmissionAcknowledgement{}, err
	}
	c, closeTransport, err := a.isolatedClient()
	if err != nil {
		return fail(err)
	}
	defer closeTransport()
	if err := execution.ValidateRecoverySubmission(s, attempt, c.now().UTC()); err != nil {
		return fail(err)
	}
	key, err := executionSubmissionKey(credentials, s.PortfolioID)
	if err != nil {
		return fail(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := executionCheckPermissions(ctx, c, key, false); err != nil {
		return fail(err)
	}
	id := attempt.ProviderOrderID
	if id == "" {
		id, err = c.findExecutionOrder(ctx, key, s)
		if err != nil {
			return fail(err)
		}
	}
	var detail executionOrderDetail
	if err := c.submissionRequest(ctx, key, http.MethodGet, executionHistoryPath+id, nil, &detail); err != nil {
		return fail(err)
	}
	o := detail.Order
	if o == nil || o.RetailPortfolioID == nil || *o.RetailPortfolioID != s.PortfolioID ||
		o.ProductType == nil || *o.ProductType != "SPOT" || o.OrderType == nil || *o.OrderType != "LIMIT" ||
		o.TimeInForce == nil || *o.TimeInForce != "IMMEDIATE_OR_CANCEL" || o.CreatedTime == nil ||
		o.CreatedTime.Before(attempt.ClaimedAt) || o.CreatedTime.After(c.now().UTC()) ||
		!requiredExecutionBools(o.SizeInQuote, o.SizeInclusiveOfFees) || *o.SizeInQuote || *o.SizeInclusiveOfFees ||
		(o.IsLiquidation != nil && *o.IsLiquidation) || !emptyExecutionField(o.AttachedOrderConfiguration) ||
		!optionalExecutionDecimal(o.Leverage, "1") || (o.MarginType != nil && *o.MarginType != "") ||
		!exactExecutionConfiguration(o.OrderConfiguration, s.Order.Request) {
		return fail(invalidExecutionResponse())
	}
	ack, ok := exactExecutionIdentity(o.executionOrderIdentity, s.Order.Request)
	if !ok || ack.ProviderOrderID != id || ctx.Err() != nil {
		return fail(invalidExecutionResponse())
	}
	return ack, nil
}

func (c *Client) findExecutionOrder(ctx context.Context, key *financial.Credentials, s execution.ConfirmedSubmission) (string, error) {
	cursor, found := "", ""
	seenCursors, seenOrders := map[string]bool{}, map[string]bool{}
	for page := 0; page < maxPages; page++ {
		query := url.Values{"retail_portfolio_id": {s.PortfolioID}, "limit": {"100"}}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		var p executionHistoryPage
		if err := c.submissionRequest(ctx, key, http.MethodGet, executionHistoryPath+"batch?"+query.Encode(), nil, &p); err != nil {
			return "", err
		}
		if !validExecutionHistoryPage(p) {
			return "", invalidExecutionResponse()
		}
		for _, o := range *p.Orders {
			if o.OrderID == nil || !executionUUID(*o.OrderID) || seenOrders[*o.OrderID] || o.ClientOrderID == nil ||
				len(*o.ClientOrderID) == 0 || len(*o.ClientOrderID) > 128 {
				return "", invalidExecutionResponse()
			}
			seenOrders[*o.OrderID] = true
			if *o.ClientOrderID == s.Order.Request.ClientOrderID {
				if _, ok := exactExecutionIdentity(o, s.Order.Request); !ok || found != "" {
					return "", invalidExecutionResponse()
				}
				found = *o.OrderID
			}
		}
		if !*p.HasNext {
			if found == "" {
				return "", execution.ErrSubmissionUnknown
			}
			return found, nil
		}
		if *p.Cursor == "" || seenCursors[*p.Cursor] || len(*p.Orders) == 0 {
			return "", invalidExecutionResponse()
		}
		cursor = *p.Cursor
		seenCursors[cursor] = true
	}
	return "", invalidExecutionResponse()
}

func validExecutionHistoryPage(p executionHistoryPage) bool {
	return p.Orders != nil && len(*p.Orders) <= 100 && p.HasNext != nil && p.Cursor != nil &&
		len(*p.Cursor) <= 1024 && safeCursor(*p.Cursor) && strings.TrimSpace(*p.Cursor) == *p.Cursor &&
		(p.ProofTokenRequired == nil || !*p.ProofTokenRequired)
}

func exactExecutionIdentity(o executionOrderIdentity, r execution.Request) (execution.SubmissionAcknowledgement, bool) {
	if o.OrderID == nil || !executionUUID(*o.OrderID) || o.ClientOrderID == nil || *o.ClientOrderID != r.ClientOrderID ||
		o.ProductID == nil || *o.ProductID != r.ProductID || o.Side == nil || *o.Side != r.Side {
		return execution.SubmissionAcknowledgement{}, false
	}
	return execution.SubmissionAcknowledgement{ProviderOrderID: *o.OrderID, ClientOrderID: *o.ClientOrderID, ProductID: *o.ProductID, Side: *o.Side}, true
}

func emptyExecutionField(raw json.RawMessage) bool {
	return len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || bytes.Equal(bytes.TrimSpace(raw), []byte("{}"))
}

func exactExecutionConfiguration(raw json.RawMessage, r execution.Request) bool {
	var config map[string]json.RawMessage
	if json.Unmarshal(raw, &config) != nil || len(config) != 1 {
		return false
	}
	var ioc map[string]executionDecimal
	if json.Unmarshal(config["sor_limit_ioc"], &ioc) != nil || len(ioc) != 2 {
		return false
	}
	base, baseOK := ioc["base_size"]
	price, priceOK := ioc["limit_price"]
	return baseOK && priceOK && compareDecimal(financial.Decimal(base), financial.Decimal(r.BaseSize)) == 0 &&
		compareDecimal(financial.Decimal(price), financial.Decimal(r.LimitPrice)) == 0
}

// submissionRequest is a separate endpoint allowlist. Do not expand the
// read/preview transport to admit writes. Only a 200 fully decoded response can
// acknowledge or report rejection; HTTP errors carry sanitized codes only.
func (c *Client) submissionRequest(ctx context.Context, key *financial.Credentials, method, path string, input, output any) error {
	u, err := c.base.Parse(path)
	if err != nil || u.Host != c.base.Host || u.Scheme != c.base.Scheme || u.User != nil || u.Fragment != "" {
		return invalidExecutionResponse()
	}
	if method == http.MethodPost {
		if u.Path != "/api/v3/brokerage/orders" || u.RawQuery != "" || input == nil {
			return invalidExecutionResponse()
		}
	} else if method != http.MethodGet || input != nil || (u.Path != executionHistoryPath+"batch" &&
		!executionUUID(strings.TrimPrefix(u.Path, executionHistoryPath))) || !strings.HasPrefix(u.Path, executionHistoryPath) {
		return invalidExecutionResponse()
	}
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil || len(encoded) > 16*1024 {
			return invalidExecutionResponse()
		}
		body = bytes.NewReader(encoded)
	}
	token, err := c.jwt(key, method, u.Path)
	if err != nil {
		return err
	}
	r, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return invalidExecutionResponse()
	}
	r.GetBody = nil
	r.Close = true
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Accept", "application/json")
	if input != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(r)
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
		return invalidExecutionResponse()
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes+1))
	if err != nil || len(encoded) > maxBodyBytes || !executionUniqueJSON(encoded) || json.Unmarshal(encoded, output) != nil {
		return invalidExecutionResponse()
	}
	return nil
}
