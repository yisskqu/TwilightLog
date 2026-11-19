// Package seed loads repeatable demo content used by the public site and tests.
package seed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yisskqu/TwilightLog/services/api/internal/content"
	"github.com/yisskqu/TwilightLog/services/api/internal/media"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/storage"
)

// Stats reports how many demo rows are present after the run.
type Stats struct {
	Media          int
	ArticleColumns int
	Articles       int
	PhotoColumns   int
	Albums         int
	Photos         int
}

type seeder struct {
	pool      *pgxpool.Pool
	backend   storage.Backend
	processor media.Processor
	ownerID   string
	stats     Stats
}

// Run upserts the demo dataset. It is safe to execute repeatedly.
func Run(ctx context.Context, pool *pgxpool.Pool, backend storage.Backend, ownerID string, logger *slog.Logger) (Stats, error) {
	if logger == nil {
		logger = slog.Default()
	}
	s := &seeder{
		pool:      pool,
		backend:   backend,
		processor: media.NewLocalProcessor(backend, logger),
		ownerID:   ownerID,
	}
	if err := s.run(ctx); err != nil {
		return Stats{}, err
	}
	return s.stats, nil
}

func (s *seeder) run(ctx context.Context) error {
	articleColumns := []struct {
		id, slug, name, description string
		tags                        []string
	}{
		{seedUUID(1, 1), "tech", "技术笔记", "工程实践、语言与工具的使用记录。", []string{"技术", "前端"}},
		{seedUUID(1, 2), "travel", "旅记", "路途中的观察、路线与见闻。", []string{"旅记", "摄影"}},
		{seedUUID(1, 3), "notes", "随笔", "日常片段与慢下来的练习。", []string{"生活"}},
	}
	for i, column := range articleColumns {
		mediaID := seedUUID(6, i+1)
		if err := s.ensureMedia(ctx, mediaID, media.PurposeArticle, i, 1200, 800); err != nil {
			return err
		}
		if err := s.upsertColumn(ctx, content.EntityArticleColumn, column.id, column.slug,
			map[string]any{"name": column.name, "description": column.description, "coverMediaId": mediaID},
			column.tags, i); err != nil {
			return err
		}
		s.stats.ArticleColumns++
	}

	articleSpecs := articleSeedSpecs()
	for i, spec := range articleSpecs {
		mediaID := ""
		if spec.cover {
			mediaID = seedUUID(6, 10+i)
			if err := s.ensureMedia(ctx, mediaID, media.PurposeArticle, 20+i, 1200, 800); err != nil {
				return err
			}
		}
		if err := s.upsertArticle(ctx, spec, mediaID, i); err != nil {
			return err
		}
		s.stats.Articles++
	}

	photoColumns := []struct {
		id, slug, name, description, theme string
		tags                               []string
	}{
		{seedUUID(3, 1), "street", "街头", "城市里的人、光与偶然。", "街拍", []string{"街拍", "人文"}},
		{seedUUID(3, 2), "landscape", "风景", "自然光线下的地貌与季节。", "自然风光", []string{"自然风光"}},
		{seedUUID(3, 3), "cityscape", "城市风光", "从高处和街角看城市轮廓。", "城市风光", []string{"城市", "夜景"}},
	}
	for i, column := range photoColumns {
		mediaID := seedUUID(7, i+1)
		if err := s.ensureMedia(ctx, mediaID, media.PurposePhoto, 40+i, 1200, 800); err != nil {
			return err
		}
		if err := s.upsertColumn(ctx, content.EntityPhotoColumn, column.id, column.slug,
			map[string]any{
				"name": column.name, "description": column.description,
				"theme": column.theme, "coverMediaId": mediaID,
			}, column.tags, i); err != nil {
			return err
		}
		s.stats.PhotoColumns++
	}

	albums := albumSeedSpecs()
	for i, album := range albums {
		mediaID := ""
		if album.hasPhotos {
			mediaID = seedUUID(4, 100+i)
			if err := s.ensureMedia(ctx, mediaID, media.PurposePhoto, 60+i, 1200, 800); err != nil {
				return err
			}
		}
		if err := s.upsertAlbum(ctx, album, mediaID, i); err != nil {
			return err
		}
		s.stats.Albums++
	}

	photoIndex := 0
	for _, album := range albums {
		if !album.hasPhotos {
			continue
		}
		for j := 0; j < album.photoCount; j++ {
			photoIndex++
			photoID := seedUUID(5, photoIndex)
			mediaID := seedUUID(8, photoIndex)
			width, height := 1200, 800
			if photoIndex%3 == 0 {
				width, height = 900, 1200
			}
			if err := s.ensureMedia(ctx, mediaID, media.PurposePhoto, 100+photoIndex, width, height); err != nil {
				return err
			}
			createdAt := time.Now().Add(-time.Duration(photoIndex*3) * 24 * time.Hour)
			title := fmt.Sprintf("%s %02d", album.photoTitle, j+1)
			description := fmt.Sprintf("%s，%s。", album.photoDescription, album.photoTitle)
			tags := album.photoTags
			if err := s.upsertPhoto(ctx, photoID, album.id, mediaID, title, description,
				album.photoLocation, createdAt, tags, "published", photoIndex); err != nil {
				return err
			}
			s.stats.Photos++
		}
	}

	// One draft photo proves non-public rows never reach the read API.
	draftPhotoID := seedUUID(5, 90)
	draftMediaID := seedUUID(8, 90)
	if err := s.ensureMedia(ctx, draftMediaID, media.PurposePhoto, 190, 1200, 800); err != nil {
		return err
	}
	if err := s.upsertPhoto(ctx, draftPhotoID, albums[0].id, draftMediaID, "未提交的草稿",
		"这张照片仍处于草稿状态。", "", time.Now(), []string{"街拍"}, "draft", 90); err != nil {
		return err
	}

	if err := s.upsertRecommendations(ctx, []string{
		content.EntityArticle + ":" + seedUUID(2, 1),
		content.EntityAlbum + ":" + seedUUID(4, 1),
		content.EntityArticle + ":" + seedUUID(2, 3),
		content.EntityAlbum + ":" + seedUUID(4, 2),
		content.EntityArticle + ":" + seedUUID(2, 5),
	}); err != nil {
		return err
	}

	return s.countMedia(ctx)
}

