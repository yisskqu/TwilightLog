package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yisskqu/TwilightLog/services/api/internal/media"
)

// Repository reads the published snapshots used by the public site.
type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

type articlePayload struct {
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	BodyMarkdown string `json:"bodyMarkdown"`
	BodyIndex    string `json:"bodyIndex"`
	CoverMediaID string `json:"coverMediaId"`
}

type columnPayload struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Theme        string `json:"theme"`
	CoverMediaID string `json:"coverMediaId"`
}

type albumPayload struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	Location     string `json:"location"`
	ShotAt       string `json:"shotAt"`
	CoverMediaID string `json:"coverMediaId"`
}

type photoPayload struct {
	MediaID     string `json:"mediaId"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Location    string `json:"location"`
	TakenAt     string `json:"takenAt"`
}

func decodePayload[T any](raw []byte) T {
	var value T
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &value)
	}
	return value
}

func decodeVariants(raw []byte) map[string]media.VariantInfo {
	variants := map[string]media.VariantInfo{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &variants)
	}
	return variants
}

// buildMedia returns nil when the joined media was missing or not ready.
func buildMedia(id string, width, height int, blurhash string, variantsRaw []byte) *MediaAsset {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	return &MediaAsset{
		ID:       id,
		Width:    width,
		Height:   height,
		Blurhash: blurhash,
		Variants: decodeVariants(variantsRaw),
	}
}

func likePattern(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + replacer.Replace(value) + "%"
}

type argList struct {
	values []any
}

func (a *argList) add(value any) string {
	a.values = append(a.values, value)
	return fmt.Sprintf("$%d", len(a.values))
}

func (a *argList) all() []any {
	return a.values
}

const articleSelect = `
SELECT a.id::text, a.slug, cv.payload, cv.published_at,
       c.slug, COALESCE(cv2.payload ->> 'name', ''),
       COALESCE(m.id::text, ''), COALESCE(m.width, 0), COALESCE(m.height, 0),
       COALESCE(m.blurhash, ''), COALESCE(m.variants, '{}'::jsonb),
       COALESCE(tg.names, ARRAY[]::text[]),
       count(*) OVER() AS total
FROM articles a
JOIN article_columns c ON c.id = a.column_id
JOIN content_versions cv ON cv.id = a.published_version_id
JOIN content_versions cv2 ON cv2.id = c.published_version_id
LEFT JOIN media_assets m
       ON m.id::text = NULLIF(cv.payload ->> 'coverMediaId', '') AND m.status = 'ready'
LEFT JOIN LATERAL (
    SELECT array_agg(t.name ORDER BY t.name) AS names
    FROM content_tags ct
    JOIN tags t ON t.id = ct.tag_id
    WHERE ct.content_version_id = cv.id AND t.active
) tg ON TRUE
WHERE a.status = 'published' AND a.deleted_at IS NULL
  AND c.status = 'published' AND c.deleted_at IS NULL AND c.published_version_id IS NOT NULL`

// ListArticles lists published articles, optionally restricted to one column slug.
func (r *Repository) ListArticles(ctx context.Context, columnSlug string, q Query) (Page[Article], error) {
	var args argList
	var where strings.Builder
	where.WriteString(articleSelect)

	if columnSlug != "" {
		fmt.Fprintf(&where, " AND c.slug = %s", args.add(columnSlug))
	}
	searchRank := ""
	if q.Search != "" {
		pattern := args.add(likePattern(q.Search))
		fmt.Fprintf(&where, ` AND (cv.payload ->> 'title' ILIKE %[1]s OR cv.payload ->> 'summary' ILIKE %[1]s
			OR cv.payload ->> 'bodyIndex' ILIKE %[1]s OR EXISTS (
				SELECT 1 FROM content_tags ct2 JOIN tags t2 ON t2.id = ct2.tag_id
				WHERE ct2.content_version_id = cv.id AND t2.active AND t2.name ILIKE %[1]s))`, pattern)
		searchRank = fmt.Sprintf(`CASE WHEN cv.payload ->> 'title' ILIKE %[1]s THEN 1
			WHEN cv.payload ->> 'summary' ILIKE %[1]s THEN 2
			WHEN cv.payload ->> 'bodyIndex' ILIKE %[1]s THEN 3 ELSE 2 END`, pattern)
	}
	appendTagFilter(&where, &args, "cv.id", q.Tags)
	appendDateFilter(&where, &args, "cv.published_at::date", q.DateFrom, q.DateTo)

	order := listOrder(searchRank, "cv.published_at", q.Sort)
	fmt.Fprintf(&where, " ORDER BY %s, a.id ASC LIMIT %s OFFSET %s",
		order, args.add(q.PageSize), args.add((q.Page-1)*q.PageSize))

	rows, err := r.pool.Query(ctx, where.String(), args.all()...)
	if err != nil {
		return Page[Article]{}, fmt.Errorf("list articles: %w", err)
	}
	defer rows.Close()

	var items []Article
	total := 0
	for rows.Next() {
		article, rowTotal, err := scanArticle(rows)
		if err != nil {
			return Page[Article]{}, err
		}
		total = rowTotal
		items = append(items, article)
	}
	if err := rows.Err(); err != nil {
		return Page[Article]{}, fmt.Errorf("iterate articles: %w", err)
	}
	return newPage(items, q.Page, q.PageSize, total), nil
}

// GetArticle returns one published article by slug.
func (r *Repository) GetArticle(ctx context.Context, slug string) (Article, error) {
	return r.getArticle(ctx, "a.slug = $1 AND "+visibleArticleTail, slug)
}

const visibleArticleTail = `a.status = 'published' AND a.deleted_at IS NULL
  AND c.status = 'published' AND c.deleted_at IS NULL AND c.published_version_id IS NOT NULL`

func (r *Repository) getArticle(ctx context.Context, condition string, value string) (Article, error) {
	sql := articleSelect + " AND " + condition
	row := r.pool.QueryRow(ctx, sql, value)
	article, _, err := scanArticle(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Article{}, ErrNotFound
	}
	if err != nil {
		return Article{}, fmt.Errorf("get article: %w", err)
	}
	return article, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanArticle(row rowScanner) (Article, int, error) {
	var article Article
	var payloadRaw []byte
	var publishedAt time.Time
	var columnSlug, columnName string
	var mediaID, blurhash string
	var mediaWidth, mediaHeight int
	var variantsRaw []byte
	var tags []string
	var total int64

	err := row.Scan(&article.ID, &article.Slug, &payloadRaw, &publishedAt,
		&columnSlug, &columnName, &mediaID, &mediaWidth, &mediaHeight, &blurhash,
		&variantsRaw, &tags, &total)
	if err != nil {
		return Article{}, 0, err
	}
	payload := decodePayload[articlePayload](payloadRaw)
	article.Title = payload.Title
	article.Summary = payload.Summary
	article.BodyMarkdown = payload.BodyMarkdown
	article.Column = ColumnRef{Slug: columnSlug, Name: columnName}
	article.Cover = buildMedia(mediaID, mediaWidth, mediaHeight, blurhash, variantsRaw)
	article.Tags = normalizeArray(tags)
	article.PublishedAt = publishedAt
	return article, int(total), nil
}

const articleColumnSelect = `
SELECT c.id::text, c.slug, cv.payload,
       COALESCE(m.id::text, ''), COALESCE(m.width, 0), COALESCE(m.height, 0),
       COALESCE(m.blurhash, ''), COALESCE(m.variants, '{}'::jsonb),
       COALESCE(tg.names, ARRAY[]::text[]),
       (SELECT count(*) FROM articles a2
        WHERE a2.column_id = c.id AND a2.status = 'published' AND a2.deleted_at IS NULL
          AND a2.published_version_id IS NOT NULL) AS article_count,
       GREATEST(c.created_at, COALESCE((
           SELECT max(cv2.published_at) FROM articles a2
           JOIN content_versions cv2 ON cv2.id = a2.published_version_id
           WHERE a2.column_id = c.id AND a2.status = 'published' AND a2.deleted_at IS NULL
       ), c.created_at)) AS updated_at,
       count(*) OVER() AS total
FROM article_columns c
JOIN content_versions cv ON cv.id = c.published_version_id
LEFT JOIN media_assets m
       ON m.id::text = NULLIF(cv.payload ->> 'coverMediaId', '') AND m.status = 'ready'
LEFT JOIN LATERAL (
    SELECT array_agg(t.name ORDER BY t.name) AS names
    FROM content_tags ct
    JOIN tags t ON t.id = ct.tag_id
    WHERE ct.content_version_id = cv.id AND t.active
) tg ON TRUE
WHERE c.status = 'published' AND c.deleted_at IS NULL`

// ListArticleColumns lists published article columns.
func (r *Repository) ListArticleColumns(ctx context.Context, q Query) (Page[ArticleColumn], error) {
	var args argList
	var sb strings.Builder
	sb.WriteString(articleColumnSelect)

	searchRank := ""
	if q.Search != "" {
		pattern := args.add(likePattern(q.Search))
		fmt.Fprintf(&sb, ` AND (cv.payload ->> 'name' ILIKE %[1]s OR cv.payload ->> 'description' ILIKE %[1]s
			OR EXISTS (SELECT 1 FROM content_tags ct2 JOIN tags t2 ON t2.id = ct2.tag_id
			WHERE ct2.content_version_id = cv.id AND t2.active AND t2.name ILIKE %[1]s))`, pattern)
		searchRank = fmt.Sprintf(`CASE WHEN cv.payload ->> 'name' ILIKE %[1]s THEN 1 ELSE 2 END`, pattern)
	}
	appendTagFilter(&sb, &args, "cv.id", q.Tags)
	appendDateFilter(&sb, &args, "c.created_at::date", q.DateFrom, q.DateTo)

	order := listOrder(searchRank, "updated_at", q.Sort)
	fmt.Fprintf(&sb, " ORDER BY %s, c.id ASC LIMIT %s OFFSET %s",
		order, args.add(q.PageSize), args.add((q.Page-1)*q.PageSize))

	rows, err := r.pool.Query(ctx, sb.String(), args.all()...)
	if err != nil {
		return Page[ArticleColumn]{}, fmt.Errorf("list article columns: %w", err)
	}
	defer rows.Close()

	var items []ArticleColumn
	total := 0
	for rows.Next() {
		var column ArticleColumn
		var payloadRaw []byte
		var mediaID, blurhash string
		var mediaWidth, mediaHeight int
		var variantsRaw []byte
		var tags []string
		var rowTotal int64
		if err := rows.Scan(&column.ID, &column.Slug, &payloadRaw,
			&mediaID, &mediaWidth, &mediaHeight, &blurhash, &variantsRaw, &tags,
			&column.ArticleCount, &column.UpdatedAt, &rowTotal); err != nil {
			return Page[ArticleColumn]{}, fmt.Errorf("scan article column: %w", err)
		}
		payload := decodePayload[columnPayload](payloadRaw)
		column.Name = payload.Name
		column.Description = payload.Description
		column.Cover = buildMedia(mediaID, mediaWidth, mediaHeight, blurhash, variantsRaw)
		column.Tags = normalizeArray(tags)
		total = int(rowTotal)
		items = append(items, column)
	}
	if err := rows.Err(); err != nil {
		return Page[ArticleColumn]{}, fmt.Errorf("iterate article columns: %w", err)
	}
	return newPage(items, q.Page, q.PageSize, total), nil
}

// GetArticleColumn returns one published article column by slug.
func (r *Repository) GetArticleColumn(ctx context.Context, slug string) (ArticleColumn, error) {
	var column ArticleColumn
	var payloadRaw []byte
	var mediaID, blurhash string
	var mediaWidth, mediaHeight int
	var variantsRaw []byte
	var tags []string
	var total int64
	err := r.pool.QueryRow(ctx, articleColumnSelect+" AND c.slug = $1", slug).Scan(
		&column.ID, &column.Slug, &payloadRaw,
		&mediaID, &mediaWidth, &mediaHeight, &blurhash, &variantsRaw, &tags,
		&column.ArticleCount, &column.UpdatedAt, &total)
	if errors.Is(err, pgx.ErrNoRows) {
		return ArticleColumn{}, ErrNotFound
	}
	if err != nil {
		return ArticleColumn{}, fmt.Errorf("get article column: %w", err)
	}
	payload := decodePayload[columnPayload](payloadRaw)
	column.Name = payload.Name
	column.Description = payload.Description
	column.Cover = buildMedia(mediaID, mediaWidth, mediaHeight, blurhash, variantsRaw)
	column.Tags = normalizeArray(tags)
	return column, nil
}

const photoColumnSelect = `SELECT pc.id::text, pc.slug, cv.payload,
       COALESCE(m.id::text, ''), COALESCE(m.width, 0), COALESCE(m.height, 0),
       COALESCE(m.blurhash, ''), COALESCE(m.variants, '{}'::jsonb),
       COALESCE(tg.names, ARRAY[]::text[]),
       (SELECT count(*) FROM albums al
        WHERE al.column_id = pc.id AND al.status = 'published' AND al.deleted_at IS NULL
          AND album_has_public_photo(al.id)) AS album_count,
       GREATEST(pc.created_at, COALESCE((
           SELECT max(al.updated_at) FROM albums al
           WHERE al.column_id = pc.id AND al.status = 'published' AND al.deleted_at IS NULL
             AND album_has_public_photo(al.id)
       ), pc.created_at)) AS updated_at,
       count(*) OVER() AS total
FROM photo_columns pc
JOIN content_versions cv ON cv.id = pc.published_version_id
LEFT JOIN media_assets m
       ON m.id::text = NULLIF(cv.payload ->> 'coverMediaId', '') AND m.status = 'ready'
LEFT JOIN LATERAL (
    SELECT array_agg(t.name ORDER BY t.name) AS names
    FROM content_tags ct
    JOIN tags t ON t.id = ct.tag_id
    WHERE ct.content_version_id = cv.id AND t.active
) tg ON TRUE
WHERE pc.status = 'published' AND pc.deleted_at IS NULL`

// ListPhotoColumns lists published photography columns.
func (r *Repository) ListPhotoColumns(ctx context.Context, theme string, q Query) (Page[PhotoColumn], error) {
	var args argList
	var sb strings.Builder
	sb.WriteString(photoColumnSelect)

	if theme != "" {
		fmt.Fprintf(&sb, " AND cv.payload ->> 'theme' = %s", args.add(theme))
	}
	searchRank := ""
	if q.Search != "" {
		pattern := args.add(likePattern(q.Search))
		fmt.Fprintf(&sb, ` AND (cv.payload ->> 'name' ILIKE %[1]s OR cv.payload ->> 'description' ILIKE %[1]s
			OR EXISTS (SELECT 1 FROM content_tags ct2 JOIN tags t2 ON t2.id = ct2.tag_id
			WHERE ct2.content_version_id = cv.id AND t2.active AND t2.name ILIKE %[1]s))`, pattern)
		searchRank = fmt.Sprintf(`CASE WHEN cv.payload ->> 'name' ILIKE %[1]s THEN 1 ELSE 2 END`, pattern)
	}
	appendTagFilter(&sb, &args, "cv.id", q.Tags)
	appendDateFilter(&sb, &args, "pc.created_at::date", q.DateFrom, q.DateTo)

	order := listOrder(searchRank, "updated_at", q.Sort)
	fmt.Fprintf(&sb, " ORDER BY %s, pc.id ASC LIMIT %s OFFSET %s",
		order, args.add(q.PageSize), args.add((q.Page-1)*q.PageSize))

	rows, err := r.pool.Query(ctx, sb.String(), args.all()...)
	if err != nil {
		return Page[PhotoColumn]{}, fmt.Errorf("list photo columns: %w", err)
	}
	defer rows.Close()

	var items []PhotoColumn
	total := 0
	for rows.Next() {
		var column PhotoColumn
		var payloadRaw []byte
		var mediaID, blurhash string
		var mediaWidth, mediaHeight int
		var variantsRaw []byte
		var tags []string
		var rowTotal int64
		if err := rows.Scan(&column.ID, &column.Slug, &payloadRaw,
			&mediaID, &mediaWidth, &mediaHeight, &blurhash, &variantsRaw, &tags,
			&column.AlbumCount, &column.UpdatedAt, &rowTotal); err != nil {
			return Page[PhotoColumn]{}, fmt.Errorf("scan photo column: %w", err)
		}
		payload := decodePayload[columnPayload](payloadRaw)
		column.Name = payload.Name
		column.Description = payload.Description
		column.Theme = payload.Theme
		column.Cover = buildMedia(mediaID, mediaWidth, mediaHeight, blurhash, variantsRaw)
		column.Tags = normalizeArray(tags)
		total = int(rowTotal)
		items = append(items, column)
	}
	if err := rows.Err(); err != nil {
		return Page[PhotoColumn]{}, fmt.Errorf("iterate photo columns: %w", err)
	}
	return newPage(items, q.Page, q.PageSize, total), nil
}

// GetPhotoColumn returns one published photography column by slug.
func (r *Repository) GetPhotoColumn(ctx context.Context, slug string) (PhotoColumn, error) {
	var column PhotoColumn
	var payloadRaw []byte
	var mediaID, blurhash string
	var mediaWidth, mediaHeight int
	var variantsRaw []byte
	var tags []string
	var total int64
	err := r.pool.QueryRow(ctx, photoColumnSelect+" AND pc.slug = $1", slug).Scan(
		&column.ID, &column.Slug, &payloadRaw,
		&mediaID, &mediaWidth, &mediaHeight, &blurhash, &variantsRaw, &tags,
		&column.AlbumCount, &column.UpdatedAt, &total)
	if errors.Is(err, pgx.ErrNoRows) {
		return PhotoColumn{}, ErrNotFound
	}
	if err != nil {
		return PhotoColumn{}, fmt.Errorf("get photo column: %w", err)
	}
	payload := decodePayload[columnPayload](payloadRaw)
	column.Name = payload.Name
	column.Description = payload.Description
	column.Theme = payload.Theme
	column.Cover = buildMedia(mediaID, mediaWidth, mediaHeight, blurhash, variantsRaw)
	column.Tags = normalizeArray(tags)
	return column, nil
}

const albumSelect = `
SELECT al.id::text, al.slug, cv.payload, cv.published_at,
       pc.slug, COALESCE(pcv.payload ->> 'name', ''),
       COALESCE(m.id::text, ''), COALESCE(m.width, 0), COALESCE(m.height, 0),
       COALESCE(m.blurhash, ''), COALESCE(m.variants, '{}'::jsonb),
       COALESCE(tg.names, ARRAY[]::text[]),
       stats.photo_count, stats.latest_photo_at,
       count(*) OVER() AS total
FROM albums al
JOIN photo_columns pc ON pc.id = al.column_id
JOIN content_versions cv ON cv.id = al.published_version_id
JOIN content_versions pcv ON pcv.id = pc.published_version_id
LEFT JOIN media_assets m
       ON m.id::text = NULLIF(cv.payload ->> 'coverMediaId', '') AND m.status = 'ready'
LEFT JOIN LATERAL (
    SELECT array_agg(t.name ORDER BY t.name) AS names
    FROM content_tags ct
    JOIN tags t ON t.id = ct.tag_id
    WHERE ct.content_version_id = cv.id AND t.active
) tg ON TRUE
LEFT JOIN LATERAL (
    SELECT count(*) AS photo_count, max(p2.created_at) AS latest_photo_at
    FROM photos p2
    JOIN content_versions cv2 ON cv2.id = p2.published_version_id
    JOIN media_assets m2
         ON m2.id::text = NULLIF(cv2.payload ->> 'mediaId', '') AND m2.status = 'ready'
    WHERE p2.album_id = al.id AND p2.status = 'published' AND p2.deleted_at IS NULL
) stats ON TRUE
WHERE al.status = 'published' AND al.deleted_at IS NULL
  AND pc.status = 'published' AND pc.deleted_at IS NULL AND pc.published_version_id IS NOT NULL
  AND stats.photo_count > 0`

// ListAlbums lists published albums that contain at least one public photo.
func (r *Repository) ListAlbums(ctx context.Context, columnSlug string, q Query) (Page[Album], error) {
	var args argList
	var sb strings.Builder
	sb.WriteString(albumSelect)

	if columnSlug != "" {
		fmt.Fprintf(&sb, " AND pc.slug = %s", args.add(columnSlug))
	}
	searchRank := ""
	if q.Search != "" {
		pattern := args.add(likePattern(q.Search))
		fmt.Fprintf(&sb, ` AND (cv.payload ->> 'title' ILIKE %[1]s OR cv.payload ->> 'description' ILIKE %[1]s
			OR cv.payload ->> 'location' ILIKE %[1]s OR EXISTS (
				SELECT 1 FROM content_tags ct2 JOIN tags t2 ON t2.id = ct2.tag_id
				WHERE ct2.content_version_id = cv.id AND t2.active AND t2.name ILIKE %[1]s))`, pattern)
		searchRank = fmt.Sprintf(`CASE WHEN cv.payload ->> 'title' ILIKE %[1]s THEN 1
			WHEN cv.payload ->> 'description' ILIKE %[1]s THEN 2 ELSE 2 END`, pattern)
	}
	appendTagFilter(&sb, &args, "cv.id", q.Tags)
	appendDateFilter(&sb, &args, "COALESCE(stats.latest_photo_at::date, al.created_at::date)", q.DateFrom, q.DateTo)

	order := listOrder(searchRank, "al.updated_at", q.Sort)
	fmt.Fprintf(&sb, " ORDER BY %s, al.id ASC LIMIT %s OFFSET %s",
		order, args.add(q.PageSize), args.add((q.Page-1)*q.PageSize))

	rows, err := r.pool.Query(ctx, sb.String(), args.all()...)
	if err != nil {
		return Page[Album]{}, fmt.Errorf("list albums: %w", err)
	}
	defer rows.Close()

	var items []Album
	total := 0
	for rows.Next() {
		album, rowTotal, err := scanAlbum(rows)
		if err != nil {
			return Page[Album]{}, err
		}
		total = rowTotal
		items = append(items, album)
	}
	if err := rows.Err(); err != nil {
		return Page[Album]{}, fmt.Errorf("iterate albums: %w", err)
	}
	return newPage(items, q.Page, q.PageSize, total), nil
}

// GetAlbum returns one published album by slug.
func (r *Repository) GetAlbum(ctx context.Context, slug string) (Album, error) {
	album, _, err := scanAlbum(r.pool.QueryRow(ctx, albumSelect+" AND al.slug = $1", slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return Album{}, ErrNotFound
	}
	if err != nil {
		return Album{}, fmt.Errorf("get album: %w", err)
	}
	return album, nil
}

func scanAlbum(row rowScanner) (Album, int, error) {
	var album Album
	var payloadRaw []byte
	var publishedAt time.Time
	var columnSlug, columnName string
	var mediaID, blurhash string
	var mediaWidth, mediaHeight int
	var variantsRaw []byte
	var tags []string
	var total int64

	err := row.Scan(&album.ID, &album.Slug, &payloadRaw, &publishedAt,
		&columnSlug, &columnName, &mediaID, &mediaWidth, &mediaHeight, &blurhash,
		&variantsRaw, &tags, &album.PhotoCount, &album.LatestPhotoAt, &total)
	if err != nil {
		return Album{}, 0, err
	}
	payload := decodePayload[albumPayload](payloadRaw)
	album.Title = payload.Title
	album.Description = payload.Description
	album.Location = payload.Location
	album.ShotAt = payload.ShotAt
	album.Column = ColumnRef{Slug: columnSlug, Name: columnName}
	album.Cover = buildMedia(mediaID, mediaWidth, mediaHeight, blurhash, variantsRaw)
	album.Tags = normalizeArray(tags)
	album.PublishedAt = publishedAt
	return album, int(total), nil
}

const photoSelect = `
SELECT p.id::text, pv.payload, pv.published_at,
       al.slug, COALESCE(av.payload ->> 'title', ''),
       pc.slug, COALESCE(pcv.payload ->> 'name', ''),
       COALESCE(tg.names, ARRAY[]::text[]),
       m.id::text, m.width, m.height, COALESCE(m.blurhash, ''),
       COALESCE(m.variants, '{}'::jsonb), COALESCE(m.exif, '{}'::jsonb),
       count(*) OVER() AS total
FROM photos p
JOIN albums al ON al.id = p.album_id
JOIN photo_columns pc ON pc.id = al.column_id
JOIN content_versions pv ON pv.id = p.published_version_id
JOIN content_versions av ON av.id = al.published_version_id
JOIN content_versions pcv ON pcv.id = pc.published_version_id
JOIN media_assets m
     ON m.id::text = NULLIF(pv.payload ->> 'mediaId', '') AND m.status = 'ready'
LEFT JOIN LATERAL (
    SELECT array_agg(t.name ORDER BY t.name) AS names
    FROM content_tags ct
    JOIN tags t ON t.id = ct.tag_id
    WHERE ct.content_version_id = pv.id AND t.active
) tg ON TRUE
WHERE p.status = 'published' AND p.deleted_at IS NULL
  AND al.status = 'published' AND al.deleted_at IS NULL AND al.published_version_id IS NOT NULL
  AND pc.status = 'published' AND pc.deleted_at IS NULL AND pc.published_version_id IS NOT NULL`

// ListPhotos lists published photos across albums, or scoped to one album/column.
func (r *Repository) ListPhotos(ctx context.Context, albumSlug, columnSlug string, q Query) (Page[Photo], error) {
	var args argList
	var sb strings.Builder
	sb.WriteString(photoSelect)

	if albumSlug != "" {
		fmt.Fprintf(&sb, " AND al.slug = %s", args.add(albumSlug))
	}
	if columnSlug != "" {
		fmt.Fprintf(&sb, " AND pc.slug = %s", args.add(columnSlug))
	}
	searchRank := ""
	if q.Search != "" {
		pattern := args.add(likePattern(q.Search))
		fmt.Fprintf(&sb, ` AND (pv.payload ->> 'title' ILIKE %[1]s OR pv.payload ->> 'description' ILIKE %[1]s
			OR pv.payload ->> 'location' ILIKE %[1]s OR EXISTS (
				SELECT 1 FROM content_tags ct2 JOIN tags t2 ON t2.id = ct2.tag_id
				WHERE ct2.content_version_id = pv.id AND t2.active AND t2.name ILIKE %[1]s))`, pattern)
		searchRank = fmt.Sprintf(`CASE WHEN pv.payload ->> 'title' ILIKE %[1]s THEN 1
			WHEN pv.payload ->> 'description' ILIKE %[1]s THEN 2 ELSE 2 END`, pattern)
	}
	appendTagFilter(&sb, &args, "pv.id", q.Tags)
	appendDateFilter(&sb, &args,
		"COALESCE(NULLIF(pv.payload ->> 'takenAt', '')::timestamptz::date, pv.published_at::date)",
		q.DateFrom, q.DateTo)

	order := listOrder(searchRank, "pv.published_at", q.Sort)
	fmt.Fprintf(&sb, " ORDER BY %s, p.id ASC LIMIT %s OFFSET %s",
		order, args.add(q.PageSize), args.add((q.Page-1)*q.PageSize))

	rows, err := r.pool.Query(ctx, sb.String(), args.all()...)
	if err != nil {
		return Page[Photo]{}, fmt.Errorf("list photos: %w", err)
	}
	defer rows.Close()

	var items []Photo
	total := 0
	for rows.Next() {
		photo, rowTotal, err := scanPhoto(rows)
		if err != nil {
			return Page[Photo]{}, err
		}
		total = rowTotal
		items = append(items, photo)
	}
	if err := rows.Err(); err != nil {
		return Page[Photo]{}, fmt.Errorf("iterate photos: %w", err)
	}
	return newPage(items, q.Page, q.PageSize, total), nil
}

// GetPhoto returns one published photo by id.
func (r *Repository) GetPhoto(ctx context.Context, id string) (Photo, error) {
	photo, _, err := scanPhoto(r.pool.QueryRow(ctx, photoSelect+" AND p.id::text = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Photo{}, ErrNotFound
	}
	if err != nil {
		return Photo{}, fmt.Errorf("get photo: %w", err)
	}
	return photo, nil
}

func scanPhoto(row rowScanner) (Photo, int, error) {
	var photo Photo
	var payloadRaw []byte
	var publishedAt time.Time
	var albumSlug, albumTitle, columnSlug, columnName string
	var tags []string
	var mediaID, blurhash string
	var mediaWidth, mediaHeight int
	var variantsRaw, exifRaw []byte
	var total int64

	err := row.Scan(&photo.ID, &payloadRaw, &publishedAt,
		&albumSlug, &albumTitle, &columnSlug, &columnName, &tags,
		&mediaID, &mediaWidth, &mediaHeight, &blurhash, &variantsRaw, &exifRaw, &total)
	if err != nil {
		return Photo{}, 0, err
	}
	payload := decodePayload[photoPayload](payloadRaw)
	photo.Title = payload.Title
	photo.Description = payload.Description
	photo.Location = payload.Location
	photo.TakenAt = payload.TakenAt
	photo.Album = ColumnRef{Slug: albumSlug, Name: albumTitle}
	photo.Column = ColumnRef{Slug: columnSlug, Name: columnName}
	photo.Tags = normalizeArray(tags)
	photo.Media = MediaAsset{
		ID:       mediaID,
		Width:    mediaWidth,
		Height:   mediaHeight,
		Blurhash: blurhash,
		Variants: decodeVariants(variantsRaw),
		Exif:     decodePayload[media.ExifInfo](exifRaw),
	}
	photo.PublishedAt = publishedAt
	return photo, int(total), nil
}

// ListTags returns the public vocabulary for one scope.
func (r *Repository) ListTags(ctx context.Context, scope string) ([]Tag, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT name FROM tags WHERE scope = $1 AND active ORDER BY name ASC
	`, scope)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()

	tags := make([]Tag, 0)
	for rows.Next() {
		var tag Tag
		if err := rows.Scan(&tag.Name); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tags: %w", err)
	}
	return tags, nil
}

