package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/MrJamesThe3rd/finny/internal/auth"
	authStore "github.com/MrJamesThe3rd/finny/internal/auth/store"
	"github.com/MrJamesThe3rd/finny/internal/config"
	"github.com/MrJamesThe3rd/finny/internal/database"
	"github.com/MrJamesThe3rd/finny/internal/document"
	"github.com/MrJamesThe3rd/finny/internal/document/local"
	"github.com/MrJamesThe3rd/finny/internal/document/paperless"
	docStore "github.com/MrJamesThe3rd/finny/internal/document/store"
	"github.com/MrJamesThe3rd/finny/internal/export"
	finnyHttp "github.com/MrJamesThe3rd/finny/internal/http"
	authHandler "github.com/MrJamesThe3rd/finny/internal/http/auth"
	docHandler "github.com/MrJamesThe3rd/finny/internal/http/document"
	exportHandler "github.com/MrJamesThe3rd/finny/internal/http/export"
	importHandler "github.com/MrJamesThe3rd/finny/internal/http/importcsv"
	matchingHandler "github.com/MrJamesThe3rd/finny/internal/http/matching"
	txHandler "github.com/MrJamesThe3rd/finny/internal/http/transaction"
	"github.com/MrJamesThe3rd/finny/internal/importer"
	"github.com/MrJamesThe3rd/finny/internal/matching"
	matchingStore "github.com/MrJamesThe3rd/finny/internal/matching/store"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
	txStore "github.com/MrJamesThe3rd/finny/internal/transaction/store"
)

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

	registry := document.NewRegistry()
	registry.Register("paperless", paperless.NewFromConfig)
	registry.Register("local", local.NewFromConfig)

	var (
		authService        = auth.NewService(authStore.New(db), cfg.Auth.JWTSecret, cfg.Auth.AccessTokenExpiry, cfg.Auth.RefreshTokenExpiry)
		transactionService = transaction.NewService(txStore.New(db))
		matchingService    = matching.NewService(matchingStore.New(db))
		importService      = importer.NewService()
		documentService    = document.NewService(docStore.New(db), registry)
		exportService      = export.NewService(transactionService, documentService)
	)

	if cfg.Paperless.BaseURL != "" {
		// Seed with the default user until Phase 2 org management is in place.
		seedCtx := auth.WithUserID(context.Background(), auth.DefaultUserID)
		if err := documentService.SeedLegacyBackend(seedCtx, cfg.Paperless.BaseURL, cfg.Paperless.Token); err != nil {
			slog.Warn("failed to seed legacy paperless backend", "error", err)
		}
	}

	var (
		authH        = authHandler.NewHandler(authService)
		transactionH = txHandler.NewHandler(transactionService)
		importH      = importHandler.NewHandler(importService, transactionService, matchingService)
		matchingH    = matchingHandler.NewHandler(matchingService)
		exportH      = exportHandler.NewHandler(exportService)
		documentH    = docHandler.NewHandler(documentService, transactionService, registry)
	)

	router := finnyHttp.New(
		transactionH,
		importH,
		matchingH,
		exportH,
		documentH,
		authH,
		finnyHttp.Config{
			JWTSecret:         cfg.Auth.JWTSecret,
			CORSAllowedOrigin: cfg.Auth.CORSAllowedOrigin,
		},
	)

	port := fmt.Sprintf(":%d", cfg.App.Port)
	slog.Info("starting server", "port", port)

	if err := http.ListenAndServe(port, router); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
