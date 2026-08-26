// Package sec implements read-only SEC EDGAR ownership-filing discovery.
package sec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/arbion/platform/services/api/internal/marketintelligence"
)

const maxResponseBytes = 8 << 20

var (
	ErrInvalidConfiguration = errors.New("invalid SEC EDGAR configuration")
	ErrRateLimited          = errors.New("SEC EDGAR rate limited")
	ErrNotFound             = errors.New("SEC EDGAR issuer not found")
	ErrUnavailable          = errors.New("SEC EDGAR unavailable")
	ErrInvalidResponse      = errors.New("invalid SEC EDGAR response")
)

var (
	emailPattern     = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	cikPattern       = regexp.MustCompile(`^[0-9]{1,10}$`)
	accessionPattern = regexp.MustCompile(`^[0-9]{10}-[0-9]{2}-[0-9]{6}$`)
)

type Config struct {
	UserAgent     string
	BaseURL       string
	FilesBaseURL  string
	Timeout       time.Duration
	RateInterval  time.Duration
	MaxFutureSkew time.Duration
}

type Client struct {
	userAgent     string
	baseURL       *url.URL
	filesBaseURL  *url.URL
	rateInterval  time.Duration
	maxFutureSkew time.Duration
	http          *http.Client
	mu            sync.Mutex
	nextRequest   time.Time
}

type submissionsResponse struct {
	Filings struct {
		Recent struct {
			AccessionNumber    []string `json:"accessionNumber"`
			FilingDate         []string `json:"filingDate"`
			ReportDate         []string `json:"reportDate"`
			AcceptanceDateTime []string `json:"acceptanceDateTime"`
			Form               []string `json:"form"`
			PrimaryDocument    []string `json:"primaryDocument"`
		} `json:"recent"`
	} `json:"filings"`
}

type companyTickerEntry struct {
	CIK    int64  `json:"cik_str"`
	Ticker string `json:"ticker"`
	Title  string `json:"title"`
}

func New(config Config, httpClient *http.Client) (*Client, error) {
	config.UserAgent = strings.TrimSpace(config.UserAgent)
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if config.BaseURL == "" {
		config.BaseURL = "https://data.sec.gov"
	}
	config.FilesBaseURL = strings.TrimRight(strings.TrimSpace(config.FilesBaseURL), "/")
	if config.FilesBaseURL == "" {
		config.FilesBaseURL = "https://www.sec.gov"
	}
	if len(config.UserAgent) < 8 || len(config.UserAgent) > 256 || !emailPattern.MatchString(config.UserAgent) || config.Timeout <= 0 || config.RateInterval < 100*time.Millisecond || config.MaxFutureSkew < 0 || httpClient == nil {
		return nil, ErrInvalidConfiguration
	}
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil || baseURL.Host == "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" || !approvedBaseURL(baseURL) {
		return nil, ErrInvalidConfiguration
	}
	filesBaseURL, err := url.Parse(config.FilesBaseURL)
	if err != nil || filesBaseURL.Host == "" || filesBaseURL.User != nil || filesBaseURL.RawQuery != "" || filesBaseURL.Fragment != "" || !approvedFilesBaseURL(filesBaseURL) {
		return nil, ErrInvalidConfiguration
	}
	configuredHTTP := *httpClient
	configuredHTTP.Timeout = config.Timeout
	return &Client{userAgent: config.UserAgent, baseURL: baseURL, filesBaseURL: filesBaseURL, rateInterval: config.RateInterval, maxFutureSkew: config.MaxFutureSkew, http: &configuredHTTP}, nil
}

func approvedBaseURL(baseURL *url.URL) bool {
	if baseURL.String() == "https://data.sec.gov" {
		return true
	}
	if baseURL.Scheme != "http" || baseURL.Path != "" {
		return false
	}
	host := baseURL.Hostname()
	return host == "localhost" || net.ParseIP(host).IsLoopback()
}

