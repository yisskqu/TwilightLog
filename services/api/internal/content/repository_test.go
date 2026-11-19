package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yisskqu/TwilightLog/services/api/internal/migrations"
)

func testUUID(group, n int) string {
	return fmt.Sprintf("%08d-0000-4000-8000-%012d", group, n)
}

func setupContentDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	if err := migrations.Up(ctx, url); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)

	mustExec(t, pool, `TRUNCATE TABLE users, tags, content_versions, article_columns, articles,
		photo_columns, albums, photos, content_tags, recommendations, reviews, media_assets, notifications
		RESTART IDENTITY CASCADE`)
	loadFixtures(t, pool)
	return pool
}

func loadFixtures(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	owner := testUUID(9, 1)
	mustExec(t, pool, `INSERT INTO users (id, username, email, password_hash, role_id)
		VALUES ($1, 'content_test', 'content@test.local', 'x',
		(SELECT id FROM roles WHERE code = 'admin'))`, owner)

	readyMedia := testUUID(5, 1)
	processingMedia := testUUID(5, 2)
	mustExec(t, pool, `INSERT INTO media_assets (id, owner_id, purpose, storage_key, original_filename, mime_type, status)
		VALUES ($1, $2, 'photo', 'private/test/ready.png', 'ready.png', 'image/png', 'ready')`, readyMedia, owner)
	mustExec(t, pool, `INSERT INTO media_assets (id, owner_id, purpose, storage_key, original_filename, mime_type, status)
		VALUES ($1, $2, 'photo', 'private/test/processing.png', 'processing.png', 'image/png', 'processing')`, processingMedia, owner)

	columnID := testUUID(1, 1)
	mustExec(t, pool, `INSERT INTO article_columns (id, slug, status, created_by, created_at)
		VALUES ($1, 'col-pub', 'published', $2, '2026-09-01')`, columnID, owner)
	columnVersion := insertVersion(t, pool, EntityArticleColumn, columnID, 1, "approved",
		map[string]any{"name": "公开专栏", "description": "公开专栏简介"}, timePtr("2026-09-01"))
	mustExec(t, pool, `UPDATE article_columns SET published_version_id = $2 WHERE id = $1`, columnID, columnVersion)

	draftColumnID := testUUID(1, 2)
	mustExec(t, pool, `INSERT INTO article_columns (id, slug, status, created_by)
		VALUES ($1, 'col-draft', 'draft', $2)`, draftColumnID, owner)

	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 13; i++ {
		id := testUUID(2, i)
		publishedAt := base.AddDate(0, 0, i)
		mustExec(t, pool, `INSERT INTO articles (id, column_id, slug, status, created_by, created_at)
			VALUES ($1, $2, $3, 'published', $4, $5)`,
			id, columnID, fmt.Sprintf("article-%02d", i), owner, publishedAt)
		body := fmt.Sprintf("第 %02d 篇正文", i)
		index := body
		if i == 1 {
			index = "潮汐与礁石的长曝光笔记"
			body += "\n\n```go\nSECRET_CODE_XYZ\n```\n"
		}
		versionID := insertVersion(t, pool, EntityArticle, id, 1, "approved", map[string]any{
			"title":        fmt.Sprintf("文章 %02d", i),
			"summary":      fmt.Sprintf("摘要 %02d", i),
			"bodyMarkdown": body,
			"bodyIndex":    index,
		}, &publishedAt)
		mustExec(t, pool, `UPDATE articles SET published_version_id = $2 WHERE id = $1`, id, versionID)

		switch i {
		case 1:
			attachTag(t, pool, ScopeArticle, "摄影", versionID)
			attachTag(t, pool, ScopeArticle, "旅记", versionID)
		case 2:
			attachTag(t, pool, ScopeArticle, "摄影", versionID)
		case 3:
			attachTag(t, pool, ScopeArticle, "旅记", versionID)
		case 4:
			attachTag(t, pool, ScopeArticle, "城市", versionID)
		}
	}

	// A newer pending version must not replace the published snapshot.
	insertVersion(t, pool, EntityArticle, testUUID(2, 1), 2, "pending", map[string]any{
		"title": "文章 01（待审）", "summary": "待审摘要", "bodyMarkdown": "待审正文", "bodyIndex": "待审正文",
	}, nil)

	hiddenArticles := []struct {
		id, slug, status, columnID string
		deleted                    bool
	}{
		{testUUID(2, 20), "article-draft", "draft", columnID, false},
		{testUUID(2, 21), "article-offline", "offline", columnID, false},
		{testUUID(2, 22), "article-trashed", "trashed", columnID, true},
		{testUUID(2, 23), "article-unpublished-column", "published", draftColumnID, false},
	}
	for _, item := range hiddenArticles {
		mustExec(t, pool, `INSERT INTO articles (id, column_id, slug, status, deleted_at, created_by)
			VALUES ($1, $2, $3, $4, CASE WHEN $5 THEN now() END, $6)`,
			item.id, item.columnID, item.slug, item.status, item.deleted, owner)
		versionID := insertVersion(t, pool, EntityArticle, item.id, 1, "approved", map[string]any{
			"title": item.slug, "summary": "", "bodyMarkdown": "", "bodyIndex": "",
		}, timePtr("2026-10-01"))
		mustExec(t, pool, `UPDATE articles SET published_version_id = $2 WHERE id = $1`, item.id, versionID)
	}

	photoColumnID := testUUID(3, 1)
	mustExec(t, pool, `INSERT INTO photo_columns (id, slug, status, created_by)
		VALUES ($1, 'photo-col', 'published', $2)`, photoColumnID, owner)
	photoColumnVersion := insertVersion(t, pool, EntityPhotoColumn, photoColumnID, 1, "approved",
		map[string]any{"name": "公开摄影专栏", "description": "摄影", "theme": "街拍"}, timePtr("2026-09-01"))
	mustExec(t, pool, `UPDATE photo_columns SET published_version_id = $2 WHERE id = $1`,
		photoColumnID, photoColumnVersion)

	draftPhotoColumnID := testUUID(3, 2)
	mustExec(t, pool, `INSERT INTO photo_columns (id, slug, status, created_by)
		VALUES ($1, 'photo-col-draft', 'draft', $2)`, draftPhotoColumnID, owner)

	albumID := testUUID(4, 1)
	mustExec(t, pool, `INSERT INTO albums (id, column_id, slug, status, created_by)
		VALUES ($1, $2, 'album-1', 'published', $3)`, albumID, photoColumnID, owner)
	albumVersion := insertVersion(t, pool, EntityAlbum, albumID, 1, "approved", map[string]any{
		"title": "雨夜相册", "description": "雨夜", "location": "上海", "shotAt": "2026-10-10",
	}, timePtr("2026-10-10"))
	mustExec(t, pool, `UPDATE albums SET published_version_id = $2 WHERE id = $1`, albumID, albumVersion)

	emptyAlbumID := testUUID(4, 2)
	mustExec(t, pool, `INSERT INTO albums (id, column_id, slug, status, created_by)
		VALUES ($1, $2, 'album-empty', 'published', $3)`, emptyAlbumID, photoColumnID, owner)
	emptyVersion := insertVersion(t, pool, EntityAlbum, emptyAlbumID, 1, "approved", map[string]any{
		"title": "空相册", "description": "", "location": "", "shotAt": "",
	}, timePtr("2026-10-09"))
	mustExec(t, pool, `UPDATE albums SET published_version_id = $2 WHERE id = $1`, emptyAlbumID, emptyVersion)

	draftAlbumID := testUUID(4, 3)
	mustExec(t, pool, `INSERT INTO albums (id, column_id, slug, status, created_by)
		VALUES ($1, $2, 'album-draft-column', 'published', $3)`, draftAlbumID, draftPhotoColumnID, owner)
	draftAlbumVersion := insertVersion(t, pool, EntityAlbum, draftAlbumID, 1, "approved", map[string]any{
		"title": "不可见相册", "description": "", "location": "", "shotAt": "",
	}, timePtr("2026-10-08"))
	mustExec(t, pool, `UPDATE albums SET published_version_id = $2 WHERE id = $1`, draftAlbumID, draftAlbumVersion)

	insertPhoto(t, pool, testUUID(5, 10), albumID, readyMedia, owner, "published", 1)
	insertPhoto(t, pool, testUUID(5, 11), albumID, processingMedia, owner, "published", 2)
	insertPhoto(t, pool, testUUID(5, 12), albumID, readyMedia, owner, "draft", 3)

	var photoVersion string
	if err := pool.QueryRow(ctx, `SELECT published_version_id::text FROM photos WHERE id = $1`,
		testUUID(5, 10)).Scan(&photoVersion); err != nil {
		t.Fatalf("read photo version: %v", err)
	}
	attachTag(t, pool, ScopePhoto, "城市", photoVersion)
}

