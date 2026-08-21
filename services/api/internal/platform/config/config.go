package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Environment string

const (
	Development Environment = "development"
	Test        Environment = "test"
	Production  Environment = "production"
)

type Config struct {
	Environment Environment
	Port        string
	Database    Database
	Redis       Redis
	Credential  CredentialEncryption
	Auth        Auth
	Email       Email
	AI          AIService
	Schwab      Schwab
	MarketData  MarketData
	Scheduler   Scheduler
}

type Database struct {
	URL              string
	MaxConnections   int32
	MinConnections   int32
	ConnectTimeout   time.Duration
	ReadinessTimeout time.Duration
}

type Redis struct{ URL string }
type Schwab struct {
	ClientID, ClientSecret, RedirectURI, AuthorizationURL, TokenURL, TraderBaseURL, MarketDataBaseURL string
	Timeout                                                                                           time.Duration
}
type AIService struct {
	URL           string
	InternalToken string
	Timeout       time.Duration
}

type Scheduler struct{ Enabled bool }

type MarketData struct {
	AlpacaKeyID        string
	AlpacaSecretKey    string
	AlpacaBaseURL      string
	AlpacaEquityFeed   string
	CoinGeckoAPIKey    string
	CoinGeckoTier      string
	CoinGeckoBaseURL   string
	SECEdgarUserAgent  string
	SECEdgarBaseURL    string
	RequestTimeout     time.Duration
	EquityMaxAge       time.Duration
	CryptoMaxAge       time.Duration
	MaxFutureSkew      time.Duration
	SECRateInterval    time.Duration
	EquityRateInterval time.Duration
	CryptoRateInterval time.Duration
	EquityCacheTTL     time.Duration
	CryptoCacheTTL     time.Duration
	InsiderFilingTTL   time.Duration
}

type CredentialEncryption struct{ Key []byte }
type Auth struct {
	SessionCookie          string
	SessionTTL             time.Duration
	CookieSecure           bool
	AllowedOrigins         []string
	RegistrationRestricted bool
	RegistrationAllowlist  []string
}

type Email struct {
	DeliveryMode         string
	FromAddress          string
	FromName             string
	PublicBaseURL        string
	SMTPHost             string
	SMTPPort             int
	SMTPUsername         string
	SMTPPassword         string
	VerificationRequired bool
	VerificationTTL      time.Duration
	PasswordResetTTL     time.Duration
}

func Load() (Config, error) { return LoadFrom(os.LookupEnv) }