func (s *seeder) countMedia(ctx context.Context) error {
	return s.pool.QueryRow(ctx,
		`SELECT count(*) FROM media_assets WHERE storage_key LIKE 'private/seed/%'`).Scan(&s.stats.Media)
}

type articleSpec struct {
	id, slug, columnSlug, title, summary, body string
	tags                                       []string
	cover                                      bool
	daysAgo                                    int
	status                                     string // published, draft, offline, trashed
	pending                                    *articleSpec
}

func articleSeedSpecs() []articleSpec {
	return []articleSpec{
		{
			id: seedUUID(2, 1), slug: "night-city-rain", columnSlug: "notes",
			title: "夜行城市：雨夜街拍的曝光取舍", summary: "在雨夜拍摄城市街景，霓虹与积水如何兼顾。",
			body: "# 雨夜的光\n\n雨把霓虹揉进积水里，快门时间成了取舍。\n\n## 参数\n\n- 光圈 f/2.8\n- 快门 1/60s\n- ISO 3200\n\n先保住高光，再给暗部留一点呼吸。",
			tags: []string{"摄影", "生活", "城市"}, cover: true, daysAgo: 1, status: "published",
		},
		{
			id: seedUUID(2, 2), slug: "go-observability", columnSlug: "tech",
			title: "Go 服务的可观测性入门", summary: "从日志、指标到追踪，给一个小服务补上基本观测能力。",
			body: "# 可观测性\n\n观测不是日志越多越好，而是出问题时能快速回答三个问题。\n\n```go\nfunc main() {\n    slog.Info(\"server started\")\n}\n```\n\n代码之外，更重要的是统一字段与请求 ID。",
			tags: []string{"技术"}, cover: true, daysAgo: 3, status: "published",
			pending: &articleSpec{
				title: "Go 服务的可观测性入门（修订）", summary: "补充指标与追踪章节，等待审核。",
				body: "# 修订中的版本\n\n这一版仍在审核中，公开端应继续显示旧版本。",
				tags: []string{"技术", "前端"},
			},
		},
		{
			id: seedUUID(2, 3), slug: "kyoto-four-days", columnSlug: "travel",
			title: "京都四日：从清水寺到鸭川", summary: "避开人流的四条散步路线，以及每天的落脚点。",
			body: "# 第一日\n\n清晨的清水寺还没有旅行团，石阶上只有扫地的声音。\n\n# 第四日\n\n鸭川边的风把四天的疲惫吹散。",
			tags: []string{"旅记", "摄影"}, cover: true, daysAgo: 6, status: "published",
		},
		{
			id: seedUUID(2, 4), slug: "astro-static-blog", columnSlug: "tech",
			title: "用 Astro 构建静态博客", summary: "静态生成的取舍、目录结构与图片处理方式。",
			body: "# 为什么是静态\n\n内容站的请求大多是读取，静态文件最直接。\n\n```js\nconst posts = await Astro.glob('./posts/*.md');\n```\n\n## 构建的代价\n\n假设有 $n$ 个页面，每个页面平均耗时 $t_i$，总构建时间近似为：\n\n$$\nT = \\sum_{i=1}^{n} t_i\n$$\n\n发布链路可以简化为：\n\n```mermaid\nflowchart LR\n  A[编辑提交] --> B[管理员审批]\n  B --> C[Astro 构建]\n  C --> D[静态发布]\n```\n\n构建失败时旧版本继续服务，是静态站的额外好处。",
			tags: []string{"前端", "技术"}, cover: true, daysAgo: 9, status: "published",
		},
		{
			id: seedUUID(2, 5), slug: "tide-and-rocks", columnSlug: "travel",
			title: "潮汐与礁石：海边长曝光的节奏", summary: "等一场退潮，把海面拍成雾。",
			body: "# 等潮\n\n潮位表比天气更可靠。\n\n长曝光把浪抹平，但礁石的纹理需要保留，所以曝光不能一味拉长。",
			tags: []string{"摄影", "旅记"}, cover: true, daysAgo: 12, status: "published",
		},
		{
			id: seedUUID(2, 6), slug: "postgres-index-notes", columnSlug: "tech",
			title: "PostgreSQL 索引选择笔记", summary: "B-tree、GIN 与 trigram 各自适合什么问题。",
			body: "# 索引不是越多越好\n\n先看查询模式，再决定索引。\n\n```sql\nCREATE INDEX ON articles USING gin ((payload ->> 'title') gin_trgm_ops);\n```\n\n模糊搜索与精确匹配要分开考虑。",
			tags: []string{"技术"}, cover: true, daysAgo: 15, status: "published",
		},
		{
			id: seedUUID(2, 7), slug: "mountain-mist", columnSlug: "travel",
			title: "山间晨雾", summary: "五点出发，只为一段十分钟的雾。",
			body: "# 早起\n\n雾散得比想象中快，手忙脚乱地换镜头也是旅途的一部分。",
			tags: []string{"旅记"}, cover: true, daysAgo: 18, status: "published",
		},
		{
			id: seedUUID(2, 8), slug: "dark-theme-tokens", columnSlug: "tech",
			title: "设计 token 与暗色主题", summary: "用语义化 token 管理两套主题，而不是到处写颜色值。",
			body: "# 语义化\n\n颜色要有名字，并且名字描述用途，而不是色相。",
			tags: []string{"前端"}, cover: true, daysAgo: 21, status: "published",
		},
		{
			id: seedUUID(2, 9), slug: "convenience-store-4am", columnSlug: "notes",
			title: "凌晨四点的便利店", summary: "城市安静下来之后，灯还亮着的地方。",
			body: "# 四点\n\n收银员在补货，热柜的声音比白天清楚。",
			tags: []string{"生活"}, cover: false, daysAgo: 24, status: "published",
		},
		{
			id: seedUUID(2, 10), slug: "practice-of-slowing-down", columnSlug: "notes",
			title: "关于慢下来的练习", summary: "把注意力放回手头的一件事。",
			body: "# 慢\n\n慢不是拖延，而是让动作完整。",
			tags: []string{"生活"}, cover: false, daysAgo: 27, status: "published",
		},
		{
			id: seedUUID(2, 11), slug: "unfinished-draft", columnSlug: "notes",
			title: "尚未完成的草稿", summary: "草稿不应出现在公开接口。",
			body: "草稿正文。", tags: []string{"生活"}, daysAgo: 2, status: "draft",
		},
		{
			id: seedUUID(2, 12), slug: "offline-article", columnSlug: "tech",
			title: "已下线的旧文章", summary: "下线后公开端不可见。",
			body: "下线正文。", tags: []string{"技术"}, daysAgo: 30, status: "offline",
		},
		{
			id: seedUUID(2, 13), slug: "trashed-article", columnSlug: "tech",
			title: "回收站里的文章", summary: "回收站内容不可见。",
			body: "回收站正文。", tags: []string{"技术"}, daysAgo: 33, status: "trashed",
		},
	}
}