func insertPhoto(t *testing.T, pool *pgxpool.Pool, id, albumID, mediaID, owner, status string, order int) {
	t.Helper()
	mustExec(t, pool, `INSERT INTO photos (id, album_id, status, sort_order, created_by)
		VALUES ($1, $2, $3, $4, $5)`, id, albumID, status, order, owner)
	versionID := insertVersion(t, pool, EntityPhoto, id, 1, "approved", map[string]any{
		"mediaId": mediaID, "title": "照片 01", "description": "城市夜色",
		"location": "上海", "takenAt": "2026-10-10T20:00:00+08:00",
	}, timePtr("2026-10-11"))
	if status == "published" {
		mustExec(t, pool, `UPDATE photos SET published_version_id = $2 WHERE id = $1`, id, versionID)
	}
}

func insertVersion(t *testing.T, pool *pgxpool.Pool, entityType, entityID string, no int, status string,
	payload map[string]any, publishedAt *time.Time) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	var id string
	err = pool.QueryRow(context.Background(), `
		INSERT INTO content_versions (entity_type, entity_id, version_no, status, payload, published_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)
		RETURNING id::text`, entityType, entityID, no, status, string(raw), publishedAt).Scan(&id)
	if err != nil {
		t.Fatalf("insert version: %v", err)
	}
	return id
}

