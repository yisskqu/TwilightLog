// Command admin creates accounts that must never be created from the web console.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/mail"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/yisskqu/TwilightLog/services/api/internal/auth"
	"github.com/yisskqu/TwilightLog/services/api/internal/migrations"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/config"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/db"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9_.-]{3,32}$`)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "create" {
		usage()
		os.Exit(2)
	}

	flags := flag.NewFlagSet("create", flag.ExitOnError)
	username := flags.String("username", "", "管理员用户名（3-32 位小写字母、数字、点、横线或下划线）")
	email := flags.String("email", "", "管理员邮箱")
	password := flags.String("password", os.Getenv("ADMIN_PASSWORD"), "管理员密码（至少 8 位），默认读取 ADMIN_PASSWORD")
	role := flags.String("role", "admin", "角色：admin 或 super_admin")
	if err := flags.Parse(os.Args[2:]); err != nil {
		os.Exit(2)
	}

	normalizedUsername := strings.ToLower(strings.TrimSpace(*username))
	normalizedEmail := strings.ToLower(strings.TrimSpace(*email))
	selectedRole := strings.TrimSpace(*role)

	if !usernamePattern.MatchString(normalizedUsername) {
		fail("用户名需为 3-32 位小写字母、数字、点、横线或下划线")
	}
	if !validEmail(normalizedEmail) {
		fail("邮箱格式不正确")
	}
	if !auth.IsStrongEnough(*password) {
		fail("密码至少需要 8 位，可通过 --password 或 ADMIN_PASSWORD 提供")
	}
	if selectedRole != "admin" && selectedRole != "super_admin" {
		fail("角色只能是 admin 或 super_admin")
	}

	cfg, err := config.Load()
	if err != nil {
		fail("读取配置失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fail("连接数据库失败: %v", err)
	}
	defer pool.Close()

	if err := migrations.Up(ctx, cfg.DatabaseURL); err != nil {
		fail("执行迁移失败: %v", err)
	}

	passwordHash, err := auth.HashPassword(*password)
	if err != nil {
		fail("生成密码哈希失败: %v", err)
	}

	var id string
	err = pool.QueryRow(ctx, `
		INSERT INTO users (username, email, password_hash, role_id)
		VALUES ($1, $2, $3, (SELECT id FROM roles WHERE code = $4))
		RETURNING id::text
	`, normalizedUsername, normalizedEmail, passwordHash, selectedRole).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			fail("用户名或邮箱已存在")
		}
		fail("创建管理员失败: %v", err)
	}

	fmt.Printf("已创建 %s 账号\n  id:       %s\n  username: %s\n  email:    %s\n", selectedRole, id, normalizedUsername, normalizedEmail)
}

func validEmail(email string) bool {
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email
}

func usage() {
	fmt.Fprintln(os.Stderr, "用法: go run ./cmd/admin create --username <用户名> --email <邮箱> --password <密码> [--role admin|super_admin]")
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
