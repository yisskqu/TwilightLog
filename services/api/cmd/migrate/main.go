package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/yisskqu/TwilightLog/services/api/internal/migrations"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: migrate <up|down|status>")
		os.Exit(1)
	}

	ctx := context.Background()
	switch os.Args[1] {
	case "up":
		err = migrations.Up(ctx, cfg.DatabaseURL)
	case "down":
		err = migrations.Down(ctx, cfg.DatabaseURL)
	case "status":
		err = migrations.Status(ctx, cfg.DatabaseURL)
	default:
		fmt.Fprintf(os.Stderr, "unknown migration command: %s\n", os.Args[1])
		os.Exit(1)
	}

	if err != nil {
		logger.Error("migration command failed", "command", os.Args[1], "error", err)
		os.Exit(1)
	}
}
