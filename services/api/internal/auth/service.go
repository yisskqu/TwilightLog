package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yisskqu/TwilightLog/services/api/internal/audit"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/config"
)

const RefreshCookieName = "twilightlog_refresh"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrRateLimited        = errors.New("login rate limited")
	ErrInvalidSession     = errors.New("invalid session")
)

type User struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	Permissions []string  `json:"permissions"`
}

type LoginResult struct {
	AccessToken      string
	ExpiresIn        int64
	RefreshToken     string
	RefreshExpiresAt time.Time
	Remember         bool
	User             User
}

type Service struct {
	pool      *pgxpool.Pool
	cfg       config.Config
	logger    *slog.Logger
	audit     *audit.Service
	limiter   *loginLimiter
	dummyHash string
}

func NewService(pool *pgxpool.Pool, cfg config.Config, logger *slog.Logger, auditService *audit.Service) (*Service, error) {
	if len(cfg.JWTSecret) < 32 {
		return nil, errors.New("JWT_SECRET must be at least 32 characters")
	}
	dummyHash, err := HashPassword("not-a-real-password")
	if err != nil {
		return nil, fmt.Errorf("create dummy password hash: %w", err)
	}
	return &Service{
		pool:      pool,
		cfg:       cfg,
		logger:    logger,
		audit:     auditService,
		limiter:   newLoginLimiter(5, 15*time.Minute),
		dummyHash: dummyHash,
	}, nil
}

