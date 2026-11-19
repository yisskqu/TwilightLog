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
	"github.com/yisskqu/TwilightLog/services/api/internal/content"
	"github.com/yisskqu/TwilightLog/services/api/internal/media"
	"github.com/yisskqu/TwilightLog/services/api/internal/migrations"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/config"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/db"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/httpx"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/storage"
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
	contentService := content.NewService(content.NewRepository(pool))

	backend, err := storage.NewLocal(cfg.LocalStorageRoot, cfg.AssetBaseURL, cfg.APIBaseURL, cfg.MediaSigningSecret)
	if err != nil {
		logger.Error("initialize storage adapter", "error", err)
		os.Exit(1)
	}
	mediaRepo := media.NewRepository(pool)
	mediaService := media.NewService(
		mediaRepo,
		backend,
		media.NewLocalProcessor(backend, logger),
		media.ServiceConfig{
			Limits: media.Limits{
				MaxArticleBytes: cfg.MaxArticleBytes,
				MaxPhotoBytes:   cfg.MaxPhotoBytes,
			},
			BatchLimit:   cfg.MediaBatchLimit,
			ProcessAsync: cfg.MediaProcessAsync,
		},
		logger,
	)
	go runMediaCleanup(ctx, mediaService, logger)

	handler := httpx.NewRouterWithOptions(httpx.Options{
		Pinger:      pool,
		Logger:      logger,
		CORSOrigins: cfg.CORSOrigins,
		Assets:      backend.PublicHandler(),
		Register: server.Register(server.Deps{
			Auth:     authService,
			Accounts: accountsService,
			Audit:    auditService,
			Media:    mediaService,
			Content:  contentService,
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

func runMediaCleanup(ctx context.Context, svc *media.Service, logger *slog.Logger) {
	run := func() {
		cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		removed, err := svc.CleanupStaleUploads(cleanupCtx)
		if err != nil {
			logger.Warn("cleanup stale media", "error", err)
			return
		}
		if removed > 0 {
			logger.Info("cleanup stale media", "removed", removed)
		}
	}
	run()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
