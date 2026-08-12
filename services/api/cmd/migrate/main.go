package main

import (
	"context"
	"database/sql"
	"github.com/arbion/platform/services/api/internal/platform/config"
	"github.com/arbion/platform/services/api/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"log/slog"
	"os"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	db, err := sql.Open("pgx", cfg.Database.URL)
	if err != nil {
		slog.Error("open migration database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	ctx := context.Background()
	if err = db.PingContext(ctx); err != nil {
		slog.Error("connect migration database", "error", err)
		os.Exit(1)
	}
	goose.SetBaseFS(migrations.Files)
	if err = goose.SetDialect("postgres"); err != nil {
		slog.Error("set migration dialect", "error", err)
		os.Exit(1)
	}
	if err = goose.UpContext(ctx, db, "."); err != nil {
		slog.Error("run migrations", "error", err)
		os.Exit(1)
	}
	slog.Info("database migrations complete")
}