func LoadFrom(lookup func(string) (string, bool)) (Config, error) {
	get := func(key, fallback string) string {
		if value, ok := lookup(key); ok {
			return strings.TrimSpace(value)
		}
		return fallback
	}
	environment := Environment(get("ARBION_ENV", string(Development)))
	if environment != Development && environment != Test && environment != Production {
		return Config{}, fmt.Errorf("ARBION_ENV must be development, test, or production")
	}
	databaseURL := get("DATABASE_URL", "")
	if databaseURL == "" {
		databaseURL = databaseURLFromParts(get)
	}
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL or all DATABASE_HOST, DATABASE_NAME, DATABASE_USER, and DATABASE_PASSWORD fields are required")
	}
	parsedDB, err := url.Parse(databaseURL)
	if err != nil || (parsedDB.Scheme != "postgres" && parsedDB.Scheme != "postgresql") || parsedDB.Host == "" || strings.Trim(parsedDB.Path, "/") == "" {
		return Config{}, errors.New("DATABASE_URL must be a valid PostgreSQL URL with host and database name")
	}
	if environment == Production && (parsedDB.Query().Get("sslmode") == "disable" || parsedDB.Query().Get("sslmode") == "") {
		if parsedDB.Hostname() != "postgres" || parsedDB.Query().Get("sslmode") != "disable" {
			return Config{}, errors.New("production DATABASE_URL must enable TLS unless using the private Compose postgres service")
		}
	}
	if environment == Production && (parsedDB.User == nil || parsedDB.User.Username() == "" || passwordMissingOrDevelopment(parsedDB)) {
		return Config{}, errors.New("production DATABASE_URL must include non-development database credentials")
	}
	redisURL := get("REDIS_URL", "")
	parsedRedis, err := url.Parse(redisURL)
	if err != nil || (parsedRedis.Scheme != "redis" && parsedRedis.Scheme != "rediss") || parsedRedis.Host == "" {
		return Config{}, errors.New("REDIS_URL must be a valid redis URL")
	}
	keyText := get("CREDENTIAL_ENCRYPTION_KEY", "")
	key, err := base64.StdEncoding.DecodeString(keyText)
	if err != nil || len(key) != 32 {
		return Config{}, errors.New("CREDENTIAL_ENCRYPTION_KEY must be a base64-encoded 32-byte key")
	}
	if environment == Production && allZero(key) {
		return Config{}, errors.New("production CREDENTIAL_ENCRYPTION_KEY must not use the development placeholder")
	}
	maxConnections, err := positiveInt(get("DATABASE_MAX_CONNECTIONS", "10"))
	if err != nil {
		return Config{}, fmt.Errorf("DATABASE_MAX_CONNECTIONS: %w", err)
	}
	minConnections, err := nonnegativeInt(get("DATABASE_MIN_CONNECTIONS", "1"))
	if err != nil || minConnections > maxConnections {
		return Config{}, errors.New("DATABASE_MIN_CONNECTIONS must be non-negative and no greater than DATABASE_MAX_CONNECTIONS")
	}
	ttl, err := time.ParseDuration(get("AUTH_SESSION_TTL", "12h"))
	if err != nil || ttl < time.Minute {
		return Config{}, errors.New("AUTH_SESSION_TTL must be at least one minute")
	}
	origins, err := allowedOrigins(get("AUTH_ALLOWED_ORIGINS", "http://localhost:3000"), environment)
	if err != nil {
		return Config{}, err
	}
	registrationValue, registrationConfigured := lookup("REGISTRATION_ALLOWLIST")
	registrationAllowlist, err := allowedRegistrationEmails(registrationValue)
	if err != nil {
		return Config{}, err
	}
	registrationRestricted := environment == Production || (registrationConfigured && strings.TrimSpace(registrationValue) != "")
	schedulerEnabled, err := strconv.ParseBool(get("NONLIVE_SCHEDULER_ENABLED", "false"))
	if err != nil {
		return Config{}, errors.New("NONLIVE_SCHEDULER_ENABLED must be true or false")
	}
	email, err := emailConfiguration(get, environment)
	if err != nil {
		return Config{}, err
	}
	aiURL := strings.TrimRight(get("AI_SERVICE_URL", "http://localhost:8000"), "/")
	internalToken := get("AI_INTERNAL_SERVICE_TOKEN", "")
	if internalToken == "" {
		return Config{}, errors.New("AI_INTERNAL_SERVICE_TOKEN is required")
	}
	if environment == Production && (internalToken == "local-internal-development-token" || len(internalToken) < 32) {
		return Config{}, errors.New("production AI_INTERNAL_SERVICE_TOKEN must be a strong non-development secret")
	}
	schwab := Schwab{ClientID: get("SCHWAB_CLIENT_ID", ""), ClientSecret: get("SCHWAB_CLIENT_SECRET", ""), RedirectURI: get("SCHWAB_REDIRECT_URI", "http://localhost:8080/api/connections/financial/schwab/callback"), AuthorizationURL: get("SCHWAB_AUTHORIZATION_URL", "https://api.schwabapi.com/v1/oauth/authorize"), TokenURL: get("SCHWAB_TOKEN_URL", "https://api.schwabapi.com/v1/oauth/token"), TraderBaseURL: get("SCHWAB_TRADER_BASE_URL", "https://api.schwabapi.com/trader/v1"), MarketDataBaseURL: get("SCHWAB_MARKET_DATA_BASE_URL", "https://api.schwabapi.com/marketdata/v1"), Timeout: 10 * time.Second}
	if environment == Production {
		enabled := schwab.ClientID != "" || schwab.ClientSecret != ""
		if enabled && (schwab.ClientID == "" || schwab.ClientSecret == "" || schwab.RedirectURI != "https://www.arbion.ai/api/connections/financial/schwab/callback") {
			return Config{}, errors.New("production Schwab configuration requires client ID, client secret, and the approved callback URI")
		}
	}
	marketData, err := marketDataConfiguration(get)
	if err != nil {
		return Config{}, err
	}
	return Config{Environment: environment, Port: get("PORT", "8080"), Database: Database{URL: databaseURL, MaxConnections: int32(maxConnections), MinConnections: int32(minConnections), ConnectTimeout: 10 * time.Second, ReadinessTimeout: 2 * time.Second}, Redis: Redis{URL: redisURL}, Credential: CredentialEncryption{Key: key}, Auth: Auth{SessionCookie: get("AUTH_SESSION_COOKIE", "arbion_session"), SessionTTL: ttl, CookieSecure: environment == Production, AllowedOrigins: origins, RegistrationRestricted: registrationRestricted, RegistrationAllowlist: registrationAllowlist}, Email: email, AI: AIService{URL: aiURL, InternalToken: internalToken, Timeout: 40 * time.Second}, Schwab: schwab, MarketData: marketData, Scheduler: Scheduler{Enabled: schedulerEnabled}}, nil
}