type albumSpec struct {
	id, slug, columnSlug, title, description, location, shotAt string
	tags                                                       []string
	daysAgo                                                    int
	hasPhotos                                                  bool
	photoCount                                                 int
	photoTitle, photoDescription, photoLocation                string
	photoTags                                                  []string
}

func albumSeedSpecs() []albumSpec {
	return []albumSpec{
		{
			id: seedUUID(4, 1), slug: "neon-rain", columnSlug: "street",
			title: "雨夜霓虹", description: "雨停之前的两个小时。", location: "上海 · 南京东路",
			shotAt: "2026-10-18", tags: []string{"街拍", "夜景", "城市"}, daysAgo: 10,
			hasPhotos: true, photoCount: 5, photoTitle: "霓虹倒影", photoDescription: "积水里的灯牌",
			photoLocation: "上海 · 南京东路", photoTags: []string{"街拍", "夜景"},
		},
		{
			id: seedUUID(4, 2), slug: "afternoon-alley", columnSlug: "street",
			title: "巷口的午后", description: "旧城区里缓慢的时间。", location: "苏州 · 平江路",
			shotAt: "2026-09-27", tags: []string{"街拍", "人文"}, daysAgo: 20,
			hasPhotos: true, photoCount: 5, photoTitle: "巷口", photoDescription: "午后的光落在墙上",
			photoLocation: "苏州 · 平江路", photoTags: []string{"街拍", "人文"},
		},
		{
			id: seedUUID(4, 3), slug: "intertidal-zone", columnSlug: "landscape",
			title: "潮间带", description: "退潮之后露出的礁石。", location: "舟山 · 东极岛",
			shotAt: "2026-08-15", tags: []string{"自然风光"}, daysAgo: 30,
			hasPhotos: true, photoCount: 5, photoTitle: "潮间带", photoDescription: "潮汐退去后的礁石",
			photoLocation: "舟山 · 东极岛", photoTags: []string{"自然风光"},
		},
		{
			id: seedUUID(4, 4), slug: "ridge-line", columnSlug: "landscape",
			title: "山脊线", description: "晨雾里的山脊与草坡。", location: "云南 · 雨崩",
			shotAt: "2026-07-04", tags: []string{"自然风光"}, daysAgo: 40,
			hasPhotos: true, photoCount: 5, photoTitle: "山脊", photoDescription: "雾散之前的山脊线",
			photoLocation: "云南 · 雨崩", photoTags: []string{"自然风光"},
		},
		{
			id: seedUUID(4, 5), slug: "skyline", columnSlug: "cityscape",
			title: "天际线", description: "从高处看城市的边缘。", location: "深圳 · 莲花山",
			shotAt: "2026-06-11", tags: []string{"城市", "夜景"}, daysAgo: 50,
			hasPhotos: true, photoCount: 5, photoTitle: "天际线", photoDescription: "入夜前的城市轮廓",
			photoLocation: "深圳 · 莲花山", photoTags: []string{"城市", "夜景"},
		},
		{
			id: seedUUID(4, 6), slug: "old-town-walk", columnSlug: "cityscape",
			title: "旧城散步", description: "沿着旧城墙走一圈。", location: "西安 · 城墙",
			shotAt: "2026-05-06", tags: []string{"城市", "人文"}, daysAgo: 60,
			hasPhotos: true, photoCount: 5, photoTitle: "旧城", photoDescription: "城墙边的日常",
			photoLocation: "西安 · 城墙", photoTags: []string{"城市", "人文"},
		},
		{
			id: seedUUID(4, 7), slug: "empty-album", columnSlug: "cityscape",
			title: "空相册", description: "还没有公开照片的相册，不应出现在公开列表。",
			location: "", shotAt: "", tags: []string{"城市"}, daysAgo: 5,
			hasPhotos: false,
		},
	}
}

