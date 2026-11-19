package server

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/yisskqu/TwilightLog/services/api/internal/accounts"
	"github.com/yisskqu/TwilightLog/services/api/internal/audit"
	"github.com/yisskqu/TwilightLog/services/api/internal/auth"
	"github.com/yisskqu/TwilightLog/services/api/internal/content"
	"github.com/yisskqu/TwilightLog/services/api/internal/media"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/httpx"
)

// Deps carries the services used by the HTTP layer.
type Deps struct {
	Auth     *auth.Service
	Accounts *accounts.Service
	Audit    *audit.Service
	Media    *media.Service
	Content  *content.Service
}

type sessionResponse struct {
	AccessToken string    `json:"accessToken"`
	ExpiresIn   int64     `json:"expiresIn"`
	User        auth.User `json:"user"`
}

// Register mounts authentication, account management and audit routes under /api/v1.
func Register(deps Deps) func(chi.Router) {
	return func(r chi.Router) {
		r.Route("/home", func(r chi.Router) {
			r.Get("/recommendations", recommendationsHandler(deps.Content, deps.Media))
		})
		r.Get("/article-columns", listArticleColumnsHandler(deps.Content, deps.Media))
		r.Get("/article-columns/{slug}/articles", listColumnArticlesHandler(deps.Content, deps.Media))
		r.Get("/articles/{slug}", getArticleHandler(deps.Content, deps.Media))
		r.Get("/photo-columns", listPhotoColumnsHandler(deps.Content, deps.Media))
		r.Get("/photo-columns/{slug}/albums", listColumnAlbumsHandler(deps.Content, deps.Media))
		r.Get("/albums/{slug}", getAlbumHandler(deps.Content, deps.Media))
		r.Get("/albums/{slug}/photos", listAlbumPhotosHandler(deps.Content, deps.Media))
		r.Get("/photos", listPhotosHandler(deps.Content, deps.Media))
		r.Get("/photos/{id}", getPhotoHandler(deps.Content, deps.Media))
		r.Get("/search", searchHandler(deps.Content, deps.Media))
		r.Get("/tags", listTagsHandler(deps.Content))

		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", loginHandler(deps.Auth))
			r.Post("/refresh", refreshHandler(deps.Auth))
			r.Post("/logout", logoutHandler(deps.Auth))
			r.With(deps.Auth.RequireAuth).Get("/me", meHandler(deps.Auth))
		})

		r.Route("/admin", func(r chi.Router) {
			r.Use(deps.Auth.RequireAuth, deps.Auth.RequireAdmin)
			r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
				httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "scope": "admin"})
			})
			r.Get("/users", listUsersHandler(deps.Accounts))
			r.Post("/users", createUserHandler(deps.Accounts))
			r.Patch("/users/{id}/role", setRoleHandler(deps.Accounts))
			r.Post("/users/{id}/reset-password", resetPasswordHandler(deps.Accounts))
			r.Patch("/users/{id}/status", setStatusHandler(deps.Accounts))
			r.Get("/audit", listAuditHandler(deps.Audit))
		})

		r.With(deps.Auth.RequireAuth, deps.Auth.RequireEditor).Get("/editor/ping", func(w http.ResponseWriter, r *http.Request) {
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "scope": "editor"})
		})

		r.Route("/media", func(r chi.Router) {
			// Capability URLs are authenticated by their signed token, not by a session.
			r.Put("/upload/{id}", uploadHandler(deps.Media))
			r.Put("/upload/{id}/parts/{part}", uploadHandler(deps.Media))

			r.Group(func(r chi.Router) {
				r.Use(deps.Auth.RequireAuth, deps.Auth.RequireEditor)
				r.Post("/presign", presignHandler(deps.Media))
				r.Post("/complete", completeMediaHandler(deps.Media))
				r.Get("/{id}", getMediaHandler(deps.Media))
				r.Post("/{id}/retry", retryMediaHandler(deps.Media))
				r.Get("/{id}/original", originalMediaHandler(deps.Media))
			})
		})

		r.Route("/assets", func(r chi.Router) {
			r.Get("/signed", signedAssetHandler(deps.Media))
			r.Put("/upload", signedPutHandler(deps.Media))
		})
	}
}

func loginHandler(svc *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Remember bool   `json:"remember"`
		}
		if !decodeJSON(w, r, &payload) {
			return
		}
		result, err := svc.Login(r.Context(), payload.Username, payload.Password, payload.Remember, clientIP(r), r.UserAgent())
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		svc.SetRefreshCookie(w, result.RefreshToken, result.Remember, result.RefreshExpiresAt)
		httpx.WriteJSON(w, http.StatusOK, sessionResponse{
			AccessToken: result.AccessToken,
			ExpiresIn:   result.ExpiresIn,
			User:        result.User,
		})
	}
}

