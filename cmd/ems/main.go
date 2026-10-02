package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bariskode/email-management-service/internal/audit"
	"github.com/bariskode/email-management-service/internal/config"
	"github.com/bariskode/email-management-service/internal/destination"
	"github.com/bariskode/email-management-service/internal/httpapi"
	"github.com/bariskode/email-management-service/internal/provider"
	"github.com/bariskode/email-management-service/internal/provider/cloudflare"
	"github.com/bariskode/email-management-service/internal/provider/mock"
	"github.com/bariskode/email-management-service/internal/routing"
	"github.com/bariskode/email-management-service/internal/storage"
	"github.com/bariskode/email-management-service/internal/storage/memory"
	"github.com/bariskode/email-management-service/internal/storage/postgres"
	emsSync "github.com/bariskode/email-management-service/internal/sync"
)

func main() {
	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", slog.Any("error", err))
		os.Exit(1)
	}

	// 2. Initialize logger
	var level slog.Level
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	logger.Info("starting Email Management Service (EMS)",
		slog.String("env", cfg.Env),
		slog.String("http_addr", cfg.HTTPAddr),
	)

	// 3. Initialize storage
	var repos *storage.Repositories
	var dbCloser func() error

	if cfg.DatabaseURL != "" {
		logger.Info("connecting to postgres database")
		pgDB, err := postgres.New(cfg.DatabaseURL)
		if err != nil {
			logger.Error("failed to connect to database", slog.Any("error", err))
			os.Exit(1)
		}
		repos = pgDB.Repositories()
		dbCloser = pgDB.Close
	} else {
		logger.Warn("no EMS_DATABASE_URL provided; using in-memory storage (data will not persist)")
		repos = memory.New()
	}

	// 4. Initialize Provider
	// If a global CLOUDFLARE_API_TOKEN is supplied in env, create real client, otherwise use mock
	var emailProvider provider.EmailProvider
	cfToken := os.Getenv("CLOUDFLARE_API_TOKEN")
	if cfToken != "" {
		logger.Info("initializing Cloudflare email provider")
		emailProvider = cloudflare.NewClient(cfToken)
	} else {
		logger.Info("no CLOUDFLARE_API_TOKEN provided; using mock provider")
		emailProvider = mock.New()
	}

	// 5. Initialize services
	auditService := audit.NewService(repos.Audit)
	destService := destination.NewService(repos.Destinations, emailProvider, auditService)
	routeService := routing.NewService(repos, emailProvider, destService, auditService)
	syncEngine := emsSync.NewEngine(repos, emailProvider, auditService)

	// 6. Build HTTP API and Router
	serverHandler := httpapi.NewServer(repos, destService, routeService, syncEngine, cfg.MasterKey)
	router := httpapi.NewRouter(serverHandler, cfg.APIKey, cfg.RateLimit, logger)

	httpServer := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      router,
		ReadTimeout:  cfg.RequestTimeout,
		WriteTimeout: cfg.RequestTimeout,
		IdleTimeout:  60 * time.Second,
	}

	// 7. Start server in goroutine
	serverErrChan := make(chan error, 1)
	go func() {
		logger.Info("HTTP server listening", slog.String("addr", cfg.HTTPAddr))
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrChan <- err
		}
	}()

	// 8. Graceful shutdown listening to OS signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrChan:
		logger.Error("server error encountered", slog.Any("error", err))
	case sig := <-quit:
		logger.Info("shutdown signal received", slog.String("signal", sig.String()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", slog.Any("error", err))
	}

	if dbCloser != nil {
		if err := dbCloser(); err != nil {
			logger.Error("error closing database", slog.Any("error", err))
		}
	}

	logger.Info("EMS server gracefully stopped")
}