func (s *seeder) upsertColumn(ctx context.Context, entityType, id, slug string, payload map[string]any, tags []string, offset int) error {
	table := "article_columns"
	scope := content.ScopeArticle
	if entityType == content.EntityPhotoColumn {
		table = "photo_columns"
		scope = content.ScopePhoto
	}
	createdAt := time.Now().Add(-time.Duration(offset+30) * 24 * time.Hour)
	_, err := s.pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s (id, slug, status, created_by, created_at, updated_at)
		VALUES ($1, $2, 'published', $3, $4, now())
		ON CONFLICT (id) DO UPDATE SET slug = EXCLUDED.slug, status = 'published',
		    deleted_at = NULL, updated_at = now()
	`, table), id, slug, s.ownerID, createdAt)
	if err != nil {
		return fmt.Errorf("upsert %s: %w", table, err)
	}

	publishedAt := createdAt
	versionID, err := s.upsertVersion(ctx, entityType, id, 1, "approved", payload, &publishedAt)
	if err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, fmt.Sprintf(
		`UPDATE %s SET published_version_id = $2 WHERE id = $1`, table), id, versionID); err != nil {
		return fmt.Errorf("link %s version: %w", table, err)
	}
	return s.replaceTags(ctx, versionID, scope, tags)
}

func (s *seeder) upsertArticle(ctx context.Context, spec articleSpec, mediaID string, offset int) error {
	columnID := seedUUID(1, columnIndex(spec.columnSlug))
	publishedAt := time.Now().Add(-time.Duration(spec.daysAgo) * 24 * time.Hour)
	status := spec.status
	var deletedAt *time.Time
	if status == "trashed" {
		now := time.Now()
		deletedAt = &now
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO articles (id, column_id, slug, status, deleted_at, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (id) DO UPDATE SET column_id = EXCLUDED.column_id, slug = EXCLUDED.slug,
		    status = EXCLUDED.status, deleted_at = EXCLUDED.deleted_at, updated_at = now()
	`, spec.id, columnID, spec.slug, status, deletedAt, s.ownerID, publishedAt)
	if err != nil {
		return fmt.Errorf("upsert article: %w", err)
	}

	payload := map[string]any{
		"title": spec.title, "summary": spec.summary,
		"bodyMarkdown": spec.body, "bodyIndex": bodyIndex(spec.body),
		"coverMediaId": mediaID,
	}
	versionStatus := "approved"
	if status == "draft" {
		versionStatus = "draft"
	}
	versionID, err := s.upsertVersion(ctx, content.EntityArticle, spec.id, 1, versionStatus, payload, &publishedAt)
	if err != nil {
		return err
	}
	if err := s.replaceTags(ctx, versionID, content.ScopeArticle, spec.tags); err != nil {
		return err
	}

	var publishedVersionID any
	if status == "published" || status == "offline" {
		publishedVersionID = versionID
	}
	if _, err := s.pool.Exec(ctx, `UPDATE articles SET published_version_id = $2 WHERE id = $1`,
		spec.id, publishedVersionID); err != nil {
		return fmt.Errorf("link article version: %w", err)
	}

	if spec.pending != nil {
		pendingPayload := map[string]any{
			"title": spec.pending.title, "summary": spec.pending.summary,
			"bodyMarkdown": spec.pending.body, "bodyIndex": bodyIndex(spec.pending.body),
			"coverMediaId": mediaID,
		}
		pendingVersion, err := s.upsertVersion(ctx, content.EntityArticle, spec.id, 2, "pending", pendingPayload, nil)
		if err != nil {
			return err
		}
		if err := s.replaceTags(ctx, pendingVersion, content.ScopeArticle, spec.pending.tags); err != nil {
			return err
		}
	}
	return nil
}

