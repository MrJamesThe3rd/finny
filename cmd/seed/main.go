package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/MrJamesThe3rd/finny/internal/config"
	"github.com/MrJamesThe3rd/finny/internal/database"
)

// seedUserID is the user row every migration since 20260404000001 creates, and
// the one the organization migration granted the owner membership to. Keyed on
// the id and not on username on purpose: after a db-reset an ON CONFLICT
// (username) upsert would mint a fresh uuid *after* the migration handed out
// memberships, leaving the only account you can log in as with none.
var seedUserID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	db, err := database.New(cfg.ConnectionString())
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	email := envOr("SEED_EMAIL", "admin@finny.local")
	username := envOr("SEED_USERNAME", "admin")
	name := envOr("SEED_NAME", "Admin")
	password := envOr("SEED_PASSWORD", "changeme")

	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		slog.Error("failed to hash password", "error", err)
		os.Exit(1)
	}

	// Upsert onto seedUserID so this UUID is stable across db-reset cycles.
	// The migration always creates this row; we just fill in credentials.
	_, err = db.ExecContext(context.Background(), `
		INSERT INTO users (id, email, username, name, password_hash, is_admin, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, TRUE, $6, $6)
		ON CONFLICT (id) DO UPDATE
		    SET email         = EXCLUDED.email,
		        username      = EXCLUDED.username,
		        name          = EXCLUDED.name,
		        password_hash = EXCLUDED.password_hash,
		        is_admin      = TRUE,
		        updated_at    = EXCLUDED.updated_at
	`, seedUserID, email, username, name, string(hash), time.Now())
	if err != nil {
		slog.Error("failed to upsert admin user", "error", err)
		os.Exit(1)
	}

	slog.Info("admin user ready",
		"id", seedUserID,
		"email", email,
		"username", username,
	)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
