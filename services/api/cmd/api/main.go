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

	"github.com/yisskqu/TwilightLog/services/api/internal/accounts"
	"github.com/yisskqu/TwilightLog/services/api/internal/audit"
	"github.com/yisskqu/TwilightLog/services/api/internal/auth"
	"github.com/yisskqu/TwilightLog/services/api/internal/migrations"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/config"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/db"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/httpx"
	"github.com/yisskqu/TwilightLog/services/api/internal/server"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if cfg.RunMigrations {
		if err := migrations.Up(ctx, cfg.DatabaseURL); err != nil {
			logger.Error("run migrations", "error", err)
			os.Exit(1)
		}
		logger.Info("database migrations applied")
	}

	auditService := audit.NewService(pool)
	authService, err := auth.NewService(pool, cfg, logger, auditService)
	if err != nil {
		logger.Error("initialize auth service", "error", err)
		os.Exit(1)
	}
	accountsService := accounts.NewService(pool, auditService)

	handler := httpx.NewRouterWithOptions(httpx.Options{
		Pinger:      pool,
		Logger:      logger,
		CORSOrigins: cfg.CORSOrigins,
		Register: server.Register(server.Deps{
			Auth:     authService,
			Accounts: accountsService,
			Audit:    auditService,
		}),
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("api listening", "addr", cfg.HTTPAddr, "env", cfg.AppEnv)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErr:
		logger.Error("api server error", "error", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown", "error", err)
	}
	logger.Info("api stopped")
}
