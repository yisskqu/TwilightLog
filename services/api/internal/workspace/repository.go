package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository owns every workspace and review query. Table names always come
// from TypeInfo, never from request input.
type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func statusExpr() string {
	return `CASE
		WHEN e.deleted_at IS NOT NULL THEN 'trashed'
		WHEN lv.status = 'pending' THEN 'pending'
		WHEN lv.status = 'rejected' THEN 'rejected'
		WHEN lv.status = 'draft' AND (e.published_version_id IS NULL OR lv.id <> e.published_version_id) THEN 'draft'
		WHEN e.status = 'published' THEN 'published'
		WHEN e.status = 'offline' THEN 'offline'
		ELSE 'draft'
	END`
}

func latestVersionJoin(info TypeInfo) string {
	return fmt.Sprintf(`LEFT JOIN LATERAL (
		SELECT cv.id, cv.status, cv.version_no, cv.payload FROM content_versions cv
		WHERE cv.entity_type = '%s' AND cv.entity_id = e.id
		ORDER BY cv.version_no DESC LIMIT 1
	) lv ON TRUE`, info.Code)
}

type parentSQL struct {
	join      string
	idExpr    string
	titleExpr string
}

func parentFor(info TypeInfo) parentSQL {
	switch info.Code {
	case TypeArticle:
		return parentSQL{
			join: `LEFT JOIN article_columns pc ON pc.id = e.column_id
				LEFT JOIN LATERAL (
					SELECT payload FROM content_versions
					WHERE entity_type = 'article_column' AND entity_id = pc.id
					ORDER BY version_no DESC LIMIT 1
				) ppc ON TRUE`,
			idExpr:    "COALESCE(pc.id::text, '')",
			titleExpr: "COALESCE(ppc.payload ->> 'name', pc.slug, '')",
		}
	case TypeAlbum:
		return parentSQL{
			join: `LEFT JOIN photo_columns pc ON pc.id = e.column_id
				LEFT JOIN LATERAL (
					SELECT payload FROM content_versions
					WHERE entity_type = 'photo_column' AND entity_id = pc.id
					ORDER BY version_no DESC LIMIT 1
				) ppc ON TRUE`,
			idExpr:    "COALESCE(pc.id::text, '')",
			titleExpr: "COALESCE(ppc.payload ->> 'name', pc.slug, '')",
		}
	case TypePhoto:
		return parentSQL{
			join: `LEFT JOIN albums pc ON pc.id = e.album_id
				LEFT JOIN LATERAL (
					SELECT payload FROM content_versions
					WHERE entity_type = 'album' AND entity_id = pc.id
					ORDER BY version_no DESC LIMIT 1
				) ppc ON TRUE`,
			idExpr:    "COALESCE(pc.id::text, '')",
			titleExpr: "COALESCE(ppc.payload ->> 'title', pc.slug, '')",
		}
	default:
		return parentSQL{idExpr: "''::text", titleExpr: "''::text"}
	}
}

func titleExpr(info TypeInfo) string {
	if info.HasSlug {
		return fmt.Sprintf(`COALESCE(
			NULLIF(lv.payload ->> '%[1]s', ''),
			NULLIF(pv.payload ->> '%[1]s', ''),
			e.slug, '未命名')`, info.TitleField)
	}
	return fmt.Sprintf(`COALESCE(
		NULLIF(lv.payload ->> '%[1]s', ''),
		NULLIF(pv.payload ->> '%[1]s', ''),
		'未命名')`, info.TitleField)
}

func slugExpr(info TypeInfo) string {
	if info.HasSlug {
		return "COALESCE(e.slug, '')"
	}
	return "''::text"
}

// listQuery builds the shared list select for one type.
func (r *Repository) list(ctx context.Context, info TypeInfo, q ListQuery) (Page[Item], error) {
	parent := parentFor(info)
	pattern := ""
	if q.Search != "" {
		pattern = likePattern(q.Search)
	}
	sql := fmt.Sprintf(`
		SELECT e.id::text, '%[1]s'::text, %[2]s,
		       (%[3]s)::text, COALESCE(lv.status, ''), COALESCE(lv.id::text, ''),
		       COALESCE(lv.version_no, 0), (e.published_version_id IS NOT NULL),
		       %[4]s, %[5]s,
		       COALESCE(e.created_by::text, ''), COALESCE(u.username, ''),
		       e.updated_at, e.deleted_at,
		       (%[6]s)::text,
		       (SELECT count(DISTINCT COALESCE(NULLIF(cv.payload ->> 'coverMediaId', ''),
		               NULLIF(cv.payload ->> 'mediaId', '')))
		          FROM content_versions cv
		         WHERE cv.entity_type = '%[1]s' AND cv.entity_id = e.id
		           AND COALESCE(NULLIF(cv.payload ->> 'coverMediaId', ''), NULLIF(cv.payload ->> 'mediaId', '')) IS NOT NULL),
		       count(*) OVER() AS total
		FROM %[7]s e
		LEFT JOIN users u ON u.id = e.created_by
		%[8]s
		LEFT JOIN content_versions pv ON pv.id = e.published_version_id
		%[9]s
		WHERE ($1 = '' OR e.created_by::text = $1)
		  AND ($2 = '' OR (%[3]s) = $2)
		  AND ($3 = '' OR (%[6]s) ILIKE $3 OR %[2]s ILIKE $3)
		ORDER BY e.updated_at DESC, e.id ASC
		LIMIT $4 OFFSET $5`,
		info.Code, slugExpr(info), statusExpr(), parent.idExpr, parent.titleExpr,
		titleExpr(info), info.Table, latestVersionJoin(info), parent.join)

	rows, err := r.pool.Query(ctx, sql, q.OwnerID, q.Status, pattern, q.PageSize, (q.Page-1)*q.PageSize)
	if err != nil {
		return Page[Item]{}, fmt.Errorf("list %s: %w", info.Code, err)
	}
	defer rows.Close()

	var items []Item
	total := 0
	for rows.Next() {
		var item Item
		var rowTotal int64
		if err := rows.Scan(&item.ID, &item.Type, &item.Slug, &item.Status, &item.WorkingStatus,
			&item.VersionID, &item.VersionNo, &item.HasPublished, &item.ParentID, &item.ParentTitle,
			&item.OwnerID, &item.OwnerName, &item.UpdatedAt, &item.TrashedAt, &item.Title,
			&item.MediaCount, &rowTotal); err != nil {
			return Page[Item]{}, fmt.Errorf("scan %s: %w", info.Code, err)
		}
		if item.TrashedAt != nil {
			item.Status = StatusTrashed
		}
		total = int(rowTotal)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return Page[Item]{}, fmt.Errorf("iterate %s: %w", info.Code, err)
	}
	return newPage(items, q.Page, q.PageSize, total), nil
}

