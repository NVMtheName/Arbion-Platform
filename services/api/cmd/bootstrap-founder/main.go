package main

import (
	"context"
	"github.com/arbion/platform/services/api/internal/auth"
	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/platform/config"
	"github.com/arbion/platform/services/api/internal/platform/database"
	"log/slog"
	"os"
)

func main() {
	cfg, e := config.Load()
	if e != nil {
		slog.Error("invalid configuration", "error", e)
		os.Exit(1)
	}
	email := os.Getenv("FOUNDER_EMAIL")
	if auth.NormalizeEmail(email) == "" {
		slog.Error("FOUNDER_EMAIL is required")
		os.Exit(1)
	}
	ctx := context.Background()
	db, e := database.Open(ctx, cfg.Database)
	if e != nil {
		slog.Error("database unavailable")
		os.Exit(1)
	}
	defer db.Close()
	store := authorization.NewPostgresStore(db)
	u, changed, e := store.BootstrapFounder(ctx, email)
	if e != nil {
		slog.Error("founder account was not found or could not be restored")
		os.Exit(1)
	}
	aud := auth.NewPostgresStore(db)
	id := u.ID
	_ = aud.Record(ctx, &id, "authorization.founder_bootstrap", map[string]any{"target_user_id": u.ID, "changed": changed})
	slog.Info("founder bootstrap complete", "user_id", u.ID, "changed", changed)
}
