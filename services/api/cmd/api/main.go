package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/arbion/platform/services/api/internal/aiconnection"
	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/arbion/platform/services/api/internal/automationnotification"
	"github.com/arbion/platform/services/api/internal/credential"
	financialcoinbase "github.com/arbion/platform/services/api/internal/financial/coinbase"
	"github.com/arbion/platform/services/api/internal/financial/oauthstate"
	"github.com/arbion/platform/services/api/internal/financial/schwab"
	"github.com/arbion/platform/services/api/internal/financialconnection"
	"github.com/arbion/platform/services/api/internal/mailer"
	"github.com/arbion/platform/services/api/internal/marketintelligence"
	marketalpaca "github.com/arbion/platform/services/api/internal/marketintelligence/alpaca"
	marketcoinbase "github.com/arbion/platform/services/api/internal/marketintelligence/coinbase"
	"github.com/arbion/platform/services/api/internal/marketintelligence/coingecko"
	marketsec "github.com/arbion/platform/services/api/internal/marketintelligence/sec"
	"github.com/arbion/platform/services/api/internal/neural"
	"github.com/arbion/platform/services/api/internal/orderintent"
	"github.com/arbion/platform/services/api/internal/ownerattention"
	"github.com/arbion/platform/services/api/internal/platform/config"
	"github.com/arbion/platform/services/api/internal/platform/database"
	platformhttp "github.com/arbion/platform/services/api/internal/platform/http"
	"github.com/arbion/platform/services/api/internal/platformops"
	"github.com/arbion/platform/services/api/internal/risk"
	"github.com/arbion/platform/services/api/internal/strategy"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	pool, err := database.Open(context.Background(), cfg.Database)
	if err != nil {
		slog.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	redisOptions, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		slog.Error("invalid Redis configuration", "error", err)
		os.Exit(1)
	}
	redisClient := redis.NewClient(redisOptions)
	defer redisClient.Close()
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		slog.Error("Redis unavailable", "error", err)
		os.Exit(1)
	}
	users := auth.NewPostgresStore(pool)
	sessions := auth.NewRedisStore(redisClient)
	authService := auth.NewService(users, sessions, sessions, users, cfg.Auth.SessionTTL, auth.RegistrationPolicy{Restricted: cfg.Auth.RegistrationRestricted, AllowedEmails: cfg.Auth.RegistrationAllowlist})
	mfaProtector, err := auth.NewMFASecretProtector(cfg.Credential.Key)
	if err != nil {
		slog.Error("MFA secret protection unavailable", "error", err)
		os.Exit(1)
	}
	authService.ConfigureMFA(users, sessions, mfaProtector)
	var emailSender mailer.Sender = mailer.DisabledSender{}
	if cfg.Email.DeliveryMode == "smtp" {
		emailSender = mailer.NewSMTPSender(mailer.SMTPConfig{Host: cfg.Email.SMTPHost, Port: cfg.Email.SMTPPort, Username: cfg.Email.SMTPUsername, Password: cfg.Email.SMTPPassword, FromAddress: cfg.Email.FromAddress, FromName: cfg.Email.FromName, Timeout: 10 * time.Second})
	}
	authService.ConfigureEmail(users, emailSender, auth.EmailPolicy{VerificationRequired: cfg.Email.VerificationRequired, PublicBaseURL: cfg.Email.PublicBaseURL, VerificationTTL: cfg.Email.VerificationTTL, PasswordResetTTL: cfg.Email.PasswordResetTTL})
	authorizationService := authorization.NewService(authorization.NewPostgresStore(pool), users)
	registry := aiconnection.DefaultRegistry()
	vault, err := credential.NewEncryptedVault(cfg.Credential.Key, credential.NewPostgresStore(pool))
	if err != nil {
		slog.Error("credential vault unavailable", "error", err)
		os.Exit(1)
	}
	neuralClient := neural.NewHTTPClient(cfg.AI.URL, cfg.AI.InternalToken, &http.Client{Timeout: cfg.AI.Timeout})
	aiConnections := aiconnection.NewService(aiconnection.NewPostgresStore(pool, registry), vault, users, registry, neuralClient, sessions)
	states := oauthstate.New(oauthstate.NewRedisStore(redisClient), 10*time.Minute)
	schwabClient := schwab.New(schwab.Config{ClientID: cfg.Schwab.ClientID, ClientSecret: cfg.Schwab.ClientSecret, RedirectURI: cfg.Schwab.RedirectURI, AuthorizationURL: cfg.Schwab.AuthorizationURL, TokenURL: cfg.Schwab.TokenURL, TraderBaseURL: cfg.Schwab.TraderBaseURL, MarketDataBaseURL: cfg.Schwab.MarketDataBaseURL}, &http.Client{Timeout: cfg.Schwab.Timeout})
	coinbaseClient, err := financialcoinbase.New(financialcoinbase.Config{Timeout: 10 * time.Second}, nil)
	if err != nil {
		slog.Error("Coinbase financial connection unavailable", "error", err)
		os.Exit(1)
	}
	financialConnections := financialconnection.NewService(financialconnection.NewPostgresStore(pool), vault, states, schwabClient, users, financialconnection.NamedProvider{ID: "coinbase", Provider: coinbaseClient})
	orderIntents := orderintent.NewService(orderintent.NewPostgresStore(pool), financialConnections, authService, users, aiConnections)
	automations := automation.NewService(automation.NewPostgresStore(pool), users)
	strategyStore := strategy.NewPostgresStore(pool)
	strategies := strategy.NewInstanceService(strategyStore, automations, users)
	strategies.ConfigureEvidenceReview(authService)
	breakers := risk.NewBreakerService(risk.NewPostgresBreakerStore(pool), users, authService)
	platformOperations := platformops.NewService(platformops.NewPostgresStore(pool), users)
	ownerAttention := ownerattention.NewService(ownerattention.NewPostgresStore(pool))
	markets, err := newMarketIntelligenceService(cfg.MarketData, marketintelligence.NewPostgresHealthStore(pool), marketintelligence.NewPostgresWatchlistStore(pool))
	if err != nil {
		slog.Error("market intelligence unavailable", "error", err)
		os.Exit(1)
	}
	probeContext, cancelProbe := context.WithTimeout(context.Background(), cfg.MarketData.RequestTimeout+2*time.Second)
	markets.Probe(probeContext)
	cancelProbe()
	availableSources := 0
	for _, source := range markets.Sources() {
		if source.Enabled && source.Healthy {
			availableSources++
		}
	}
	slog.Info("market intelligence initialized", "available_sources", availableSources)
	evaluations := strategy.NewEvaluationService(strategyStore, automations, financialConnections)
	evaluations.ConfigureAIShadow(aiConnections, markets)
	if cfg.Scheduler.Enabled {
		notifications := automationnotification.NewEmailSender(emailSender, cfg.Email.PublicBaseURL)
		scheduler := strategy.NewScheduler(strategyStore, evaluations, notifications)
		scheduler.ConfigureReconciliation(financialConnections)
		go scheduler.Run(context.Background())
		slog.Info("non-live strategy scheduler enabled")
	}

	applicationHandler := platformhttp.NewFullApplicationHandlerWithEvaluationMarketsOrderIntentsAndPlatformOperations(pool, cfg, authService, authorizationService, aiConnections, financialConnections, automations, strategies, evaluations, breakers, markets, orderIntents, platformOperations)
	applicationHandler = platformhttp.WithOwnerAttention(applicationHandler, cfg, authService, ownerAttention)
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           applicationHandler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      45 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	slog.Info("API listening", "address", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("API stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}

func newMarketIntelligenceService(cfg config.MarketData, healthHistory marketintelligence.HealthHistoryStore, watchlists marketintelligence.WatchlistStore) (*marketintelligence.Service, error) {
	client := &http.Client{}
	var equity marketintelligence.EquityQuoteProvider
	var equitySourceID string
	if cfg.AlpacaKeyID != "" {
		provider, err := marketalpaca.New(marketalpaca.Config{
			KeyID: cfg.AlpacaKeyID, SecretKey: cfg.AlpacaSecretKey, BaseURL: cfg.AlpacaBaseURL,
			EquityFeed: cfg.AlpacaEquityFeed, Timeout: cfg.RequestTimeout,
			MaxAge: cfg.EquityMaxAge, MaxFutureSkew: cfg.MaxFutureSkew,
		}, client)
		if err != nil {
			return nil, err
		}
		equity = provider
		equitySourceID = "alpaca_" + cfg.AlpacaEquityFeed
	}

	var crypto marketintelligence.CryptoMarketProvider
	cryptoSourceID := "coinbase_exchange"
	cryptoCacheTTL := 5 * time.Second
	cryptoInterval := 500 * time.Millisecond
	coinbaseProvider, err := marketcoinbase.New(marketcoinbase.Config{
		BaseURL: cfg.CoinbaseBaseURL, Timeout: cfg.RequestTimeout,
		MaxAge: cfg.CryptoMaxAge, MaxFutureSkew: cfg.MaxFutureSkew,
	}, client)
	if err != nil {
		return nil, err
	}
	crypto = coinbaseProvider
	if cfg.CoinGeckoAPIKey != "" {
		coinGeckoProvider, coinGeckoErr := coingecko.New(coingecko.Config{
			APIKey: cfg.CoinGeckoAPIKey, Tier: cfg.CoinGeckoTier, BaseURL: cfg.CoinGeckoBaseURL,
			Timeout: cfg.RequestTimeout, MaxAge: cfg.CryptoMaxAge, MaxFutureSkew: cfg.MaxFutureSkew,
		}, client)
		if coinGeckoErr != nil {
			return nil, coinGeckoErr
		}
		crypto = coinGeckoProvider
		cryptoSourceID = "coingecko_rest"
		cryptoCacheTTL = cfg.CryptoCacheTTL
		cryptoInterval = cfg.CryptoRateInterval
	}

	var filings marketintelligence.InsiderFilingProvider
	if cfg.SECEdgarUserAgent != "" {
		provider, err := marketsec.New(marketsec.Config{
			UserAgent: cfg.SECEdgarUserAgent, BaseURL: cfg.SECEdgarBaseURL,
			Timeout: cfg.RequestTimeout, RateInterval: cfg.SECRateInterval, MaxFutureSkew: cfg.MaxFutureSkew,
		}, client)
		if err != nil {
			return nil, err
		}
		filings = provider
	}

	return marketintelligence.NewService(marketintelligence.ServiceConfig{
		HealthHistory:  healthHistory,
		Watchlists:     watchlists,
		EquityProvider: equity, EquitySourceID: equitySourceID, EquityCacheTTL: cfg.EquityCacheTTL, EquityInterval: cfg.EquityRateInterval,
		CryptoProvider: crypto, CryptoSourceID: cryptoSourceID, CryptoCacheTTL: cryptoCacheTTL, CryptoInterval: cryptoInterval,
		CryptoAssetProvider: coinbaseProvider, CryptoAssetSourceID: "coinbase_exchange",
		CryptoCandleProvider: coinbaseProvider, CryptoCandleSourceID: "coinbase_exchange", CryptoCandleCacheTTL: time.Minute, CryptoCandleInterval: 500 * time.Millisecond,
		CryptoLiquidityProvider: coinbaseProvider, CryptoLiquiditySourceID: "coinbase_exchange", CryptoLiquidityCacheTTL: time.Second, CryptoLiquidityInterval: 250 * time.Millisecond,
		CryptoTradeProvider: coinbaseProvider, CryptoTradeSourceID: "coinbase_exchange", CryptoTradeCacheTTL: time.Second, CryptoTradeInterval: 250 * time.Millisecond,
		CryptoStatsProvider: coinbaseProvider, CryptoStatsSourceID: "coinbase_exchange", CryptoStatsCacheTTL: 30 * time.Second, CryptoStatsInterval: 250 * time.Millisecond,
		FilingProvider: filings, FilingCacheTTL: cfg.InsiderFilingTTL, FilingInterval: cfg.SECRateInterval,
	})
}