// ListRecommendations returns the home page selection in position order.
// Invisible targets are skipped so unpublished content never leaks.
func (r *Repository) ListRecommendations(ctx context.Context, limit int) ([]Recommendation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT entity_type, entity_id::text FROM recommendations ORDER BY position ASC LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list recommendations: %w", err)
	}
	defer rows.Close()

	type target struct {
		kind string
		id   string
	}
	var targets []target
	for rows.Next() {
		var item target
		if err := rows.Scan(&item.kind, &item.id); err != nil {
			return nil, fmt.Errorf("scan recommendation: %w", err)
		}
		targets = append(targets, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recommendations: %w", err)
	}

	result := make([]Recommendation, 0, len(targets))
	for _, item := range targets {
		switch item.kind {
		case EntityArticle:
			article, _, err := scanArticle(r.pool.QueryRow(ctx,
				articleSelect+" AND a.id::text = $1", item.id))
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("load recommended article: %w", err)
			}
			result = append(result, Recommendation{Type: EntityArticle, Article: &article})
		case EntityAlbum:
			album, err := r.getAlbumByID(ctx, item.id)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			result = append(result, Recommendation{Type: EntityAlbum, Album: &album})
		}
	}
	return result, nil
}

func (r *Repository) getAlbumByID(ctx context.Context, id string) (Album, error) {
	album, _, err := scanAlbum(r.pool.QueryRow(ctx, albumSelect+" AND al.id::text = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Album{}, ErrNotFound
	}
	if err != nil {
		return Album{}, fmt.Errorf("get album by id: %w", err)
	}
	return album, nil
}

const searchSelect = `
WITH hits AS (
    SELECT 'article' AS kind, a.id::text AS id, a.slug,
           COALESCE(cv.payload ->> 'title', '') AS title,
           COALESCE(cv.payload ->> 'summary', '') AS summary,
           CASE WHEN cv.payload ->> 'title' ILIKE $1 THEN 1
                WHEN cv.payload ->> 'summary' ILIKE $1 THEN 2
                WHEN cv.payload ->> 'bodyIndex' ILIKE $1 THEN 3 ELSE 2 END AS rank,
           cv.published_at AS sort_at,
           COALESCE(m.id::text, '') AS media_id, COALESCE(m.width, 0) AS media_width,
           COALESCE(m.height, 0) AS media_height, COALESCE(m.blurhash, '') AS blurhash,
           COALESCE(m.variants, '{}'::jsonb) AS variants,
           c.slug AS column_slug, COALESCE(cv2.payload ->> 'name', '') AS column_name,
           ''::text AS album_slug, ''::text AS album_title
    FROM articles a
    JOIN article_columns c ON c.id = a.column_id
    JOIN content_versions cv ON cv.id = a.published_version_id
    JOIN content_versions cv2 ON cv2.id = c.published_version_id
    LEFT JOIN media_assets m ON m.id::text = NULLIF(cv.payload ->> 'coverMediaId', '') AND m.status = 'ready'
    WHERE a.status = 'published' AND a.deleted_at IS NULL
      AND c.status = 'published' AND c.deleted_at IS NULL AND c.published_version_id IS NOT NULL
      AND (cv.payload ->> 'title' ILIKE $1 OR cv.payload ->> 'summary' ILIKE $1
           OR cv.payload ->> 'bodyIndex' ILIKE $1 OR EXISTS (
               SELECT 1 FROM content_tags ct JOIN tags t ON t.id = ct.tag_id
               WHERE ct.content_version_id = cv.id AND t.active AND t.name ILIKE $1))
    UNION ALL
    SELECT 'article_column', c.id::text, c.slug,
           COALESCE(cv.payload ->> 'name', ''), COALESCE(cv.payload ->> 'description', ''),
           CASE WHEN cv.payload ->> 'name' ILIKE $1 THEN 1 ELSE 2 END,
           c.created_at,
           COALESCE(m.id::text, ''), COALESCE(m.width, 0), COALESCE(m.height, 0),
           COALESCE(m.blurhash, ''), COALESCE(m.variants, '{}'::jsonb),
           ''::text, ''::text, ''::text, ''::text
    FROM article_columns c
    JOIN content_versions cv ON cv.id = c.published_version_id
    LEFT JOIN media_assets m ON m.id::text = NULLIF(cv.payload ->> 'coverMediaId', '') AND m.status = 'ready'
    WHERE c.status = 'published' AND c.deleted_at IS NULL
      AND (cv.payload ->> 'name' ILIKE $1 OR cv.payload ->> 'description' ILIKE $1)
    UNION ALL
    SELECT 'photo_column', pc.id::text, pc.slug,
           COALESCE(cv.payload ->> 'name', ''), COALESCE(cv.payload ->> 'description', ''),
           CASE WHEN cv.payload ->> 'name' ILIKE $1 THEN 1 ELSE 2 END,
           pc.created_at,
           COALESCE(m.id::text, ''), COALESCE(m.width, 0), COALESCE(m.height, 0),
           COALESCE(m.blurhash, ''), COALESCE(m.variants, '{}'::jsonb),
           ''::text, ''::text, ''::text, ''::text
    FROM photo_columns pc
    JOIN content_versions cv ON cv.id = pc.published_version_id
    LEFT JOIN media_assets m ON m.id::text = NULLIF(cv.payload ->> 'coverMediaId', '') AND m.status = 'ready'
    WHERE pc.status = 'published' AND pc.deleted_at IS NULL
      AND (cv.payload ->> 'name' ILIKE $1 OR cv.payload ->> 'description' ILIKE $1
           OR cv.payload ->> 'theme' ILIKE $1)
    UNION ALL
    SELECT 'album', al.id::text, al.slug,
           COALESCE(cv.payload ->> 'title', ''), COALESCE(cv.payload ->> 'description', ''),
           CASE WHEN cv.payload ->> 'title' ILIKE $1 THEN 1
                WHEN cv.payload ->> 'description' ILIKE $1 THEN 2 ELSE 2 END,
           cv.published_at,
           COALESCE(m.id::text, ''), COALESCE(m.width, 0), COALESCE(m.height, 0),
           COALESCE(m.blurhash, ''), COALESCE(m.variants, '{}'::jsonb),
           pc.slug, COALESCE(pcv.payload ->> 'name', ''), ''::text, ''::text
    FROM albums al
    JOIN photo_columns pc ON pc.id = al.column_id
    JOIN content_versions cv ON cv.id = al.published_version_id
    JOIN content_versions pcv ON pcv.id = pc.published_version_id
    LEFT JOIN media_assets m ON m.id::text = NULLIF(cv.payload ->> 'coverMediaId', '') AND m.status = 'ready'
    WHERE al.status = 'published' AND al.deleted_at IS NULL
      AND pc.status = 'published' AND pc.deleted_at IS NULL AND pc.published_version_id IS NOT NULL
      AND album_has_public_photo(al.id)
      AND (cv.payload ->> 'title' ILIKE $1 OR cv.payload ->> 'description' ILIKE $1
           OR cv.payload ->> 'location' ILIKE $1 OR EXISTS (
               SELECT 1 FROM content_tags ct JOIN tags t ON t.id = ct.tag_id
               WHERE ct.content_version_id = cv.id AND t.active AND t.name ILIKE $1))
    UNION ALL
    SELECT 'photo', p.id::text, ''::text,
           COALESCE(pv.payload ->> 'title', ''), COALESCE(pv.payload ->> 'description', ''),
           CASE WHEN pv.payload ->> 'title' ILIKE $1 THEN 1
                WHEN pv.payload ->> 'description' ILIKE $1 THEN 2 ELSE 2 END,
           pv.published_at,
           m.id::text, m.width, m.height, COALESCE(m.blurhash, ''),
           COALESCE(m.variants, '{}'::jsonb),
           pc.slug, COALESCE(pcv.payload ->> 'name', ''), al.slug, COALESCE(av.payload ->> 'title', '')
    FROM photos p
    JOIN albums al ON al.id = p.album_id
    JOIN photo_columns pc ON pc.id = al.column_id
    JOIN content_versions pv ON pv.id = p.published_version_id
    JOIN content_versions av ON av.id = al.published_version_id
    JOIN content_versions pcv ON pcv.id = pc.published_version_id
    JOIN media_assets m ON m.id::text = NULLIF(pv.payload ->> 'mediaId', '') AND m.status = 'ready'
    WHERE p.status = 'published' AND p.deleted_at IS NULL
      AND al.status = 'published' AND al.deleted_at IS NULL AND al.published_version_id IS NOT NULL
      AND pc.status = 'published' AND pc.deleted_at IS NULL AND pc.published_version_id IS NOT NULL
      AND (pv.payload ->> 'title' ILIKE $1 OR pv.payload ->> 'description' ILIKE $1
           OR pv.payload ->> 'location' ILIKE $1 OR EXISTS (
               SELECT 1 FROM content_tags ct JOIN tags t ON t.id = ct.tag_id
               WHERE ct.content_version_id = pv.id AND t.active AND t.name ILIKE $1))
)
SELECT kind, id, slug, title, summary, sort_at,
       media_id, media_width, media_height, blurhash, variants,
       column_slug, column_name, album_slug, album_title,
       count(*) OVER() AS total
FROM hits
WHERE ($2::text[] IS NULL OR kind = ANY($2::text[]))
ORDER BY rank ASC, sort_at DESC, id ASC
LIMIT $3 OFFSET $4`

// Search returns merged public results. Empty types means every content type.
func (r *Repository) Search(ctx context.Context, q Query, types []string) (Page[SearchHit], error) {
	var typeFilter any
	if len(types) > 0 {
		typeFilter = types
	}
	rows, err := r.pool.Query(ctx, searchSelect,
		likePattern(q.Search), typeFilter, q.PageSize, (q.Page-1)*q.PageSize)
	if err != nil {
		return Page[SearchHit]{}, fmt.Errorf("search content: %w", err)
	}
	defer rows.Close()

	var items []SearchHit
	total := 0
	for rows.Next() {
		var hit SearchHit
		var publishedAt time.Time
		var mediaID, blurhash, columnSlug, columnName, albumSlug, albumTitle string
		var mediaWidth, mediaHeight int
		var variantsRaw []byte
		var rowTotal int64
		if err := rows.Scan(&hit.Type, &hit.ID, &hit.Slug, &hit.Title, &hit.Summary, &publishedAt,
			&mediaID, &mediaWidth, &mediaHeight, &blurhash, &variantsRaw,
			&columnSlug, &columnName, &albumSlug, &albumTitle, &rowTotal); err != nil {
			return Page[SearchHit]{}, fmt.Errorf("scan search hit: %w", err)
		}
		hit.PublishedAt = publishedAt
		hit.Cover = buildMedia(mediaID, mediaWidth, mediaHeight, blurhash, variantsRaw)
		hit.ColumnSlug = columnSlug
		hit.ColumnName = columnName
		hit.AlbumSlug = albumSlug
		hit.AlbumTitle = albumTitle
		total = int(rowTotal)
		items = append(items, hit)
	}
	if err := rows.Err(); err != nil {
		return Page[SearchHit]{}, fmt.Errorf("iterate search hits: %w", err)
	}
	return newPage(items, q.Page, q.PageSize, total), nil
}

func appendTagFilter(sb *strings.Builder, args *argList, versionColumn string, tags []string) {
	if len(tags) == 0 {
		return
	}
	placeholder := args.add(tags)
	fmt.Fprintf(sb, ` AND (SELECT count(DISTINCT t.name) FROM content_tags ct JOIN tags t ON t.id = ct.tag_id
		WHERE ct.content_version_id = %s AND t.active AND t.name = ANY(%s)) = %d`,
		versionColumn, placeholder, len(tags))
}

func appendDateFilter(sb *strings.Builder, args *argList, expression, from, to string) {
	if from != "" {
		fmt.Fprintf(sb, " AND %s >= %s::date", expression, args.add(from))
	}
	if to != "" {
		fmt.Fprintf(sb, " AND %s <= %s::date", expression, args.add(to))
	}
}

func listOrder(searchRank, timeColumn, sort string) string {
	direction := "DESC"
	if sort == SortOldest {
		direction = "ASC"
	}
	if searchRank != "" {
		return searchRank + " ASC, " + timeColumn + " " + direction
	}
	return timeColumn + " " + direction
}

func normalizeArray(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
