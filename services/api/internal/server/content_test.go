package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yisskqu/TwilightLog/services/api/internal/content"
	"github.com/yisskqu/TwilightLog/services/api/internal/media"
	"github.com/yisskqu/TwilightLog/services/api/internal/migrations"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/httpx"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/storage"
)

func TestArticleHandlerETagAndNotFound(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	if err := migrations.Up(ctx, url); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	mustExecServer(t, pool, `TRUNCATE TABLE users, tags, content_versions, article_columns, articles,
		photo_columns, albums, photos, content_tags, recommendations, reviews, media_assets, notifications
		RESTART IDENTITY CASCADE`)
	owner := "aaaaaaaa-0000-4000-8000-000000000001"
	mustExecServer(t, pool, `INSERT INTO users (id, username, email, password_hash, role_id)
		VALUES ($1, 'contract_test', 'contract@test.local', 'x',
		(SELECT id FROM roles WHERE code = 'admin'))`, owner)
	columnID := "aaaaaaaa-0000-4000-8000-000000000002"
	mustExecServer(t, pool, `INSERT INTO article_columns (id, slug, status, created_by)
		VALUES ($1, 'contract-col', 'published', $2)`, columnID, owner)
	var columnVersion string
	err = pool.QueryRow(ctx, `INSERT INTO content_versions (entity_type, entity_id, version_no, status, payload, published_at)
		VALUES ('article_column', $1, 1, 'approved', '{"name":"契约专栏"}'::jsonb, now()) RETURNING id::text`,
		columnID).Scan(&columnVersion)
	if err != nil {
		t.Fatalf("insert column version: %v", err)
	}
	mustExecServer(t, pool, `UPDATE article_columns SET published_version_id = $2 WHERE id = $1`,
		columnID, columnVersion)

	articleID := "aaaaaaaa-0000-4000-8000-000000000003"
	mustExecServer(t, pool, `INSERT INTO articles (id, column_id, slug, status, created_by)
		VALUES ($1, $2, 'contract-article', 'published', $3)`, articleID, columnID, owner)
	var articleVersion string
	err = pool.QueryRow(ctx, `INSERT INTO content_versions (entity_type, entity_id, version_no, status, payload, published_at)
		VALUES ('article', $1, 1, 'approved',
		'{"title":"契约文章","summary":"摘要","bodyMarkdown":"正文","bodyIndex":"正文"}'::jsonb, now())
		RETURNING id::text`, articleID).Scan(&articleVersion)
	if err != nil {
		t.Fatalf("insert article version: %v", err)
	}
	mustExecServer(t, pool, `UPDATE articles SET published_version_id = $2 WHERE id = $1`,
		articleID, articleVersion)

	backend, err := storage.NewLocal(t.TempDir(), "http://localhost:8080/assets", "http://localhost:8080", "test-secret-0123456789")
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	mediaSvc := media.NewService(media.NewRepository(pool), backend, nil, media.ServiceConfig{}, nil)
	contentSvc := content.NewService(content.NewRepository(pool))
	handler := httpx.RequestID(getArticleHandler(contentSvc, mediaSvc))

	router := chi.NewRouter()
	router.Handle("/api/v1/articles/{slug}", handler)

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/v1/articles/contract-article", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", first.Code, first.Body.String())
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/articles/contract-article", nil)
	request.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	router.ServeHTTP(second, request)
	if second.Code != http.StatusNotModified {
		t.Fatalf("If-None-Match status = %d, want 304", second.Code)
	}

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/v1/articles/missing-slug", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d", missing.Code)
	}
	var payload struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(missing.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if payload.Code != "not_found" || !strings.Contains(payload.Message, "内容不存在") || payload.RequestID == "" {
		t.Fatalf("unexpected error payload: %+v", payload)
	}
}

func mustExecServer(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}