// List returns a paginated list for one type.
func (r *Repository) List(ctx context.Context, info TypeInfo, q ListQuery) (Page[Item], error) {
	return r.list(ctx, info, q)
}

// Stats returns the status counts for one type, scoped to the owner when set.
func (r *Repository) Stats(ctx context.Context, info TypeInfo, ownerID string) (Stats, error) {
	sql := fmt.Sprintf(`
		WITH rows AS (
			SELECT (%[1]s)::text AS s
			FROM %[2]s e
			%[3]s
			WHERE ($1 = '' OR e.created_by::text = $1)
		)
		SELECT count(*),
		       count(*) FILTER (WHERE s IN ('draft', 'pending', 'rejected', 'published', 'offline')),
		       count(*) FILTER (WHERE s = 'draft'),
		       count(*) FILTER (WHERE s = 'pending'),
		       count(*) FILTER (WHERE s = 'rejected'),
		       count(*) FILTER (WHERE s = 'published'),
		       count(*) FILTER (WHERE s = 'offline'),
		       count(*) FILTER (WHERE s = 'trashed')
		FROM rows`, statusExpr(), info.Table, latestVersionJoin(info))

	stats := Stats{Type: info.Code}
	var active int
	err := r.pool.QueryRow(ctx, sql, ownerID).Scan(
		&stats.Total, &active, &stats.Draft, &stats.Pending, &stats.Rejected,
		&stats.Published, &stats.Offline, &stats.Trashed)
	if err != nil {
		return Stats{}, fmt.Errorf("stats %s: %w", info.Code, err)
	}
	return stats, nil
}

// PendingCount counts pending submissions for one type.
func (r *Repository) PendingCount(ctx context.Context, info TypeInfo, ownerID string) (int, error) {
	sql := fmt.Sprintf(`SELECT count(*) FROM content_versions cv
		WHERE cv.entity_type = $1 AND cv.status = 'pending'
		  AND ($2 = '' OR cv.created_by::text = $2)`)
	var count int
	if err := r.pool.QueryRow(ctx, sql, info.Code, ownerID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count pending %s: %w", info.Code, err)
	}
	return count, nil
}

type entityRow struct {
	ID           string
	OwnerID      string
	Status       string
	Slug         string
	DeletedAt    *time.Time
	PublishedVer string
	ParentID     string
}

func (r *Repository) loadEntity(ctx context.Context, tx pgx.Tx, info TypeInfo, id string, lock bool) (entityRow, error) {
	parent := parentFor(info)
	sql := fmt.Sprintf(`SELECT e.id::text, COALESCE(e.created_by::text, ''), e.status,
		%s, e.deleted_at, COALESCE(e.published_version_id::text, ''), %s
		FROM %s e %s WHERE e.id = $1`, slugExpr(info), parent.idExpr, info.Table, parent.join)
	if lock {
		sql += " FOR UPDATE OF e"
	}
	var row entityRow
	err := tx.QueryRow(ctx, sql, id).Scan(&row.ID, &row.OwnerID, &row.Status, &row.Slug,
		&row.DeletedAt, &row.PublishedVer, &row.ParentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return entityRow{}, ErrNotFound
	}
	if err != nil {
		return entityRow{}, fmt.Errorf("load %s: %w", info.Code, err)
	}
	return row, nil
}

// Get returns the entity metadata plus every version for the editor.
func (r *Repository) Get(ctx context.Context, info TypeInfo, id string) (Detail, error) {
	parent := parentFor(info)
	sql := fmt.Sprintf(`SELECT e.id::text, '%s'::text, %s, e.updated_at, e.deleted_at,
		%s, %s, COALESCE(e.created_by::text, ''), COALESCE(u.username, '')
		FROM %s e LEFT JOIN users u ON u.id = e.created_by %s WHERE e.id = $1`,
		info.Code, slugExpr(info), parent.idExpr, parent.titleExpr, info.Table, parent.join)
	var detail Detail
	detail.Type = info.Code
	err := r.pool.QueryRow(ctx, sql, id).Scan(&detail.ID, &detail.Type, &detail.Slug,
		&detail.UpdatedAt, &detail.TrashedAt, &detail.ParentID, &detail.ParentTitle,
		&detail.OwnerID, &detail.OwnerName)
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, fmt.Errorf("get %s: %w", info.Code, err)
	}

	rows, err := r.pool.Query(ctx, `SELECT id::text, status, version_no, payload, review_comment,
		updated_at, submitted_at, published_at
		FROM content_versions WHERE entity_type = $1 AND entity_id = $2
		ORDER BY version_no DESC`, info.Code, id)
	if err != nil {
		return Detail{}, fmt.Errorf("list versions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		version, err := scanVersion(rows)
		if err != nil {
			return Detail{}, err
		}
		if detail.Published == nil && version.Status == StatusApproved {
			copy := version
			detail.Published = &copy
		}
		if detail.Working == nil && (version.Status == StatusDraft || version.Status == StatusPending || version.Status == StatusRejected) {
			copy := version
			detail.Working = &copy
		}
	}
	if err := rows.Err(); err != nil {
		return Detail{}, fmt.Errorf("iterate versions: %w", err)
	}
	if detail.Working == nil {
		detail.Working = detail.Published
	}
	if detail.Published != nil {
		detail.HasPublished = true
		detail.VersionID = detail.Published.ID
		detail.VersionNo = detail.Published.VersionNo
	}
	if detail.Working != nil {
		detail.WorkingStatus = detail.Working.Status
		detail.VersionID = detail.Working.ID
		detail.VersionNo = detail.Working.VersionNo
		detail.Title = TitleOf(info.Code, detail.Working.Payload)
	}
	detail.Status = StatusDraft
	if detail.TrashedAt != nil {
		detail.Status = StatusTrashed
	} else if detail.Working != nil {
		switch detail.Working.Status {
		case StatusPending:
			detail.Status = StatusPending
		case StatusRejected:
			detail.Status = StatusRejected
		case StatusApproved:
			detail.Status = StatusPublished
		}
	}
	return detail, nil
}