func (s *seeder) upsertAlbum(ctx context.Context, spec albumSpec, mediaID string, offset int) error {
	columnID := seedUUID(3, photoColumnIndex(spec.columnSlug))
	publishedAt := time.Now().Add(-time.Duration(spec.daysAgo) * 24 * time.Hour)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO albums (id, column_id, slug, status, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, 'published', $4, $5, now())
		ON CONFLICT (id) DO UPDATE SET column_id = EXCLUDED.column_id, slug = EXCLUDED.slug,
		    status = 'published', deleted_at = NULL, updated_at = now()
	`, spec.id, columnID, spec.slug, s.ownerID, publishedAt)
	if err != nil {
		return fmt.Errorf("upsert album: %w", err)
	}

	payload := map[string]any{
		"title": spec.title, "description": spec.description,
		"location": spec.location, "shotAt": spec.shotAt, "coverMediaId": mediaID,
	}
	versionID, err := s.upsertVersion(ctx, content.EntityAlbum, spec.id, 1, "approved", payload, &publishedAt)
	if err != nil {
		return err
	}
	if err := s.replaceTags(ctx, versionID, content.ScopePhoto, spec.tags); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `UPDATE albums SET published_version_id = $2 WHERE id = $1`,
		spec.id, versionID); err != nil {
		return fmt.Errorf("link album version: %w", err)
	}
	return nil
}

func (s *seeder) upsertPhoto(ctx context.Context, id, albumID, mediaID, title, description, location string,
	createdAt time.Time, tags []string, status string, sortOrder int) error {
	var deletedAt *time.Time
	_, err := s.pool.Exec(ctx, `
		INSERT INTO photos (id, album_id, status, sort_order, deleted_at, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (id) DO UPDATE SET album_id = EXCLUDED.album_id, status = EXCLUDED.status,
		    sort_order = EXCLUDED.sort_order, deleted_at = EXCLUDED.deleted_at, updated_at = now()
	`, id, albumID, status, sortOrder, deletedAt, s.ownerID, createdAt)
	if err != nil {
		return fmt.Errorf("upsert photo: %w", err)
	}

	payload := map[string]any{
		"mediaId": mediaID, "title": title, "description": description,
		"location": location, "takenAt": createdAt.Format(time.RFC3339),
	}
	versionStatus := "approved"
	if status == "draft" {
		versionStatus = "draft"
	}
	versionID, err := s.upsertVersion(ctx, content.EntityPhoto, id, 1, versionStatus, payload, &createdAt)
	if err != nil {
		return err
	}
	if err := s.replaceTags(ctx, versionID, content.ScopePhoto, tags); err != nil {
		return err
	}
	var publishedVersionID any
	if status == "published" {
		publishedVersionID = versionID
	}
	if _, err := s.pool.Exec(ctx, `UPDATE photos SET published_version_id = $2 WHERE id = $1`,
		id, publishedVersionID); err != nil {
		return fmt.Errorf("link photo version: %w", err)
	}
	return nil
}

func (s *seeder) upsertVersion(ctx context.Context, entityType, entityID string, no int, status string,
	payload map[string]any, publishedAt *time.Time) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode payload: %w", err)
	}
	var id string
	err = s.pool.QueryRow(ctx, `
		INSERT INTO content_versions (
			entity_type, entity_id, version_no, status, payload, published_at, created_by, reviewed_at
		) VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, CASE WHEN $4 = 'approved' THEN now() END)
		ON CONFLICT (entity_type, entity_id, version_no)
		DO UPDATE SET status = EXCLUDED.status, payload = EXCLUDED.payload,
		    published_at = EXCLUDED.published_at
		RETURNING id::text
	`, entityType, entityID, no, status, string(raw), publishedAt, s.ownerID).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert content version: %w", err)
	}
	return id, nil
}

func (s *seeder) replaceTags(ctx context.Context, versionID, scope string, names []string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM content_tags WHERE content_version_id = $1`, versionID); err != nil {
		return fmt.Errorf("clear content tags: %w", err)
	}
	for _, name := range names {
		var tagID string
		err := s.pool.QueryRow(ctx, `
			INSERT INTO tags (scope, name) VALUES ($1, $2)
			ON CONFLICT (scope, name) DO UPDATE SET active = true
			RETURNING id::text
		`, scope, name).Scan(&tagID)
		if err != nil {
			return fmt.Errorf("upsert tag %s: %w", name, err)
		}
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO content_tags (content_version_id, tag_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, versionID, tagID); err != nil {
			return fmt.Errorf("attach tag %s: %w", name, err)
		}
	}
	return nil
}