func marketDataConfiguration(get func(string, string) string) (MarketData, error) {
	alpacaKeyID := get("ALPACA_MARKET_DATA_KEY_ID", "")
	alpacaSecretKey := get("ALPACA_MARKET_DATA_SECRET_KEY", "")
	if (alpacaKeyID == "") != (alpacaSecretKey == "") {
		return MarketData{}, errors.New("Alpaca market data configuration requires both key ID and secret key")
	}
	alpacaFeed := strings.ToLower(get("ALPACA_EQUITY_FEED", "iex"))
	if alpacaFeed != "iex" && alpacaFeed != "sip" {
		return MarketData{}, errors.New("ALPACA_EQUITY_FEED must be iex or sip")
	}

	coinGeckoKey := get("COINGECKO_API_KEY", "")
	coinGeckoTier := strings.ToLower(get("COINGECKO_API_TIER", "demo"))
	if coinGeckoTier != "demo" && coinGeckoTier != "pro" {
		return MarketData{}, errors.New("COINGECKO_API_TIER must be demo or pro")
	}

	return MarketData{
		AlpacaKeyID: alpacaKeyID, AlpacaSecretKey: alpacaSecretKey,
		AlpacaBaseURL: get("ALPACA_MARKET_DATA_BASE_URL", "https://data.alpaca.markets"), AlpacaEquityFeed: alpacaFeed,
		CoinGeckoAPIKey: coinGeckoKey, CoinGeckoTier: coinGeckoTier, CoinGeckoBaseURL: get("COINGECKO_BASE_URL", ""),
		SECEdgarUserAgent: get("SEC_EDGAR_USER_AGENT", ""), SECEdgarBaseURL: get("SEC_EDGAR_BASE_URL", "https://data.sec.gov"),
		RequestTimeout: 8 * time.Second, EquityMaxAge: 120 * time.Hour, CryptoMaxAge: 10 * time.Minute, MaxFutureSkew: 2 * time.Minute,
		SECRateInterval: 150 * time.Millisecond, EquityRateInterval: 350 * time.Millisecond, CryptoRateInterval: 2100 * time.Millisecond,
		EquityCacheTTL: 15 * time.Second, CryptoCacheTTL: time.Minute, InsiderFilingTTL: 5 * time.Minute,
	}, nil
}

