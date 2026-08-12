package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/arbion/platform/services/api/internal/platform/config"
	"github.com/arbion/platform/services/api/internal/platform/database"
	platformhttp "github.com/arbion/platform/services/api/internal/platform/http"
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

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           platformhttp.NewHandler(pool, cfg.Database.ReadinessTimeout),
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