func (s *seeder) upsertRecommendations(ctx context.Context, items []string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM recommendations`); err != nil {
		return fmt.Errorf("clear recommendations: %w", err)
	}
	for i, item := range items {
		var entityType, entityID string
		for idx := 0; idx < len(item); idx++ {
			if item[idx] == ':' {
				entityType, entityID = item[:idx], item[idx+1:]
				break
			}
		}
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO recommendations (entity_type, entity_id, position, created_by)
			VALUES ($1, $2, $3, $4)
		`, entityType, entityID, i+1, s.ownerID); err != nil {
			return fmt.Errorf("insert recommendation: %w", err)
		}
	}
	return nil
}

func (s *seeder) ensureMedia(ctx context.Context, id, purpose string, variant int, width, height int) error {
	var variantsRaw []byte
	err := s.pool.QueryRow(ctx, `SELECT variants FROM media_assets WHERE id = $1`, id).Scan(&variantsRaw)
	if err == nil {
		var variants map[string]media.VariantInfo
		_ = json.Unmarshal(variantsRaw, &variants)
		for _, variant := range variants {
			if _, statErr := s.backend.Stat(ctx, variant.Key); statErr == nil {
				return nil
			}
			break
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("check seed media: %w", err)
	}

	data := generatePNG(width, height, variant)
	key := fmt.Sprintf("private/seed/%s.png", id)
	if _, err := s.backend.Put(ctx, key, bytes.NewReader(data), 50<<20); err != nil {
		return fmt.Errorf("store seed image: %w", err)
	}
	asset := media.Asset{
		ID:               id,
		OwnerID:          s.ownerID,
		Purpose:          purpose,
		StorageKey:       key,
		OriginalFilename: id + ".png",
		MimeType:         "image/png",
		FileSize:         int64(len(data)),
		Status:           media.StatusUploading,
	}
	result, err := s.processor.Process(ctx, asset)
	if err != nil {
		return fmt.Errorf("process seed image %s: %w", id, err)
	}
	exifRaw, _ := json.Marshal(result.Exif)
	variantsJSON, _ := json.Marshal(result.Variants)
	_, err = s.pool.Exec(ctx, `
		INSERT INTO media_assets (
			id, owner_id, purpose, storage_key, original_filename, mime_type, file_size,
			width, height, blurhash, status, master_key, exif, variants, processed_at
		) VALUES ($1, $2, $3, $4, $5, 'image/png', $6, $7, $8, $9, 'ready', $10, $11::jsonb, $12::jsonb, now())
		ON CONFLICT (id) DO UPDATE SET storage_key = EXCLUDED.storage_key,
		    file_size = EXCLUDED.file_size, width = EXCLUDED.width, height = EXCLUDED.height,
		    blurhash = EXCLUDED.blurhash, status = 'ready', master_key = EXCLUDED.master_key,
		    exif = EXCLUDED.exif, variants = EXCLUDED.variants, processed_at = now(),
		    updated_at = now()
	`, id, s.ownerID, purpose, key, asset.OriginalFilename, len(data),
		result.Width, result.Height, result.Blurhash, result.MasterKey,
		string(exifRaw), string(variantsJSON))
	if err != nil {
		return fmt.Errorf("upsert seed media: %w", err)
	}
	return nil
}

func seedUUID(group, n int) string {
	return fmt.Sprintf("%08d-0000-4000-8000-%012d", group, n)
}

func columnIndex(slug string) int {
	switch slug {
	case "tech":
		return 1
	case "travel":
		return 2
	default:
		return 3
	}
}

func photoColumnIndex(slug string) int {
	switch slug {
	case "street":
		return 1
	case "landscape":
		return 2
	default:
		return 3
	}
}

// bodyIndex strips fenced code blocks and Markdown markers before indexing.
func bodyIndex(markdown string) string {
	var out bytes.Buffer
	inFence := false
	for _, line := range bytes.Split([]byte(markdown), []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte("```")) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		trimmed = bytes.TrimLeft(trimmed, "#>-* \t")
		out.Write(trimmed)
		out.WriteByte(' ')
	}
	return string(bytes.TrimSpace(out.Bytes()))
}

