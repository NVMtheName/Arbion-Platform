package config

import (
	"encoding/base64"
	"errors"
	"fmt"
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
}

type Database struct {
	URL              string
	MaxConnections   int32
	MinConnections   int32
	ConnectTimeout   time.Duration
	ReadinessTimeout time.Duration
}

type Redis struct{ URL string }

type CredentialEncryption struct{ Key []byte }
type Auth struct {
	SessionCookie  string
	SessionTTL     time.Duration
	CookieSecure   bool
	AllowedOrigins []string
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
		return Config{}, errors.New("DATABASE_URL is required")
	}
	parsedDB, err := url.Parse(databaseURL)
	if err != nil || (parsedDB.Scheme != "postgres" && parsedDB.Scheme != "postgresql") || parsedDB.Host == "" || strings.Trim(parsedDB.Path, "/") == "" {
		return Config{}, errors.New("DATABASE_URL must be a valid PostgreSQL URL with host and database name")
	}
	if environment == Production && (parsedDB.Query().Get("sslmode") == "disable" || parsedDB.Query().Get("sslmode") == "") {
		return Config{}, errors.New("production DATABASE_URL must enable TLS with sslmode")
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
	origins := strings.Split(get("AUTH_ALLOWED_ORIGINS", "http://localhost:3000"), ",")
	return Config{Environment: environment, Port: get("PORT", "8080"), Database: Database{URL: databaseURL, MaxConnections: int32(maxConnections), MinConnections: int32(minConnections), ConnectTimeout: 10 * time.Second, ReadinessTimeout: 2 * time.Second}, Redis: Redis{URL: redisURL}, Credential: CredentialEncryption{Key: key}, Auth: Auth{SessionCookie: get("AUTH_SESSION_COOKIE", "arbion_session"), SessionTTL: ttl, CookieSecure: environment == Production, AllowedOrigins: origins}}, nil
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