func scanVersion(row rowScanner) (Version, error) {
	var version Version
	var payloadRaw []byte
	if err := row.Scan(&version.ID, &version.Status, &version.VersionNo, &payloadRaw,
		&version.Comment, &version.UpdatedAt, &version.SubmittedAt, &version.PublishedAt); err != nil {
		return Version{}, fmt.Errorf("scan version: %w", err)
	}
	version.Payload = map[string]any{}
	_ = json.Unmarshal(payloadRaw, &version.Payload)
	return version, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

// Parent returns the parent entity, used for ownership and recycle checks.
func (r *Repository) Parent(ctx context.Context, info TypeInfo, parentID string) (entityRow, error) {
	if info.ParentType == "" {
		return entityRow{}, nil
	}
	parentInfo, err := TypeInfoFor(info.ParentType)
	if err != nil {
		return entityRow{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return entityRow{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row, err := r.loadEntity(ctx, tx, parentInfo, parentID, false)
	if err != nil {
		return entityRow{}, err
	}
	if row.DeletedAt != nil {
		return entityRow{}, fmt.Errorf("%w: 上级内容在回收站中", ErrConflict)
	}
	return row, nil
}

// Create inserts the entity and its first draft version.
func (r *Repository) Create(ctx context.Context, info TypeInfo, ownerID, slug, parentID string, payload map[string]any, tags []string) (string, error) {
	payloadRaw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var entityID string
	switch info.Code {
	case TypeArticleColumn, TypePhotoColumn:
		err = tx.QueryRow(ctx, fmt.Sprintf(`INSERT INTO %s (slug, status, created_by)
			VALUES ($1, 'draft', $2) RETURNING id::text`, info.Table), slug, ownerID).Scan(&entityID)
	case TypeArticle, TypeAlbum:
		column := "column_id"
		err = tx.QueryRow(ctx, fmt.Sprintf(`INSERT INTO %s (%s, slug, status, created_by)
			VALUES ($1, $2, 'draft', $3) RETURNING id::text`, info.Table, column), parentID, slug, ownerID).Scan(&entityID)
	case TypePhoto:
		err = tx.QueryRow(ctx, `INSERT INTO photos (album_id, status, sort_order, created_by)
			VALUES ($1, 'draft', COALESCE((SELECT max(sort_order) + 1 FROM photos WHERE album_id = $1), 0), $2)
			RETURNING id::text`, parentID, ownerID).Scan(&entityID)
	}
	if err != nil {
		return "", classifyWriteError(err)
	}

	versionID, err := insertVersion(ctx, tx, info.Code, entityID, ownerID, 1, payloadRaw, tags, info.Scope)
	if err != nil {
		return "", err
	}
	_ = versionID
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return entityID, nil
}

func insertVersion(ctx context.Context, tx pgx.Tx, code, entityID, ownerID string, versionNo int, payload []byte, tags []string, scope string) (string, error) {
	var versionID string
	err := tx.QueryRow(ctx, `INSERT INTO content_versions
		(entity_type, entity_id, version_no, status, payload, created_by)
		VALUES ($1, $2, $3, 'draft', $4::jsonb, $5) RETURNING id::text`,
		code, entityID, versionNo, string(payload), ownerID).Scan(&versionID)
	if err != nil {
		return "", classifyWriteError(err)
	}
	if err := syncTags(ctx, tx, versionID, scope, tags); err != nil {
		return "", err
	}
	return versionID, nil
}

func syncTags(ctx context.Context, tx pgx.Tx, versionID, scope string, tags []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM content_tags WHERE content_version_id = $1`, versionID); err != nil {
		return fmt.Errorf("clear tags: %w", err)
	}
	for _, name := range tags {
		var tagID string
		if err := tx.QueryRow(ctx, `INSERT INTO tags (scope, name) VALUES ($1, $2)
			ON CONFLICT (scope, name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id::text`, scope, name).Scan(&tagID); err != nil {
			return fmt.Errorf("upsert tag: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO content_tags (content_version_id, tag_id)
			VALUES ($1, $2) ON CONFLICT DO NOTHING`, versionID, tagID); err != nil {
			return fmt.Errorf("link tag: %w", err)
		}
	}
	return nil
}

// Owner returns the owner id of one entity.
func (r *Repository) Owner(ctx context.Context, info TypeInfo, id string) (string, error) {
	var owner string
	err := r.pool.QueryRow(ctx, fmt.Sprintf(`SELECT COALESCE(created_by::text, '') FROM %s WHERE id = $1`, info.Table), id).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("load owner: %w", err)
	}
	return owner, nil
}

// Save writes the working draft and returns its version id.
func (r *Repository) Save(ctx context.Context, info TypeInfo, entityID, actorID, slug string, payload map[string]any, tags []string) (string, error) {
	payloadRaw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	entity, err := r.loadEntity(ctx, tx, info, entityID, true)
	if err != nil {
		return "", err
	}
	if entity.DeletedAt != nil {
		return "", fmt.Errorf("%w: 回收站中的内容需先恢复", ErrConflict)
	}

	var latestID, latestStatus, latestOwner string
	var latestNo int
	err = tx.QueryRow(ctx, `SELECT id::text, status, COALESCE(created_by::text, ''), version_no
		FROM content_versions WHERE entity_type = $1 AND entity_id = $2
		ORDER BY version_no DESC LIMIT 1`, info.Code, entityID).Scan(&latestID, &latestStatus, &latestOwner, &latestNo)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("load latest version: %w", err)
	}
	if latestStatus == StatusPending {
		return "", fmt.Errorf("%w: 有待审版本，请先撤回再编辑", ErrConflict)
	}

	var versionID string
	if latestStatus == StatusDraft && latestOwner == actorID {
		_, err = tx.Exec(ctx, `UPDATE content_versions SET payload = $2::jsonb, updated_at = now() WHERE id = $1`,
			latestID, string(payloadRaw))
		if err != nil {
			return "", fmt.Errorf("update draft: %w", err)
		}
		versionID = latestID
		if err := syncTags(ctx, tx, versionID, info.Scope, tags); err != nil {
			return "", err
		}
	} else {
		versionID, err = insertVersion(ctx, tx, info.Code, entityID, actorID, latestNo+1, payloadRaw, tags, info.Scope)
		if err != nil {
			return "", err
		}
	}

	if info.HasSlug && strings.TrimSpace(slug) != "" && slug != entity.Slug {
		if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET slug = $2, updated_at = now() WHERE id = $1`, info.Table),
			entityID, slug); err != nil {
			return "", classifyWriteError(err)
		}
	} else {
		if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET updated_at = now() WHERE id = $1`, info.Table), entityID); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return versionID, nil
}

