package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/yisskqu/TwilightLog/services/api/internal/platform/httpx"
)

type userContextKey struct{}

func (s *Service) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			httpx.WriteError(w, r, http.StatusUnauthorized, "unauthenticated", "请先登录", nil)
			return
		}
		user, err := s.Authenticate(r.Context(), token)
		if err != nil {
			httpx.WriteError(w, r, http.StatusUnauthorized, "unauthenticated", "登录状态已失效，请重新登录", nil)
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey{}, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Service) RequireAdmin(next http.Handler) http.Handler {
	return s.RequireRole(next, "super_admin", "admin")
}

func (s *Service) RequireEditor(next http.Handler) http.Handler {
	return s.RequireRole(next, "super_admin", "admin", "editor")
}

func (s *Service) RequireRole(next http.Handler, roles ...string) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, r, http.StatusUnauthorized, "unauthenticated", "请先登录", nil)
			return
		}
		if _, ok := allowed[user.Role]; !ok {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "无权访问", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func UserFromContext(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(userContextKey{}).(User)
	return user, ok
}

func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}
