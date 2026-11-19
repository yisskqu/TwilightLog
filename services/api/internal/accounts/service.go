package accounts

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yisskqu/TwilightLog/services/api/internal/audit"
	"github.com/yisskqu/TwilightLog/services/api/internal/auth"
)

var (
	ErrNotFound       = errors.New("account not found")
	ErrAdminProtected = errors.New("admin account is protected")
	ErrConflict       = errors.New("username or email already exists")
	ErrInvalidInput   = errors.New("invalid account input")
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9_.-]{3,32}$`)

type Account struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CanManage bool      `json:"canManage"`
	CreatedAt time.Time `json:"createdAt"`
}

type Page struct {
	Items    []Account `json:"items"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
	Total    int64     `json:"total"`
}

type CreateInput struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type Service struct {
	pool  *pgxpool.Pool
	audit *audit.Service
}

func NewService(pool *pgxpool.Pool, auditService *audit.Service) *Service {
	return &Service{pool: pool, audit: auditService}
}

func (s *Service) List(ctx context.Context, search string, page, pageSize int) (Page, error) {
	page, pageSize = normalizePage(page, pageSize)
	search = strings.TrimSpace(search)
	pattern := "%" + search + "%"

	var total int64
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM users u
		WHERE $1 = '' OR u.username ILIKE $2 OR u.email ILIKE $2
	`, search, pattern).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("count accounts: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT u.id::text, u.username, u.email, r.code, u.status, u.created_at,
		       r.code IN ('user', 'editor') AS can_manage
		FROM users u
		JOIN roles r ON r.id = u.role_id
		WHERE $1 = '' OR u.username ILIKE $2 OR u.email ILIKE $2
		ORDER BY u.created_at DESC, u.id
		LIMIT $3 OFFSET $4
	`, search, pattern, pageSize, (page-1)*pageSize)
	if err != nil {
		return Page{}, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()

	items := make([]Account, 0, pageSize)
	for rows.Next() {
		var item Account
		if err := rows.Scan(&item.ID, &item.Username, &item.Email, &item.Role, &item.Status, &item.CreatedAt, &item.CanManage); err != nil {
			return Page{}, fmt.Errorf("scan account: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("iterate accounts: %w", err)
	}

	return Page{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *Service) Create(ctx context.Context, actorID string, input CreateInput, ip, userAgent string) (Account, error) {
	username := strings.ToLower(strings.TrimSpace(input.Username))
	email := strings.ToLower(strings.TrimSpace(input.Email))
	role := strings.TrimSpace(input.Role)
	if role == "" {
		role = "user"
	}
	if !usernamePattern.MatchString(username) {
		return Account{}, fmt.Errorf("%w: username must be 3-32 lowercase letters, numbers, dot, dash or underscore", ErrInvalidInput)
	}
	if !validEmail(email) {
		return Account{}, fmt.Errorf("%w: invalid email", ErrInvalidInput)
	}
	if !auth.IsStrongEnough(input.Password) {
		return Account{}, fmt.Errorf("%w: password must be at least 8 characters", ErrInvalidInput)
	}
	if role != "user" && role != "editor" {
		return Account{}, fmt.Errorf("%w: role must be user or editor", ErrInvalidInput)
	}

	passwordHash, err := auth.HashPassword(input.Password)
	if err != nil {
		return Account{}, err
	}

	var account Account
	err = s.pool.QueryRow(ctx, `
		INSERT INTO users (username, email, password_hash, role_id)
		VALUES ($1, $2, $3, (SELECT id FROM roles WHERE code = $4))
		RETURNING id::text, username, email, $4, status, created_at, true
	`, username, email, passwordHash, role).Scan(
		&account.ID, &account.Username, &account.Email, &account.Role, &account.Status, &account.CreatedAt, &account.CanManage,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Account{}, ErrConflict
		}
		return Account{}, fmt.Errorf("create account: %w", err)
	}

	s.audit.Log(ctx, &actorID, "account.create", "user", account.ID, "success", ip, userAgent, map[string]any{"role": role})
	return account, nil
}

func (s *Service) SetRole(ctx context.Context, actorID, targetID, role, ip, userAgent string) error {
	if role != "user" && role != "editor" {
		return fmt.Errorf("%w: role must be user or editor", ErrInvalidInput)
	}
	targetRole, err := s.targetRole(ctx, targetID)
	if err != nil {
		return err
	}
	if isAdminRole(targetRole) {
		return ErrAdminProtected
	}

	if _, err := s.pool.Exec(ctx, `
		UPDATE users
		SET role_id = (SELECT id FROM roles WHERE code = $2), updated_at = now()
		WHERE id = $1
	`, targetID, role); err != nil {
		return fmt.Errorf("update role: %w", err)
	}

	action := "account.grant_editor"
	if role == "user" {
		action = "account.revoke_editor"
	}
	s.audit.Log(ctx, &actorID, action, "user", targetID, "success", ip, userAgent, nil)
	return nil
}

func (s *Service) ResetPassword(ctx context.Context, actorID, targetID, password, ip, userAgent string) error {
	if !auth.IsStrongEnough(password) {
		return fmt.Errorf("%w: password must be at least 8 characters", ErrInvalidInput)
	}
	targetRole, err := s.targetRole(ctx, targetID)
	if err != nil {
		return err
	}
	if isAdminRole(targetRole) {
		return ErrAdminProtected
	}

	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1
	`, targetID, passwordHash); err != nil {
		return fmt.Errorf("reset password: %w", err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE auth_sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, targetID); err != nil {
		return fmt.Errorf("revoke sessions after password reset: %w", err)
	}

	s.audit.Log(ctx, &actorID, "account.reset_password", "user", targetID, "success", ip, userAgent, nil)
	return nil
}

func (s *Service) SetStatus(ctx context.Context, actorID, targetID, status, ip, userAgent string) error {
	if status != "active" && status != "disabled" {
		return fmt.Errorf("%w: status must be active or disabled", ErrInvalidInput)
	}
	targetRole, err := s.targetRole(ctx, targetID)
	if err != nil {
		return err
	}
	if isAdminRole(targetRole) {
		return ErrAdminProtected
	}

	if _, err := s.pool.Exec(ctx, `UPDATE users SET status = $2, updated_at = now() WHERE id = $1`, targetID, status); err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	if status == "disabled" {
		if _, err := s.pool.Exec(ctx, `UPDATE auth_sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, targetID); err != nil {
			return fmt.Errorf("revoke sessions after disable: %w", err)
		}
	}

	action := "account.enable"
	if status == "disabled" {
		action = "account.disable"
	}
	s.audit.Log(ctx, &actorID, action, "user", targetID, "success", ip, userAgent, nil)
	return nil
}

func (s *Service) targetRole(ctx context.Context, targetID string) (string, error) {
	var role string
	err := s.pool.QueryRow(ctx, `
		SELECT r.code FROM users u JOIN roles r ON r.id = u.role_id WHERE u.id = $1
	`, targetID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("load target role: %w", err)
	}
	return role, nil
}

func validEmail(email string) bool {
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email
}

func isAdminRole(role string) bool {
	return role == "super_admin" || role == "admin"
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 12
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