// Submit moves the latest draft to the review queue.
func (r *Repository) Submit(ctx context.Context, info TypeInfo, entityID, actorID string) (Version, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Version{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	entity, err := r.loadEntity(ctx, tx, info, entityID, true)
	if err != nil {
		return Version{}, err
	}
	if entity.DeletedAt != nil {
		return Version{}, fmt.Errorf("%w: 回收站中的内容不能提交", ErrConflict)
	}

	var versionID, status string
	err = tx.QueryRow(ctx, `SELECT id::text, status FROM content_versions
		WHERE entity_type = $1 AND entity_id = $2 ORDER BY version_no DESC LIMIT 1 FOR UPDATE`,
		info.Code, entityID).Scan(&versionID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, fmt.Errorf("%w: 没有可提交的草稿", ErrConflict)
	}
	if err != nil {
		return Version{}, fmt.Errorf("load draft: %w", err)
	}
	if status == StatusPending {
		return Version{}, fmt.Errorf("%w: 该内容已在审批中", ErrConflict)
	}
	if status == StatusApproved {
		return Version{}, fmt.Errorf("%w: 没有新的修改可提交", ErrConflict)
	}
	if _, err := tx.Exec(ctx, `UPDATE content_versions SET status = 'pending', submitted_at = now(),
		updated_at = now(), review_comment = '' WHERE id = $1`, versionID); err != nil {
		return Version{}, fmt.Errorf("submit version: %w", err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET updated_at = now() WHERE id = $1`, info.Table), entityID); err != nil {
		return Version{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Version{}, err
	}
	return r.versionByID(ctx, versionID)
}

// Withdraw moves a pending submission back to draft.
func (r *Repository) Withdraw(ctx context.Context, info TypeInfo, entityID string) (Version, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Version{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var versionID, status string
	err = tx.QueryRow(ctx, `SELECT id::text, status FROM content_versions
		WHERE entity_type = $1 AND entity_id = $2 ORDER BY version_no DESC LIMIT 1 FOR UPDATE`,
		info.Code, entityID).Scan(&versionID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, fmt.Errorf("%w: 没有待审版本", ErrConflict)
	}
	if err != nil {
		return Version{}, fmt.Errorf("load pending version: %w", err)
	}
	if status != StatusPending {
		return Version{}, fmt.Errorf("%w: 只有待审版本可以撤回", ErrConflict)
	}
	if _, err := tx.Exec(ctx, `UPDATE content_versions SET status = 'draft', withdrawn_at = now(),
		updated_at = now() WHERE id = $1`, versionID); err != nil {
		return Version{}, fmt.Errorf("withdraw version: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Version{}, err
	}
	return r.versionByID(ctx, versionID)
}

func (r *Repository) versionByID(ctx context.Context, versionID string) (Version, error) {
	row := r.pool.QueryRow(ctx, `SELECT id::text, status, version_no, payload, review_comment,
		updated_at, submitted_at, published_at FROM content_versions WHERE id = $1`, versionID)
	return scanVersion(row)
}

// Pending lists the review queue. When code is empty every type is merged.
func (r *Repository) Pending(ctx context.Context, code, search string) ([]ReviewItem, error) {
	codes := TypeCodes()
	if code != "" {
		codes = []string{code}
	}
	pattern := ""
	if search != "" {
		pattern = likePattern(search)
	}
	var result []ReviewItem
	for _, itemCode := range codes {
		info, err := TypeInfoFor(itemCode)
		if err != nil {
			return nil, err
		}
		sql := fmt.Sprintf(`SELECT cv.id::text, cv.entity_type, cv.entity_id::text, cv.version_no, cv.status,
			COALESCE(cv.payload ->> '%[2]s', ''), COALESCE(cv.created_by::text, ''), COALESCE(u.username, ''),
			cv.submitted_at, cv.updated_at, (e.published_version_id IS NOT NULL), cv.payload
			FROM content_versions cv
			JOIN users u ON u.id = cv.created_by
			JOIN %[1]s e ON e.id = cv.entity_id
			WHERE cv.entity_type = '%[3]s' AND cv.status = 'pending' AND e.deleted_at IS NULL
			  AND ($1 = '' OR (cv.payload ->> '%[2]s') ILIKE $1)
			ORDER BY cv.submitted_at ASC NULLS LAST, cv.updated_at ASC`, info.Table, info.TitleField, info.Code)
		rows, err := r.pool.Query(ctx, sql, pattern)
		if err != nil {
			return nil, fmt.Errorf("list pending %s: %w", itemCode, err)
		}
		for rows.Next() {
			var item ReviewItem
			var payloadRaw []byte
			if err := rows.Scan(&item.VersionID, &item.Type, &item.EntityID, &item.VersionNo, &item.Status,
				&item.Title, &item.AuthorID, &item.AuthorName, &item.SubmittedAt, &item.UpdatedAt,
				&item.HasPublished, &payloadRaw); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan pending: %w", err)
			}
			item.Payload = map[string]any{}
			_ = json.Unmarshal(payloadRaw, &item.Payload)
			result = append(result, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("iterate pending: %w", err)
		}
		rows.Close()
	}
	return result, nil
}

// ReviewDetail loads one submission for the approval center.
func (r *Repository) ReviewDetail(ctx context.Context, versionID string) (ReviewDetail, error) {
	var detail ReviewDetail
	var payloadRaw []byte
	err := r.pool.QueryRow(ctx, `SELECT cv.id::text, cv.entity_type, cv.entity_id::text, cv.version_no, cv.status,
		COALESCE(cv.payload ->> 'title', cv.payload ->> 'name', ''), COALESCE(cv.created_by::text, ''),
		COALESCE(u.username, ''), cv.submitted_at, cv.updated_at, cv.payload
		FROM content_versions cv LEFT JOIN users u ON u.id = cv.created_by
		WHERE cv.id = $1`, versionID).Scan(&detail.VersionID, &detail.Type, &detail.EntityID,
		&detail.VersionNo, &detail.Status, &detail.Title, &detail.AuthorID, &detail.AuthorName,
		&detail.SubmittedAt, &detail.UpdatedAt, &payloadRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewDetail{}, ErrNotFound
	}
	if err != nil {
		return ReviewDetail{}, fmt.Errorf("load review: %w", err)
	}
	detail.Payload = map[string]any{}
	_ = json.Unmarshal(payloadRaw, &detail.Payload)

	info, err := TypeInfoFor(detail.Type)
	if err != nil {
		return ReviewDetail{}, err
	}
	var publishedRaw []byte
	var publishedAt *time.Time
	err = r.pool.QueryRow(ctx, fmt.Sprintf(`SELECT COALESCE(pv.payload, '{}'::jsonb), pv.published_at
		FROM %s e LEFT JOIN content_versions pv ON pv.id = e.published_version_id
		WHERE e.id = $1`, info.Table), detail.EntityID).Scan(&publishedRaw, &publishedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ReviewDetail{}, fmt.Errorf("load published version: %w", err)
	}
	detail.Published = map[string]any{}
	_ = json.Unmarshal(publishedRaw, &detail.Published)
	detail.PublishedAt = publishedAt
	detail.HasPublished = publishedAt != nil

	rows, err := r.pool.Query(ctx, `SELECT t.name FROM content_tags ct
		JOIN tags t ON t.id = ct.tag_id WHERE ct.content_version_id = $1 ORDER BY t.name`, versionID)
	if err != nil {
		return ReviewDetail{}, fmt.Errorf("load review tags: %w", err)
	}
	defer rows.Close()
	detail.Tags = []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return ReviewDetail{}, err
		}
		detail.Tags = append(detail.Tags, name)
	}
	return detail, rows.Err()
}

// Approve publishes the pending version and returns the removed tag names.
func (r *Repository) Approve(ctx context.Context, versionID, reviewerID, comment string, removeTags []string) (ReviewDetail, []string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ReviewDetail{}, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	detail, info, err := loadReviewForUpdate(ctx, tx, versionID)
	if err != nil {
		return ReviewDetail{}, nil, err
	}
	if detail.Status != StatusPending {
		return ReviewDetail{}, nil, fmt.Errorf("%w: 只有待审版本可以通过", ErrConflict)
	}
	entity, err := r.loadEntity(ctx, tx, info, detail.EntityID, true)
	if err != nil {
		return ReviewDetail{}, nil, err
	}
	if entity.DeletedAt != nil {
		return ReviewDetail{}, nil, fmt.Errorf("%w: 内容在回收站中，不能通过", ErrConflict)
	}
	detail.HasPublished = entity.PublishedVer != ""

	removed := []string{}
	if len(removeTags) > 0 {
		rows, err := tx.Query(ctx, `DELETE FROM content_tags ct
			USING tags t
			WHERE ct.tag_id = t.id AND ct.content_version_id = $1 AND t.name = ANY($2)
			RETURNING t.name`, versionID, removeTags)
		if err != nil {
			return ReviewDetail{}, nil, fmt.Errorf("remove tags: %w", err)
		}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				rows.Close()
				return ReviewDetail{}, nil, err
			}
			removed = append(removed, name)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return ReviewDetail{}, nil, err
		}
		rows.Close()
		if _, err := tx.Exec(ctx, `UPDATE content_versions SET payload = jsonb_set(payload, '{tags}',
			COALESCE((SELECT jsonb_agg(t.name ORDER BY t.name) FROM content_tags ct JOIN tags t ON t.id = ct.tag_id
				WHERE ct.content_version_id = $1), '[]'::jsonb), true) WHERE id = $1`, versionID); err != nil {
			return ReviewDetail{}, nil, fmt.Errorf("sync payload tags: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE content_versions SET status = 'approved', published_at = now(),
		reviewed_by = $2, reviewed_at = now(), review_comment = $3, updated_at = now() WHERE id = $1`,
		versionID, reviewerID, strings.TrimSpace(comment)); err != nil {
		return ReviewDetail{}, nil, fmt.Errorf("approve version: %w", err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET published_version_id = $2, status = 'published',
		deleted_at = NULL, updated_at = now() WHERE id = $1`, info.Table), detail.EntityID, versionID); err != nil {
		return ReviewDetail{}, nil, fmt.Errorf("publish entity: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO reviews (entity_type, entity_id, version_id, reviewer_id, decision, comment)
		VALUES ($1, $2, $3, $4, 'approved', $5)`, detail.Type, detail.EntityID, versionID, reviewerID, strings.TrimSpace(comment)); err != nil {
		return ReviewDetail{}, nil, fmt.Errorf("record review: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ReviewDetail{}, nil, err
	}
	detail.Status = StatusApproved
	return detail, removed, nil
}

// Reject marks the submission as rejected and returns it.
func (r *Repository) Reject(ctx context.Context, versionID, reviewerID, comment string) (ReviewDetail, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ReviewDetail{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	detail, _, err := loadReviewForUpdate(ctx, tx, versionID)
	if err != nil {
		return ReviewDetail{}, err
	}
	if detail.Status != StatusPending {
		return ReviewDetail{}, fmt.Errorf("%w: 只有待审版本可以驳回", ErrConflict)
	}
	if _, err := tx.Exec(ctx, `UPDATE content_versions SET status = 'rejected', reviewed_by = $2,
		reviewed_at = now(), review_comment = $3, updated_at = now() WHERE id = $1`,
		versionID, reviewerID, strings.TrimSpace(comment)); err != nil {
		return ReviewDetail{}, fmt.Errorf("reject version: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO reviews (entity_type, entity_id, version_id, reviewer_id, decision, comment)
		VALUES ($1, $2, $3, $4, 'rejected', $5)`, detail.Type, detail.EntityID, versionID, reviewerID, strings.TrimSpace(comment)); err != nil {
		return ReviewDetail{}, fmt.Errorf("record review: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ReviewDetail{}, err
	}
	detail.Status = StatusRejected
	return detail, nil
}

func loadReviewForUpdate(ctx context.Context, tx pgx.Tx, versionID string) (ReviewDetail, TypeInfo, error) {
	var detail ReviewDetail
	var payloadRaw []byte
	err := tx.QueryRow(ctx, `SELECT cv.id::text, cv.entity_type, cv.entity_id::text, cv.version_no, cv.status,
		COALESCE(cv.payload ->> 'title', cv.payload ->> 'name', ''), COALESCE(cv.created_by::text, ''),
		COALESCE(u.username, ''), cv.submitted_at, cv.updated_at, cv.payload
		FROM content_versions cv LEFT JOIN users u ON u.id = cv.created_by
		WHERE cv.id = $1 FOR UPDATE OF cv`, versionID).Scan(&detail.VersionID, &detail.Type, &detail.EntityID,
		&detail.VersionNo, &detail.Status, &detail.Title, &detail.AuthorID, &detail.AuthorName,
		&detail.SubmittedAt, &detail.UpdatedAt, &payloadRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewDetail{}, TypeInfo{}, ErrNotFound
	}
	if err != nil {
		return ReviewDetail{}, TypeInfo{}, fmt.Errorf("lock review version: %w", err)
	}
	detail.Payload = map[string]any{}
	_ = json.Unmarshal(payloadRaw, &detail.Payload)
	info, err := TypeInfoFor(detail.Type)
	if err != nil {
		return ReviewDetail{}, TypeInfo{}, err
	}
	return detail, info, nil
}

// Trash moves one entity to the recycle bin after checking for children.
func (r *Repository) Trash(ctx context.Context, info TypeInfo, entityID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	entity, err := r.loadEntity(ctx, tx, info, entityID, true)
	if err != nil {
		return err
	}
	if entity.DeletedAt != nil {
		return fmt.Errorf("%w: 内容已在回收站", ErrConflict)
	}
	childTable, childColumn, err := childOf(info)
	if err != nil {
		return err
	}
	if childTable != "" {
		var count int
		if err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s = $1`, childTable, childColumn), entityID).Scan(&count); err != nil {
			return fmt.Errorf("count children: %w", err)
		}
		if count > 0 {
			return fmt.Errorf("%w: 请先处理下级内容，再移入回收站", ErrConflict)
		}
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET status = 'trashed', deleted_at = now(),
		updated_at = now() WHERE id = $1`, info.Table), entityID); err != nil {
		return fmt.Errorf("trash entity: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE content_versions SET status = 'draft', withdrawn_at = now(), updated_at = now()
		WHERE entity_type = $1 AND entity_id = $2 AND status = 'pending'`, info.Code, entityID); err != nil {
		return fmt.Errorf("withdraw trashed pending: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM recommendations WHERE entity_type = $1 AND entity_id = $2`, info.Code, entityID); err != nil {
		return fmt.Errorf("drop recommendation: %w", err)
	}
	return tx.Commit(ctx)
}

func childOf(info TypeInfo) (string, string, error) {
	switch info.Code {
	case TypeArticleColumn:
		return "articles", "column_id", nil
	case TypePhotoColumn:
		return "albums", "column_id", nil
	case TypeAlbum:
		return "photos", "album_id", nil
	default:
		return "", "", nil
	}
}

// Restore returns an entity to draft or published after checking its parent.
func (r *Repository) Restore(ctx context.Context, info TypeInfo, entityID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	entity, err := r.loadEntity(ctx, tx, info, entityID, true)
	if err != nil {
		return err
	}
	if entity.DeletedAt == nil {
		return fmt.Errorf("%w: 内容不在回收站", ErrConflict)
	}
	if info.ParentType != "" {
		parentInfo, err := TypeInfoFor(info.ParentType)
		if err != nil {
			return err
		}
		parent, err := r.loadEntity(ctx, tx, parentInfo, entity.ParentID, false)
		if err != nil {
			return fmt.Errorf("%w: 上级内容不存在，无法恢复", ErrConflict)
		}
		if parent.DeletedAt != nil || parent.Status == StatusTrashed {
			return fmt.Errorf("%w: 请先恢复上级内容", ErrConflict)
		}
	}
	status := StatusDraft
	if entity.PublishedVer != "" {
		status = StatusPublished
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET status = $2, deleted_at = NULL, updated_at = now()
		WHERE id = $1`, info.Table), entityID, status); err != nil {
		return fmt.Errorf("restore entity: %w", err)
	}
	return tx.Commit(ctx)
}

// PermanentDelete removes one trashed entity and returns its media ids.
func (r *Repository) PermanentDelete(ctx context.Context, info TypeInfo, entityID string) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	entity, err := r.loadEntity(ctx, tx, info, entityID, true)
	if err != nil {
		return nil, err
	}
	if entity.DeletedAt == nil {
		return nil, fmt.Errorf("%w: 只有回收站中的内容可以永久删除", ErrConflict)
	}
	childTable, childColumn, err := childOf(info)
	if err != nil {
		return nil, err
	}
	if childTable != "" {
		var count int
		if err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s = $1`, childTable, childColumn), entityID).Scan(&count); err != nil {
			return nil, fmt.Errorf("count children: %w", err)
		}
		if count > 0 {
			return nil, fmt.Errorf("%w: 请先永久删除下级内容", ErrConflict)
		}
	}

	rows, err := tx.Query(ctx, `SELECT DISTINCT COALESCE(NULLIF(payload ->> 'coverMediaId', ''), NULLIF(payload ->> 'mediaId', ''))
		FROM content_versions WHERE entity_type = $1 AND entity_id = $2
		  AND COALESCE(NULLIF(payload ->> 'coverMediaId', ''), NULLIF(payload ->> 'mediaId', '')) IS NOT NULL`,
		info.Code, entityID)
	if err != nil {
		return nil, fmt.Errorf("collect media: %w", err)
	}
	var mediaIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		mediaIDs = append(mediaIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM content_versions WHERE entity_type = $1 AND entity_id = $2`, info.Code, entityID); err != nil {
		return nil, fmt.Errorf("delete versions: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM recommendations WHERE entity_type = $1 AND entity_id = $2`, info.Code, entityID); err != nil {
		return nil, fmt.Errorf("delete recommendation: %w", err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, info.Table), entityID); err != nil {
		return nil, fmt.Errorf("delete entity: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return mediaIDs, nil
}

// MediaStillReferenced reports whether another content version uses the media.
func (r *Repository) MediaStillReferenced(ctx context.Context, mediaID, exceptType, exceptID string) (bool, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM content_versions
		WHERE NOT (entity_type = $1 AND entity_id = $2)
		  AND (payload ->> 'coverMediaId' = $3 OR payload ->> 'mediaId' = $3)`,
		exceptType, exceptID, mediaID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("count media references: %w", err)
	}
	return count > 0, nil
}

// MediaInfo validates an uploaded asset before it is attached to content.
func (r *Repository) MediaInfo(ctx context.Context, mediaID string) (ownerID, purpose, status string, err error) {
	err = r.pool.QueryRow(ctx, `SELECT COALESCE(owner_id::text, ''), purpose, status
		FROM media_assets WHERE id::text = $1`, mediaID).Scan(&ownerID, &purpose, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", fmt.Errorf("%w: 图片不存在", ErrInvalidInput)
	}
	if err != nil {
		return "", "", "", fmt.Errorf("load media: %w", err)
	}
	return ownerID, purpose, status, nil
}

// NotifyRole inserts one notification for every active user of the roles.
func (r *Repository) NotifyRole(ctx context.Context, roles []string, kind, severity, title, body, link string) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO notifications (recipient_id, type, severity, title, body, link)
		SELECT u.id, $1, $2, $3, $4, $5 FROM users u
		JOIN roles r ON r.id = u.role_id
		WHERE r.code = ANY($6) AND u.status = 'active'`, kind, severity, title, body, link, roles)
	if err != nil {
		return fmt.Errorf("notify roles: %w", err)
	}
	return nil
}

// NotifyUser inserts one notification for a specific user.
func (r *Repository) NotifyUser(ctx context.Context, userID, kind, severity, title, body, link string) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO notifications (recipient_id, type, severity, title, body, link)
		VALUES ($1, $2, $3, $4, $5, $6)`, userID, kind, severity, title, body, link)
	if err != nil {
		return fmt.Errorf("notify user: %w", err)
	}
	return nil
}

// ListNotifications returns one page and the unread badge count.
func (r *Repository) ListNotifications(ctx context.Context, userID string, page, pageSize int) (NotificationPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 12
	}
	if pageSize > 50 {
		pageSize = 50
	}
	var total, unread int
	if err := r.pool.QueryRow(ctx, `SELECT count(*),
		count(*) FILTER (WHERE read_at IS NULL) FROM notifications WHERE recipient_id = $1`, userID).Scan(&total, &unread); err != nil {
		return NotificationPage{}, fmt.Errorf("count notifications: %w", err)
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text, type, severity, title, body, link, read_at, created_at
		FROM notifications WHERE recipient_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`,
		userID, pageSize, (page-1)*pageSize)
	if err != nil {
		return NotificationPage{}, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()
	items := make([]Notification, 0, pageSize)
	for rows.Next() {
		var item Notification
		if err := rows.Scan(&item.ID, &item.Type, &item.Severity, &item.Title, &item.Body,
			&item.Link, &item.ReadAt, &item.CreatedAt); err != nil {
			return NotificationPage{}, fmt.Errorf("scan notification: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return NotificationPage{}, err
	}
	return NotificationPage{Items: items, Page: page, PageSize: pageSize, Total: total, Unread: unread}, nil
}

// MarkNotificationRead marks one message read.
func (r *Repository) MarkNotificationRead(ctx context.Context, userID string, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE notifications SET read_at = now()
		WHERE id = $1 AND recipient_id = $2 AND read_at IS NULL`, id, userID)
	return err
}

// MarkAllNotificationsRead clears the unread badge.
func (r *Repository) MarkAllNotificationsRead(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE notifications SET read_at = now()
		WHERE recipient_id = $1 AND read_at IS NULL`, userID)
	return err
}

// UnreadNotifications counts unread messages for the bell badge.
func (r *Repository) UnreadNotifications(ctx context.Context, userID string) (int, error) {
	var count int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE recipient_id = $1 AND read_at IS NULL`, userID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// ListTags returns the tag library with usage counts.
func (r *Repository) ListTags(ctx context.Context, scope string) ([]TagUsage, error) {
	rows, err := r.pool.Query(ctx, `SELECT t.id::text, t.scope, t.name, t.active,
		count(ct.content_version_id)::int
		FROM tags t LEFT JOIN content_tags ct ON ct.tag_id = t.id
		WHERE t.scope = $1
		GROUP BY t.id ORDER BY t.name ASC`, scope)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()
	items := make([]TagUsage, 0)
	for rows.Next() {
		var item TagUsage
		if err := rows.Scan(&item.ID, &item.Scope, &item.Name, &item.Active, &item.Usage); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// RenameTag updates one tag name inside its scope.
func (r *Repository) RenameTag(ctx context.Context, id, name string) error {
	var scope string
	if err := r.pool.QueryRow(ctx, `SELECT scope FROM tags WHERE id = $1`, id).Scan(&scope); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	_, err := r.pool.Exec(ctx, `UPDATE tags SET name = $2 WHERE id = $1`, id, name)
	return classifyWriteError(err)
}

// MergeTag moves every link from source to target inside the same scope.
func (r *Repository) MergeTag(ctx context.Context, sourceID, targetID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var sourceScope, targetScope string
	if err := tx.QueryRow(ctx, `SELECT scope FROM tags WHERE id = $1`, sourceID).Scan(&sourceScope); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if err := tx.QueryRow(ctx, `SELECT scope FROM tags WHERE id = $1`, targetID).Scan(&targetScope); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if sourceScope != targetScope {
		return fmt.Errorf("%w: 不同作用域的标签不能合并", ErrInvalidInput)
	}
	if sourceID == targetID {
		return fmt.Errorf("%w: 请选择不同的目标标签", ErrInvalidInput)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO content_tags (content_version_id, tag_id)
		SELECT content_version_id, $2 FROM content_tags WHERE tag_id = $1 ON CONFLICT DO NOTHING`, sourceID, targetID); err != nil {
		return fmt.Errorf("move tag links: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM content_tags WHERE tag_id = $1`, sourceID); err != nil {
		return fmt.Errorf("delete source links: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM tags WHERE id = $1`, sourceID); err != nil {
		return fmt.Errorf("delete source tag: %w", err)
	}
	return tx.Commit(ctx)
}

// SetTagActive enables or disables one tag.
func (r *Repository) SetTagActive(ctx context.Context, id string, active bool) error {
	result, err := r.pool.Exec(ctx, `UPDATE tags SET active = $2 WHERE id = $1`, id, active)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RecommendationSettings reads the counts and the configured slots.
func (r *Repository) RecommendationSettings(ctx context.Context) (RecommendationSettings, error) {
	var settings RecommendationSettings
	if err := r.pool.QueryRow(ctx, `SELECT recommendation_n1, recommendation_n2, recommendation_auto_fill
		FROM system_info WHERE id = 1`).Scan(&settings.N1, &settings.N2, &settings.AutoFill); err != nil {
		return RecommendationSettings{}, fmt.Errorf("load recommendation settings: %w", err)
	}
	rows, err := r.pool.Query(ctx, `SELECT r.position, r.entity_type, r.entity_id::text,
		COALESCE(cv.payload ->> 'title', cv.payload ->> 'name', '')
		FROM recommendations r
		LEFT JOIN LATERAL (
			SELECT payload FROM content_versions cv
			WHERE cv.entity_type = r.entity_type AND cv.entity_id = r.entity_id
			ORDER BY cv.version_no DESC LIMIT 1
		) cv ON TRUE
		ORDER BY r.position ASC`)
	if err != nil {
		return RecommendationSettings{}, fmt.Errorf("list recommendations: %w", err)
	}
	defer rows.Close()
	settings.Items = make([]RecommendationItem, 0)
	for rows.Next() {
		var item RecommendationItem
		if err := rows.Scan(&item.Position, &item.EntityType, &item.EntityID, &item.Title); err != nil {
			return RecommendationSettings{}, err
		}
		settings.Items = append(settings.Items, item)
	}
	return settings, rows.Err()
}

// SaveRecommendationSettings replaces the slots in one transaction.
func (r *Repository) SaveRecommendationSettings(ctx context.Context, settings RecommendationSettings) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `UPDATE system_info SET recommendation_n1 = $1, recommendation_n2 = $2,
		recommendation_auto_fill = $3, updated_at = now() WHERE id = 1`,
		settings.N1, settings.N2, settings.AutoFill); err != nil {
		return fmt.Errorf("update settings: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM recommendations`); err != nil {
		return fmt.Errorf("clear recommendations: %w", err)
	}
	for index, item := range settings.Items {
		if _, err := tx.Exec(ctx, `INSERT INTO recommendations (entity_type, entity_id, position)
			VALUES ($1, $2, $3)`, item.EntityType, item.EntityID, index+1); err != nil {
			return fmt.Errorf("insert recommendation: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// PublicTarget reports whether the target is currently visible to the public.
func (r *Repository) PublicTarget(ctx context.Context, entityType, entityID string) (string, bool, error) {
	switch entityType {
	case TypeArticle:
		var title string
		err := r.pool.QueryRow(ctx, `SELECT COALESCE(cv.payload ->> 'title', '')
			FROM articles a
			JOIN article_columns c ON c.id = a.column_id
			JOIN content_versions cv ON cv.id = a.published_version_id
			WHERE a.id = $1 AND a.status = 'published' AND a.deleted_at IS NULL
			  AND c.status = 'published' AND c.deleted_at IS NULL`, entityID).Scan(&title)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		if err != nil {
			return "", false, fmt.Errorf("check article: %w", err)
		}
		return title, true, nil
	case TypeAlbum:
		var title string
		err := r.pool.QueryRow(ctx, `SELECT COALESCE(cv.payload ->> 'title', '')
			FROM albums al
			JOIN photo_columns pc ON pc.id = al.column_id
			JOIN content_versions cv ON cv.id = al.published_version_id
			WHERE al.id = $1 AND al.status = 'published' AND al.deleted_at IS NULL
			  AND pc.status = 'published' AND pc.deleted_at IS NULL AND album_has_public_photo(al.id)`, entityID).Scan(&title)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		if err != nil {
			return "", false, fmt.Errorf("check album: %w", err)
		}
		return title, true, nil
	default:
		return "", false, nil
	}
}

// Candidates lists currently public articles or albums.
func (r *Repository) Candidates(ctx context.Context, entityType string) ([]Candidate, error) {
	var sql string
	switch entityType {
	case TypeArticle:
		sql = `SELECT 'article', a.id::text, COALESCE(cv.payload ->> 'title', ''), a.updated_at
			FROM articles a
			JOIN article_columns c ON c.id = a.column_id
			JOIN content_versions cv ON cv.id = a.published_version_id
			WHERE a.status = 'published' AND a.deleted_at IS NULL
			  AND c.status = 'published' AND c.deleted_at IS NULL
			ORDER BY a.updated_at DESC LIMIT 100`
	case TypeAlbum:
		sql = `SELECT 'album', al.id::text, COALESCE(cv.payload ->> 'title', ''), al.updated_at
			FROM albums al
			JOIN photo_columns pc ON pc.id = al.column_id
			JOIN content_versions cv ON cv.id = al.published_version_id
			WHERE al.status = 'published' AND al.deleted_at IS NULL
			  AND pc.status = 'published' AND pc.deleted_at IS NULL AND album_has_public_photo(al.id)
			ORDER BY al.updated_at DESC LIMIT 100`
	default:
		return nil, fmt.Errorf("%w: 推荐只支持文章与相册", ErrInvalidInput)
	}
	rows, err := r.pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("list candidates: %w", err)
	}
	defer rows.Close()
	items := make([]Candidate, 0)
	for rows.Next() {
		var item Candidate
		if err := rows.Scan(&item.EntityType, &item.EntityID, &item.Title, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// SystemInfo reads the stored system snapshot and computes storage usage.
func (r *Repository) SystemInfo(ctx context.Context) (SystemInfo, error) {
	var info SystemInfo
	if err := r.pool.QueryRow(ctx, `SELECT version, build_status, storage_usage, job_status, updated_at
		FROM system_info WHERE id = 1`).Scan(&info.Version, &info.BuildStatus, &info.StorageUsage, &info.JobStatus, &info.UpdatedAt); err != nil {
		return SystemInfo{}, fmt.Errorf("load system info: %w", err)
	}
	var usage int64
	if err := r.pool.QueryRow(ctx, `SELECT COALESCE(sum(file_size), 0) FROM media_assets`).Scan(&usage); err != nil {
		return SystemInfo{}, fmt.Errorf("sum storage: %w", err)
	}
	info.StorageUsage = usage
	return info, nil
}

func likePattern(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + replacer.Replace(value) + "%"
}

func classifyWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			if strings.Contains(pgErr.ConstraintName, "slug") {
				return fmt.Errorf("%w: 链接标识已被使用，请换一个", ErrConflict)
			}
			return fmt.Errorf("%w: 数据已存在", ErrConflict)
		case "23503":
			return fmt.Errorf("%w: 上级内容不存在", ErrInvalidInput)
		}
	}
	return err
}
