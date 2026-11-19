package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAuthRejectsMissingToken(t *testing.T) {
	svc := &Service{}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	res := httptest.NewRecorder()
	svc.RequireAuth(next).ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.Code)
	}
}

func TestRequireRoleAllowsMatchingRole(t *testing.T) {
	svc := &Service{}
	caller := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := svc.RequireRole(caller, "admin", "super_admin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ping", nil)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey{}, User{ID: "1", Role: "admin"}))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}
}

func TestRequireRoleRejectsEditorOnAdminRoute(t *testing.T) {
	svc := &Service{}
	caller := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be called")
	})
	handler := svc.RequireAdmin(caller)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ping", nil)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey{}, User{ID: "1", Role: "editor"}))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", res.Code)
	}
}
