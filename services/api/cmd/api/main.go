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
	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/financial/oauthstate"
	"github.com/arbion/platform/services/api/internal/financial/schwab"
	"github.com/arbion/platform/services/api/internal/financialconnection"
	"github.com/arbion/platform/services/api/internal/neural"
	"github.com/arbion/platform/services/api/internal/platform/config"
	"github.com/arbion/platform/services/api/internal/platform/database"
	platformhttp "github.com/arbion/platform/services/api/internal/platform/http"
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
	authService := auth.NewService(users, sessions, sessions, users, cfg.Auth.SessionTTL)
	authorizationService := authorization.NewService(authorization.NewPostgresStore(pool), users)
	registry := aiconnection.DefaultRegistry()
	vault, err := credential.NewEncryptedVault(cfg.Credential.Key, credential.NewPostgresStore(pool))
	if err != nil {
		slog.Error("credential vault unavailable", "error", err)
		os.Exit(1)
	}
	neuralClient := neural.NewHTTPClient(cfg.AI.URL, cfg.AI.InternalToken, &http.Client{Timeout: cfg.AI.Timeout})
	aiConnections := aiconnection.NewService(aiconnection.NewPostgresStore(pool, registry), vault, users, registry, neuralClient)
	states := oauthstate.New(oauthstate.NewRedisStore(redisClient), 10*time.Minute)
	schwabClient := schwab.New(schwab.Config{ClientID: cfg.Schwab.ClientID, ClientSecret: cfg.Schwab.ClientSecret, RedirectURI: cfg.Schwab.RedirectURI, AuthorizationURL: cfg.Schwab.AuthorizationURL, TokenURL: cfg.Schwab.TokenURL, TraderBaseURL: cfg.Schwab.TraderBaseURL}, &http.Client{Timeout: cfg.Schwab.Timeout})
	financialConnections := financialconnection.NewService(financialconnection.NewPostgresStore(pool), vault, states, schwabClient, users)
	automations := automation.NewService(automation.NewPostgresStore(pool), users)
	strategies := strategy.NewInstanceService(strategy.NewPostgresStore(pool), automations)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           platformhttp.NewFullApplicationHandlerWithAutomation(pool, cfg, authService, authorizationService, aiConnections, financialConnections, automations, strategies),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	slog.Info("API listening", "address", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("API stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}