func approvedFilesBaseURL(baseURL *url.URL) bool {
	if baseURL.String() == "https://www.sec.gov" {
		return true
	}
	if baseURL.Scheme != "http" || baseURL.Path != "" {
		return false
	}
	host := baseURL.Hostname()
	return host == "localhost" || net.ParseIP(host).IsLoopback()
}

// IssuerReferences returns the bounded SEC-published company_tickers file in
// one request so callers can cache the reference set without redownloading it
// for each allowlisted symbol. It is not an exchange listing assertion.
func (client *Client) IssuerReferences(ctx context.Context) ([]marketintelligence.IssuerReferenceObservation, error) {
	if err := client.wait(ctx); err != nil {
		return nil, ErrUnavailable
	}
	endpoint := *client.filesBaseURL
	endpoint.Path = "/files/company_tickers.json"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", client.userAgent)
	response, err := client.http.Do(request)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, statusError(response.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(payload) > maxResponseBytes {
		return nil, ErrInvalidResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	var entries map[string]companyTickerEntry
	if err = decoder.Decode(&entries); err != nil || len(entries) == 0 || len(entries) > 25_000 {
		return nil, ErrInvalidResponse
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, ErrInvalidResponse
	}
	now := time.Now().UTC()
	resolved := make(map[string]marketintelligence.IssuerReferenceObservation, len(entries))
	for key, entry := range entries {
		ticker := strings.ToUpper(strings.TrimSpace(entry.Ticker))
		name := strings.TrimSpace(entry.Title)
		if !digitsOnly(key) || entry.CIK <= 0 || entry.CIK > 9_999_999_999 || !validTicker(ticker) || name == "" || len(name) > 512 {
			return nil, ErrInvalidResponse
		}
		observation := marketintelligence.IssuerReferenceObservation{
			Symbol: ticker, IssuerCIK: fmt.Sprintf("%010d", entry.CIK), Name: name,
			Receipt: marketintelligence.SourceReceipt{Provider: "sec_edgar", Role: marketintelligence.ReferenceData, Feed: "company_tickers", Quality: marketintelligence.AggregatedReference, ReceivedAt: now},
		}
		if err = marketintelligence.ValidateIssuerReference(observation, now, time.Minute, client.maxFutureSkew); err != nil {
			return nil, err
		}
		if existing, exists := resolved[ticker]; exists && existing.IssuerCIK != observation.IssuerCIK {
			return nil, ErrInvalidResponse
		}
		resolved[ticker] = observation
	}
	result := make([]marketintelligence.IssuerReferenceObservation, 0, len(resolved))
	for _, observation := range resolved {
		result = append(result, observation)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Symbol < result[right].Symbol })
	return result, nil
}

func validTicker(value string) bool {
	if len(value) < 1 || len(value) > 15 || value[0] < 'A' || value[0] > 'Z' {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '.' && character != '-' {
			return false
		}
	}
	return true
}

func digitsOnly(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func (client *Client) RecentInsiderFilings(ctx context.Context, cik string, limit int) ([]marketintelligence.InsiderFilingObservation, error) {
	cik = strings.TrimSpace(cik)
	if !cikPattern.MatchString(cik) || limit < 1 || limit > 100 {
		return nil, marketintelligence.ErrInvalidObservation
	}
	cik = strings.Repeat("0", 10-len(cik)) + cik
	if strings.TrimLeft(cik, "0") == "" {
		return nil, marketintelligence.ErrInvalidObservation
	}
	if err := client.wait(ctx); err != nil {
		return nil, ErrUnavailable
	}

	endpoint := *client.baseURL
	endpoint.Path = "/submissions/CIK" + cik + ".json"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", client.userAgent)

	response, err := client.http.Do(request)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, statusError(response.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(payload) > maxResponseBytes {
		return nil, ErrInvalidResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	var raw submissionsResponse
	if err = decoder.Decode(&raw); err != nil {
		return nil, ErrInvalidResponse
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, ErrInvalidResponse
	}
	recent := raw.Filings.Recent
	count := len(recent.AccessionNumber)
	if len(recent.FilingDate) != count || len(recent.ReportDate) != count || len(recent.AcceptanceDateTime) != count || len(recent.Form) != count || len(recent.PrimaryDocument) != count {
		return nil, ErrInvalidResponse
	}

	now := time.Now().UTC()
	result := make([]marketintelligence.InsiderFilingObservation, 0, limit)
	for index := 0; index < count && len(result) < limit; index++ {
		form := strings.TrimSpace(recent.Form[index])
		if !isInsiderForm(form) {
			continue
		}
		observation, normalizeErr := client.normalize(cik, form, recent.AccessionNumber[index], recent.FilingDate[index], recent.ReportDate[index], recent.AcceptanceDateTime[index], recent.PrimaryDocument[index], now)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		result = append(result, observation)
	}
	return result, nil
}

func (client *Client) wait(ctx context.Context) error {
	for {
		client.mu.Lock()
		now := time.Now()
		delay := client.nextRequest.Sub(now)
		if delay <= 0 {
			client.nextRequest = now.Add(client.rateInterval)
			client.mu.Unlock()
			return nil
		}
		client.mu.Unlock()

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (client *Client) normalize(cik, form, accession, filingDate, reportDate, acceptedAt, primaryDocument string, now time.Time) (marketintelligence.InsiderFilingObservation, error) {
	accession = strings.TrimSpace(accession)
	primaryDocument = strings.TrimSpace(primaryDocument)
	if !accessionPattern.MatchString(accession) || !safeDocumentPath(primaryDocument) {
		return marketintelligence.InsiderFilingObservation{}, ErrInvalidResponse
	}
	accepted, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(acceptedAt))
	if err != nil {
		return marketintelligence.InsiderFilingObservation{}, ErrInvalidResponse
	}
	filed, err := time.Parse("2006-01-02", strings.TrimSpace(filingDate))
	if err != nil || filed.After(accepted) {
		return marketintelligence.InsiderFilingObservation{}, ErrInvalidResponse
	}
	reportDate = strings.TrimSpace(reportDate)
	if reportDate != "" {
		if _, err = time.Parse("2006-01-02", reportDate); err != nil {
			return marketintelligence.InsiderFilingObservation{}, ErrInvalidResponse
		}
	}
	issuerPath := strings.TrimLeft(cik, "0")
	accessionPath := strings.ReplaceAll(accession, "-", "")
	sourceURL := "https://www.sec.gov/Archives/edgar/data/" + issuerPath + "/" + accessionPath + "/" + escapeDocumentPath(primaryDocument)
	observation := marketintelligence.InsiderFilingObservation{
		IssuerCIK: cik, AccessionNumber: accession, Form: form, IsAmendment: strings.HasSuffix(form, "/A"),
		FiledAt: accepted.UTC(), ReportDate: reportDate, PrimaryDocument: primaryDocument, SourceURL: sourceURL,
		Provenance: marketintelligence.Provenance{
			Provider: "sec_edgar", Role: marketintelligence.PrimaryFiling, Feed: "submissions", Quality: marketintelligence.Filing,
			ProviderTimestamp: accepted.UTC(), ReceivedAt: now,
		},
	}
	if err = marketintelligence.ValidateInsiderFiling(observation, now, client.maxFutureSkew); err != nil {
		return marketintelligence.InsiderFilingObservation{}, err
	}
	return observation, nil
}

func safeDocumentPath(value string) bool {
	if value == "" || len(value) > 512 || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || path.Clean(value) != value {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func escapeDocumentPath(value string) string {
	segments := strings.Split(value, "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	return strings.Join(segments, "/")
}

func isInsiderForm(form string) bool {
	switch form {
	case "3", "3/A", "4", "4/A", "5", "5/A":
		return true
	default:
		return false
	}
}

func statusError(status int) error {
	switch status {
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusForbidden, http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		return ErrUnavailable
	}
}

var _ marketintelligence.InsiderFilingProvider = (*Client)(nil)
var _ marketintelligence.InsiderIssuerProvider = (*Client)(nil)
