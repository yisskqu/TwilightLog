// Command seed loads repeatable demo content for the public site.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yisskqu/TwilightLog/services/api/internal/migrations"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/config"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/db"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/storage"
	"github.com/yisskqu/TwilightLog/services/api/internal/seed"
)

func main() {
	owner := flag.String("owner", "", "内容归属用户名；默认取第一个管理员账号")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	cfg, err := config.Load()
	if err != nil {
		fail("读取配置失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fail("连接数据库失败: %v", err)
	}
	defer pool.Close()

	if err := migrations.Up(ctx, cfg.DatabaseURL); err != nil {
		fail("执行迁移失败: %v", err)
	}

	ownerID, err := resolveOwner(ctx, pool, *owner)
	if err != nil {
		fail("%v", err)
	}

	backend, err := storage.NewLocal(cfg.LocalStorageRoot, cfg.AssetBaseURL, cfg.APIBaseURL, cfg.MediaSigningSecret)
	if err != nil {
		fail("初始化存储失败: %v", err)
	}

	stats, err := seed.Run(ctx, pool, backend, ownerID, logger)
	if err != nil {
		fail("导入种子数据失败: %v", err)
	}

	fmt.Printf("种子数据已就绪（归属用户 %s）\n", ownerID)
	fmt.Printf("  媒体：%d\n  文章专栏：%d\n  文章：%d\n  摄影专栏：%d\n  相册：%d\n  照片：%d\n",
		stats.Media, stats.ArticleColumns, stats.Articles,
		stats.PhotoColumns, stats.Albums, stats.Photos)
}

func resolveOwner(ctx context.Context, pool *pgxpool.Pool, username string) (string, error) {
	var id string
	var err error
	if username != "" {
		err = pool.QueryRow(ctx,
			`SELECT id::text FROM users WHERE lower(username) = lower($1)`, username).Scan(&id)
	} else {
		err = pool.QueryRow(ctx, `
			SELECT u.id::text FROM users u
			JOIN roles r ON r.id = u.role_id
			WHERE r.code IN ('super_admin', 'admin')
			ORDER BY u.created_at ASC LIMIT 1`).Scan(&id)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		if username != "" {
			return "", fmt.Errorf("账号 %s 不存在", username)
		}
		return "", errors.New("没有找到管理员账号，请先运行 cmd/admin 创建管理员")
	}
	if err != nil {
		return "", fmt.Errorf("查询账号失败: %w", err)
	}
	return id, nil
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