func refreshHandler(svc *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(auth.RefreshCookieName)
		if err != nil {
			httpx.WriteError(w, r, http.StatusUnauthorized, "unauthenticated", "登录状态已失效，请重新登录", nil)
			return
		}
		result, err := svc.Refresh(r.Context(), cookie.Value, clientIP(r), r.UserAgent())
		if err != nil {
			svc.ClearRefreshCookie(w)
			writeServiceError(w, r, err)
			return
		}
		svc.SetRefreshCookie(w, result.RefreshToken, result.Remember, result.RefreshExpiresAt)
		httpx.WriteJSON(w, http.StatusOK, sessionResponse{
			AccessToken: result.AccessToken,
			ExpiresIn:   result.ExpiresIn,
			User:        result.User,
		})
	}
}

func logoutHandler(svc *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(auth.RefreshCookieName); err == nil {
			svc.Logout(r.Context(), cookie.Value, clientIP(r), r.UserAgent())
		}
		svc.ClearRefreshCookie(w)
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func meHandler(svc *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.UserFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, r, http.StatusUnauthorized, "unauthenticated", "请先登录", nil)
			return
		}
		permissions, err := svc.Permissions(r.Context(), user.ID)
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		user.Permissions = permissions
		httpx.WriteJSON(w, http.StatusOK, user)
	}
}

func listUsersHandler(svc *accounts.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, pageSize := pagination(r)
		result, err := svc.List(r.Context(), r.URL.Query().Get("search"), page, pageSize)
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, result)
	}
}

func createUserHandler(svc *accounts.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload accounts.CreateInput
		if !decodeJSON(w, r, &payload) {
			return
		}
		actor, _ := auth.UserFromContext(r.Context())
		account, err := svc.Create(r.Context(), actor.ID, payload, clientIP(r), r.UserAgent())
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, account)
	}
}

func setRoleHandler(svc *accounts.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Role string `json:"role"`
		}
		if !decodeJSON(w, r, &payload) {
			return
		}
		actor, _ := auth.UserFromContext(r.Context())
		if err := svc.SetRole(r.Context(), actor.ID, chi.URLParam(r, "id"), payload.Role, clientIP(r), r.UserAgent()); err != nil {
			writeServiceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func resetPasswordHandler(svc *accounts.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Password string `json:"password"`
		}
		if !decodeJSON(w, r, &payload) {
			return
		}
		actor, _ := auth.UserFromContext(r.Context())
		if err := svc.ResetPassword(r.Context(), actor.ID, chi.URLParam(r, "id"), payload.Password, clientIP(r), r.UserAgent()); err != nil {
			writeServiceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func setStatusHandler(svc *accounts.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Status string `json:"status"`
		}
		if !decodeJSON(w, r, &payload) {
			return
		}
		actor, _ := auth.UserFromContext(r.Context())
		if err := svc.SetStatus(r.Context(), actor.ID, chi.URLParam(r, "id"), payload.Status, clientIP(r), r.UserAgent()); err != nil {
			writeServiceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func listAuditHandler(svc *audit.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, pageSize := pagination(r)
		result, err := svc.List(r.Context(), page, pageSize)
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, result)
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_json", "请求体格式不正确", nil)
		return false
	}
	return true
}

func pagination(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	return page, pageSize
}

func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		httpx.WriteError(w, r, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误", nil)
	case errors.Is(err, auth.ErrRateLimited):
		httpx.WriteError(w, r, http.StatusTooManyRequests, "rate_limited", "尝试次数过多，请稍后再试", nil)
	case errors.Is(err, auth.ErrInvalidSession):
		httpx.WriteError(w, r, http.StatusUnauthorized, "unauthenticated", "登录状态已失效，请重新登录", nil)
	case errors.Is(err, accounts.ErrInvalidInput):
		httpx.WriteError(w, r, http.StatusBadRequest, "validation_error", validationMessage(err), nil)
	case errors.Is(err, accounts.ErrConflict):
		httpx.WriteError(w, r, http.StatusConflict, "conflict", "用户名或邮箱已存在", nil)
	case errors.Is(err, accounts.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "账号不存在", nil)
	case errors.Is(err, accounts.ErrAdminProtected):
		httpx.WriteError(w, r, http.StatusForbidden, "admin_protected", "管理员账号只能在服务器上操作", nil)
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "服务器内部错误", nil)
	}
}

func validationMessage(err error) string {
	message := strings.TrimPrefix(err.Error(), accounts.ErrInvalidInput.Error()+": ")
	if message == err.Error() {
		return "请求参数不合法"
	}
	return message
}

// clientIP returns the client address as seen through the reverse proxy.
func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