func emailConfiguration(get func(string, string) string, environment Environment) (Email, error) {
	mode := strings.ToLower(get("EMAIL_DELIVERY_MODE", "disabled"))
	if mode != "disabled" && mode != "smtp" {
		return Email{}, errors.New("EMAIL_DELIVERY_MODE must be disabled or smtp")
	}
	required, err := strconv.ParseBool(get("EMAIL_VERIFICATION_REQUIRED", "false"))
	if err != nil {
		return Email{}, errors.New("EMAIL_VERIFICATION_REQUIRED must be true or false")
	}
	verificationTTL, err := time.ParseDuration(get("EMAIL_VERIFICATION_TTL", "24h"))
	if err != nil || verificationTTL < 15*time.Minute || verificationTTL > 7*24*time.Hour {
		return Email{}, errors.New("EMAIL_VERIFICATION_TTL must be between 15 minutes and 7 days")
	}
	resetTTL, err := time.ParseDuration(get("PASSWORD_RESET_TTL", "30m"))
	if err != nil || resetTTL < 5*time.Minute || resetTTL > 24*time.Hour {
		return Email{}, errors.New("PASSWORD_RESET_TTL must be between 5 minutes and 24 hours")
	}
	baseFallback := "http://localhost:3000"
	if environment == Production {
		baseFallback = "https://www.arbion.ai"
	}
	publicBaseURL := strings.TrimRight(get("EMAIL_PUBLIC_BASE_URL", baseFallback), "/")
	parsedBase, err := url.Parse(publicBaseURL)
	if err != nil || (parsedBase.Scheme != "http" && parsedBase.Scheme != "https") || parsedBase.Host == "" || parsedBase.Path != "" || parsedBase.RawQuery != "" || parsedBase.Fragment != "" {
		return Email{}, errors.New("EMAIL_PUBLIC_BASE_URL must be an origin without a path")
	}
	if environment == Production && publicBaseURL != "https://www.arbion.ai" {
		return Email{}, errors.New("production EMAIL_PUBLIC_BASE_URL must be https://www.arbion.ai")
	}
	port, err := positiveInt(get("SMTP_PORT", "587"))
	if err != nil || port > 65535 {
		return Email{}, errors.New("SMTP_PORT must be between 1 and 65535")
	}
	result := Email{
		DeliveryMode: mode, FromAddress: get("EMAIL_FROM_ADDRESS", ""), FromName: get("EMAIL_FROM_NAME", "Arbion"), PublicBaseURL: publicBaseURL,
		SMTPHost: get("SMTP_HOST", ""), SMTPPort: port, SMTPUsername: get("SMTP_USERNAME", ""), SMTPPassword: get("SMTP_PASSWORD", ""),
		VerificationRequired: required, VerificationTTL: verificationTTL, PasswordResetTTL: resetTTL,
	}
	if required && mode != "smtp" {
		return Email{}, errors.New("EMAIL_VERIFICATION_REQUIRED requires EMAIL_DELIVERY_MODE=smtp")
	}
	if mode == "smtp" {
		address, parseErr := mail.ParseAddress(result.FromAddress)
		if parseErr != nil || address.Address != result.FromAddress || result.SMTPHost == "" || result.SMTPUsername == "" || result.SMTPPassword == "" || strings.ContainsAny(result.FromName, "\r\n") {
			return Email{}, errors.New("smtp email delivery requires a valid sender, host, username, password, and safe sender name")
		}
	}
	return result, nil
}

func allowedRegistrationEmails(value string) ([]string, error) {
	seen := map[string]struct{}{}
	result := []string{}
	if strings.TrimSpace(value) == "" {
		return result, nil
	}
	for _, item := range strings.Split(value, ",") {
		normalized := strings.ToLower(strings.TrimSpace(item))
		parsed, err := mail.ParseAddress(normalized)
		if err != nil || parsed.Address != normalized || len(normalized) > 320 {
			return nil, errors.New("REGISTRATION_ALLOWLIST must contain valid comma-separated email addresses")
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result, nil
}

func databaseURLFromParts(get func(string, string) string) string {
	host, name, user, password := get("DATABASE_HOST", ""), get("DATABASE_NAME", ""), get("DATABASE_USER", ""), get("DATABASE_PASSWORD", "")
	if host == "" || name == "" || user == "" || password == "" {
		return ""
	}
	port := get("DATABASE_PORT", "5432")
	sslmode := get("DATABASE_SSLMODE", "require")
	result := &url.URL{Scheme: "postgres", Host: net.JoinHostPort(host, port), Path: "/" + name, User: url.UserPassword(user, password)}
	query := result.Query()
	query.Set("sslmode", sslmode)
	result.RawQuery = query.Encode()
	return result.String()
}

func passwordMissingOrDevelopment(parsed *url.URL) bool {
	password, ok := parsed.User.Password()
	return !ok || password == "" || password == "local-development-only"
}

func allowedOrigins(value string, environment Environment) ([]string, error) {
	items := strings.Split(value, ",")
	for i := range items {
		items[i] = strings.TrimSpace(items[i])
		parsed, err := url.Parse(items[i])
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || strings.Contains(items[i], "*") {
			return nil, errors.New("AUTH_ALLOWED_ORIGINS must contain explicit origins without wildcards or paths")
		}
		if environment == Production && (parsed.Scheme != "https" || items[i] != "https://www.arbion.ai") {
			return nil, errors.New("production AUTH_ALLOWED_ORIGINS must be https://www.arbion.ai")
		}
	}
	return items, nil
}

func allZero(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}

func positiveInt(value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0, errors.New("must be a positive integer")
	}
	return n, nil
}
func nonnegativeInt(value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, errors.New("must be a non-negative integer")
	}
	return n, nil
}