func attachTag(t *testing.T, pool *pgxpool.Pool, scope, name, versionID string) {
	t.Helper()
	var tagID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO tags (scope, name) VALUES ($1, $2)
		ON CONFLICT (scope, name) DO UPDATE SET active = true
		RETURNING id::text`, scope, name).Scan(&tagID)
	if err != nil {
		t.Fatalf("insert tag: %v", err)
	}
	mustExec(t, pool, `INSERT INTO content_tags (content_version_id, tag_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, versionID, tagID)
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func timePtr(value string) *time.Time {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		panic(err)
	}
	return &parsed
}

func TestPublicContentQueries(t *testing.T) {
	pool := setupContentDB(t)
	svc := NewService(NewRepository(pool))
	ctx := context.Background()

	t.Run("pagination is stable", func(t *testing.T) {
		first, err := svc.ListArticles(ctx, "col-pub", Query{Page: 1, PageSize: 12})
		if err != nil {
			t.Fatalf("page 1: %v", err)
		}
		if first.Total != 13 || len(first.Items) != 12 || first.TotalPages != 2 {
			t.Fatalf("unexpected page 1: total=%d items=%d pages=%d",
				first.Total, len(first.Items), first.TotalPages)
		}
		second, err := svc.ListArticles(ctx, "col-pub", Query{Page: 2, PageSize: 12})
		if err != nil {
			t.Fatalf("page 2: %v", err)
		}
		if len(second.Items) != 1 {
			t.Fatalf("page 2 items = %d", len(second.Items))
		}
		seen := map[string]bool{}
		for _, item := range append(first.Items, second.Items...) {
			if seen[item.ID] {
				t.Fatalf("duplicate article across pages: %s", item.ID)
			}
			seen[item.ID] = true
		}
		if len(seen) != 13 {
			t.Fatalf("unique articles = %d", len(seen))
		}
	})

	t.Run("sort oldest first", func(t *testing.T) {
		page, err := svc.ListArticles(ctx, "col-pub", Query{Page: 1, PageSize: 1, Sort: SortOldest})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if page.Items[0].Title != "文章 01" {
			t.Fatalf("oldest title = %s", page.Items[0].Title)
		}
		latest, err := svc.ListArticles(ctx, "col-pub", Query{Page: 1, PageSize: 1})
		if err != nil {
			t.Fatalf("list latest: %v", err)
		}
		if latest.Items[0].Title != "文章 13" {
			t.Fatalf("latest title = %s", latest.Items[0].Title)
		}
	})

	t.Run("date range filter", func(t *testing.T) {
		page, err := svc.ListArticles(ctx, "col-pub", Query{
			Page: 1, PageSize: 12, DateFrom: "2026-10-05", DateTo: "2026-10-07",
		})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if page.Total != 3 {
			t.Fatalf("date filtered total = %d, want 3", page.Total)
		}
	})

	t.Run("tag AND filter", func(t *testing.T) {
		page, err := svc.ListArticles(ctx, "col-pub", Query{
			Page: 1, PageSize: 12, Tags: []string{"摄影", "旅记"},
		})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if page.Total != 1 || page.Items[0].Slug != "article-01" {
			t.Fatalf("tag AND result = %+v", page.Items)
		}
	})

	t.Run("search hits title summary body and skips code", func(t *testing.T) {
		hit, err := svc.ListArticles(ctx, "col-pub", Query{Page: 1, PageSize: 12, Search: "潮汐"})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if hit.Total != 1 || hit.Items[0].Slug != "article-01" {
			t.Fatalf("body search = %+v", hit.Items)
		}
		summary, err := svc.ListArticles(ctx, "col-pub", Query{Page: 1, PageSize: 12, Search: "摘要 03"})
		if err != nil {
			t.Fatalf("search summary: %v", err)
		}
		if summary.Total != 1 {
			t.Fatalf("summary search total = %d", summary.Total)
		}
		code, err := svc.ListArticles(ctx, "col-pub", Query{Page: 1, PageSize: 12, Search: "SECRET_CODE_XYZ"})
		if err != nil {
			t.Fatalf("search code: %v", err)
		}
		if code.Total != 0 {
			t.Fatalf("code block leaked into index: %d", code.Total)
		}
	})

	t.Run("version isolation and hidden states", func(t *testing.T) {
		article, err := svc.GetArticle(ctx, "article-01")
		if err != nil {
			t.Fatalf("get published article: %v", err)
		}
		if article.Title != "文章 01" {
			t.Fatalf("pending version leaked: %s", article.Title)
		}
		for _, slug := range []string{"article-draft", "article-offline", "article-trashed", "article-unpublished-column"} {
			if _, err := svc.GetArticle(ctx, slug); !errors.Is(err, ErrNotFound) {
				t.Fatalf("%s should be hidden, got %v", slug, err)
			}
		}
	})

	t.Run("tag scopes stay isolated", func(t *testing.T) {
		articles, err := svc.ListArticles(ctx, "col-pub", Query{Page: 1, PageSize: 12, Tags: []string{"城市"}})
		if err != nil {
			t.Fatalf("article tag filter: %v", err)
		}
		if articles.Total != 1 || articles.Items[0].Slug != "article-04" {
			t.Fatalf("article tag result = %+v", articles.Items)
		}
		photos, err := svc.ListPhotos(ctx, "", "", Query{Page: 1, PageSize: 12, Tags: []string{"城市"}})
		if err != nil {
			t.Fatalf("photo tag filter: %v", err)
		}
		if photos.Total != 1 {
			t.Fatalf("photo tag result total = %d", photos.Total)
		}
	})

	t.Run("albums need a public photo", func(t *testing.T) {
		albums, err := svc.ListAlbums(ctx, "photo-col", Query{Page: 1, PageSize: 12})
		if err != nil {
			t.Fatalf("list albums: %v", err)
		}
		if albums.Total != 1 || albums.Items[0].Slug != "album-1" {
			t.Fatalf("album list = %+v", albums.Items)
		}
		if _, err := svc.GetAlbum(ctx, "album-empty"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("empty album should be hidden, got %v", err)
		}
		if _, err := svc.GetAlbum(ctx, "album-draft-column"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("album under draft column should be hidden, got %v", err)
		}
	})

	t.Run("photos require ready derivatives", func(t *testing.T) {
		photos, err := svc.ListPhotos(ctx, "album-1", "", Query{Page: 1, PageSize: 12})
		if err != nil {
			t.Fatalf("list photos: %v", err)
		}
		if photos.Total != 1 {
			t.Fatalf("visible photos = %d, want 1", photos.Total)
		}
	})

	t.Run("search type filter", func(t *testing.T) {
		hits, err := svc.Search(ctx, Query{Page: 1, PageSize: 12, Search: "城市"}, []string{EntityArticle})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		for _, hit := range hits.Items {
			if hit.Type != EntityArticle {
				t.Fatalf("unexpected type %s", hit.Type)
			}
		}
		if hits.Total != 1 {
			t.Fatalf("article-only search total = %d", hits.Total)
		}
	})
}