func (s *Service) Login(ctx context.Context, username, password string, remember bool, ip, userAgent string) (LoginResult, error) {
	username = strings.TrimSpace(username)
	keys := loginKeys(ip, username)
	if !s.limiter.allow(keys...) {
		return LoginResult{}, ErrRateLimited
	}

	var user User
	var passwordHash string
	err := s.pool.QueryRow(ctx, `
		SELECT u.id::text, u.username, u.email, u.password_hash, u.status, r.code, u.created_at
		FROM users u
		JOIN roles r ON r.id = u.role_id
		WHERE lower(u.username) = lower($1)
	`, username).Scan(&user.ID, &user.Username, &user.Email, &passwordHash, &user.Status, &user.Role, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = VerifyPassword(password, s.dummyHash)
		s.limiter.fail(keys...)
		s.audit.Log(ctx, nil, "auth.login", "user", "", "failure", ip, userAgent, map[string]any{"username": username})
		return LoginResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return LoginResult{}, fmt.Errorf("find user: %w", err)
	}

	if !VerifyPassword(password, passwordHash) || user.Status != "active" {
		s.limiter.fail(keys...)
		s.audit.Log(ctx, &user.ID, "auth.login", "user", user.ID, "failure", ip, userAgent, map[string]any{"username": user.Username})
		return LoginResult{}, ErrInvalidCredentials
	}

	s.limiter.reset(keys...)

	permissions, err := s.Permissions(ctx, user.ID)
	if err != nil {
		return LoginResult{}, err
	}
	user.Permissions = permissions

	refreshToken, err := newRefreshToken()
	if err != nil {
		return LoginResult{}, err
	}
	refreshExpiresAt := time.Now().Add(s.sessionTTL(remember))
	var sessionID string
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO auth_sessions (user_id, refresh_token_hash, remember, user_agent, ip, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text
	`, user.ID, hashRefreshToken(refreshToken), remember, truncate(userAgent, 256), truncate(ip, 64), refreshExpiresAt).Scan(&sessionID); err != nil {
		return LoginResult{}, fmt.Errorf("create session: %w", err)
	}

	accessToken, err := newAccessToken(s.cfg.JWTSecret, user.ID, sessionID, user.Role, user.Username, s.cfg.AccessTokenTTL)
	if err != nil {
		return LoginResult{}, err
	}

	s.audit.Log(ctx, &user.ID, "auth.login", "user", user.ID, "success", ip, userAgent, map[string]any{"remember": remember})
	return LoginResult{
		AccessToken:      accessToken,
		ExpiresIn:        int64(s.cfg.AccessTokenTTL.Seconds()),
		RefreshToken:     refreshToken,
		RefreshExpiresAt: refreshExpiresAt,
		Remember:         remember,
		User:             user,
	}, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken, ip, userAgent string) (LoginResult, error) {
	if refreshToken == "" {
		return LoginResult{}, ErrInvalidSession
	}

	var sessionID, userID, username, email, status, role, remember string
	var expiresAt, createdAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT s.id::text, s.user_id::text, u.username, u.email, u.status, r.code, s.remember::text, s.expires_at, u.created_at
		FROM auth_sessions s
		JOIN users u ON u.id = s.user_id
		JOIN roles r ON r.id = u.role_id
		WHERE s.refresh_token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > now()
	`, hashRefreshToken(refreshToken)).Scan(&sessionID, &userID, &username, &email, &status, &role, &remember, &expiresAt, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return LoginResult{}, ErrInvalidSession
	}
	if err != nil {
		return LoginResult{}, fmt.Errorf("find session: %w", err)
	}
	if status != "active" {
		_, _ = s.pool.Exec(ctx, `UPDATE auth_sessions SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, sessionID)
		return LoginResult{}, ErrInvalidSession
	}

	permissions, err := s.Permissions(ctx, userID)
	if err != nil {
		return LoginResult{}, err
	}

	newToken, err := newRefreshToken()
	if err != nil {
		return LoginResult{}, err
	}
	rememberBool := remember == "true"
	newExpiresAt := time.Now().Add(s.sessionTTL(rememberBool))
	if _, err := s.pool.Exec(ctx, `
		UPDATE auth_sessions
		SET refresh_token_hash = $2, last_used_at = now(), expires_at = $3
		WHERE id = $1 AND revoked_at IS NULL
	`, sessionID, hashRefreshToken(newToken), newExpiresAt); err != nil {
		return LoginResult{}, fmt.Errorf("rotate refresh token: %w", err)
	}

	accessToken, err := newAccessToken(s.cfg.JWTSecret, userID, sessionID, role, username, s.cfg.AccessTokenTTL)
	if err != nil {
		return LoginResult{}, err
	}

	return LoginResult{
		AccessToken:      accessToken,
		ExpiresIn:        int64(s.cfg.AccessTokenTTL.Seconds()),
		RefreshToken:     newToken,
		RefreshExpiresAt: newExpiresAt,
		Remember:         rememberBool,
		User: User{
			ID:          userID,
			Username:    username,
			Email:       email,
			Role:        role,
			Status:      status,
			CreatedAt:   createdAt,
			Permissions: permissions,
		},
	}, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken, ip, userAgent string) {
	if refreshToken == "" {
		return
	}
	var userID string
	err := s.pool.QueryRow(ctx, `
		UPDATE auth_sessions
		SET revoked_at = now()
		WHERE refresh_token_hash = $1 AND revoked_at IS NULL
		RETURNING user_id::text
	`, hashRefreshToken(refreshToken)).Scan(&userID)
	if err == nil {
		s.audit.Log(ctx, &userID, "auth.logout", "user", userID, "success", ip, userAgent, nil)
	}
}

func (s *Service) Authenticate(ctx context.Context, accessToken string) (User, error) {
	parsed, err := parseAccessToken(s.cfg.JWTSecret, accessToken)
	if err != nil {
		return User{}, ErrInvalidSession
	}

	var user User
	err = s.pool.QueryRow(ctx, `
		SELECT u.id::text, u.username, u.email, u.status, r.code, u.created_at
		FROM auth_sessions s
		JOIN users u ON u.id = s.user_id
		JOIN roles r ON r.id = u.role_id
		WHERE s.id = $1 AND s.user_id = $2 AND s.revoked_at IS NULL AND s.expires_at > now()
	`, parsed.SessionID, parsed.Subject).Scan(&user.ID, &user.Username, &user.Email, &user.Status, &user.Role, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrInvalidSession
	}
	if err != nil {
		return User{}, fmt.Errorf("load session: %w", err)
	}
	if user.Status != "active" {
		return User{}, ErrInvalidSession
	}
	return user, nil
}

func (s *Service) Permissions(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.code
		FROM users u
		JOIN role_permissions rp ON rp.role_id = u.role_id
		JOIN permissions p ON p.code = rp.permission_code
		WHERE u.id = $1
		ORDER BY p.code
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("load permissions: %w", err)
	}
	defer rows.Close()

	permissions := []string{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		permissions = append(permissions, code)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate permissions: %w", err)
	}
	return permissions, nil
}

func (s *Service) SetRefreshCookie(w http.ResponseWriter, token string, remember bool, expiresAt time.Time) {
	cookie := &http.Cookie{
		Name:     RefreshCookieName,
		Value:    token,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Domain:   s.cfg.CookieDomain,
	}
	if remember {
		cookie.Expires = expiresAt
		cookie.MaxAge = int(time.Until(expiresAt).Seconds())
	}
	http.SetCookie(w, cookie)
}

func (s *Service) ClearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    "",
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Domain:   s.cfg.CookieDomain,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func (s *Service) sessionTTL(remember bool) time.Duration {
	if remember {
		return s.cfg.RefreshTokenTTL
	}
	return s.cfg.RefreshTokenTTLShort
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