// generatePNG draws a deterministic gradient so seed photos have real pixels
// and real derivatives without shipping binary fixtures.
func generatePNG(width, height, variant int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	palettes := [][2]color.RGBA{
		{{R: 24, G: 32, B: 54, A: 255}, {R: 96, G: 84, B: 120, A: 255}},
		{{R: 32, G: 56, B: 72, A: 255}, {R: 168, G: 156, B: 120, A: 255}},
		{{R: 44, G: 36, B: 30, A: 255}, {R: 196, G: 132, B: 92, A: 255}},
		{{R: 18, G: 44, B: 40, A: 255}, {R: 120, G: 176, B: 148, A: 255}},
		{{R: 48, G: 24, B: 44, A: 255}, {R: 184, G: 108, B: 136, A: 255}},
	}
	colors := palettes[variant%len(palettes)]
	for y := 0; y < height; y++ {
		ratio := float64(y) / float64(height)
		row := color.RGBA{
			R: uint8(float64(colors[0].R) + (float64(colors[1].R)-float64(colors[0].R))*ratio),
			G: uint8(float64(colors[0].G) + (float64(colors[1].G)-float64(colors[0].G))*ratio),
			B: uint8(float64(colors[0].B) + (float64(colors[1].B)-float64(colors[0].B))*ratio),
			A: 255,
		}
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, row)
		}
	}
	// A few soft blocks keep the waterfall from looking uniform.
	for i := 0; i < 4; i++ {
		cx := (variant*37 + i*211) % width
		cy := (variant*53 + i*167) % height
		radius := width/6 + (variant%7)*12
		tint := color.RGBA{
			R: uint8(120 + (variant*13+i*29)%120),
			G: uint8(120 + (variant*17+i*31)%120),
			B: uint8(120 + (variant*19+i*23)%120),
			A: 255,
		}
		drawDisc(img, cx, cy, radius, tint)
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func drawDisc(img *image.RGBA, cx, cy, radius int, tint color.RGBA) {
	bounds := img.Bounds()
	for y := cy - radius; y <= cy+radius; y++ {
		if y < bounds.Min.Y || y >= bounds.Max.Y {
			continue
		}
		for x := cx - radius; x <= cx+radius; x++ {
			if x < bounds.Min.X || x >= bounds.Max.X {
				continue
			}
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy > radius*radius {
				continue
			}
			original := img.RGBAAt(x, y)
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((int(original.R) + int(tint.R)) / 3),
				G: uint8((int(original.G) + int(tint.G)) / 3),
				B: uint8((int(original.B) + int(tint.B)) / 3),
				A: 255,
			})
		}
	}
}
