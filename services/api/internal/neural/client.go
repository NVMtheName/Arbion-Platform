package neural

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

type ErrorCode string

const (
	AuthenticationFailed ErrorCode = "AUTHENTICATION_FAILED"
	RateLimited          ErrorCode = "RATE_LIMITED"
	ProviderUnavailable  ErrorCode = "PROVIDER_UNAVAILABLE"
	Timeout              ErrorCode = "TIMEOUT"
	InvalidRequest       ErrorCode = "INVALID_REQUEST"
	Unsupported          ErrorCode = "UNSUPPORTED"
	InternalError        ErrorCode = "INTERNAL_ERROR"
)

type ProviderError struct{ Code ErrorCode }

func (e *ProviderError) Error() string { return string(e.Code) }

type Model struct {
	ID           string   `json:"id"`
	DisplayName  string   `json:"display_name"`
	Provider     string   `json:"provider"`
	Capabilities []string `json:"capabilities"`
}

type InsightMetadata struct {
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Profile     string `json:"profile"`
	InputUsage  *int   `json:"input_usage,omitempty"`
	OutputUsage *int   `json:"output_usage,omitempty"`
	RequestID   string `json:"request_id,omitempty"`
	LatencyMS   *int   `json:"latency_ms,omitempty"`
}

type Insight struct {
	Summary             string          `json:"summary"`
	KeyPoints           []string        `json:"key_points"`
	RiskFlags           []string        `json:"risk_flags"`
	Limitations         []string        `json:"limitations"`
	RequiresCurrentData bool            `json:"requires_current_data"`
	Metadata            InsightMetadata `json:"metadata"`
}

type Client interface {
	Verify(context.Context, string, []byte) error
	Models(context.Context, string, []byte) ([]Model, error)
	Analyze(context.Context, string, string, []byte, string, string) (Insight, error)
}

type HTTPClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewHTTPClient(baseURL, token string, client *http.Client) *HTTPClient {
	return &HTTPClient{baseURL: baseURL, token: token, client: client}
}

func (c *HTTPClient) Verify(ctx context.Context, provider string, credential []byte) error {
	return c.call(ctx, "/internal/neural/verify", map[string]string{
		"provider": provider, "credential": string(credential),
	}, &struct{}{})
}
func (c *HTTPClient) Models(ctx context.Context, provider string, credential []byte) ([]Model, error) {
	var out struct {
		Models []Model `json:"models"`
	}
	if err := c.call(ctx, "/internal/neural/models", map[string]string{
		"provider": provider, "credential": string(credential),
	}, &out); err != nil {
		return nil, err
	}
	return out.Models, nil
}
func (c *HTTPClient) Analyze(ctx context.Context, provider, profile string, credential []byte, prompt, safetyIdentifier string) (Insight, error) {
	var out struct {
		Insight Insight `json:"insight"`
	}
	err := c.call(ctx, "/internal/neural/insight", map[string]string{
		"provider": provider, "credential": string(credential), "profile": profile,
		"prompt": prompt, "safety_identifier": safetyIdentifier,
	}, &out)
	return out.Insight, err
}
func (c *HTTPClient) call(ctx context.Context, path string, request, out any) error {
	body, err := json.Marshal(request)
	if err != nil {
		return errors.New("encode neural request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		clear(body)
		return errors.New("create neural request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := c.client.Do(req)
	clear(body)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return &ProviderError{Code: Timeout}
		}
		return &ProviderError{Code: ProviderUnavailable}
	}
	defer res.Body.Close()
	limited := io.LimitReader(res.Body, 2<<20+1)
	payload, err := io.ReadAll(limited)
	if err != nil || len(payload) > 2<<20 {
		return &ProviderError{Code: ProviderUnavailable}
	}
	if res.StatusCode >= 400 {
		var failure struct {
			Detail struct {
				Code ErrorCode `json:"code"`
			} `json:"detail"`
		}
		if json.Unmarshal(payload, &failure) == nil && failure.Detail.Code != "" {
			return &ProviderError{Code: failure.Detail.Code}
		}
		return &ProviderError{Code: InternalError}
	}
	if json.Unmarshal(payload, out) != nil {
		return &ProviderError{Code: InternalError}
	}
	return nil
}

func Code(err error) ErrorCode {
	var value *ProviderError
	if errors.As(err, &value) {
		return value.Code
	}
	return InternalError
}
