package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const assetColumns = `id::text, owner_id::text, purpose, storage_key, original_filename, mime_type,
	file_size, width, height, blurhash, status, batch_id, upload_mode, upload_id, part_size,
	part_count, master_key, exif, variants, error_code, error_message, processed_at,
	created_at, updated_at`

func (r *Repository) Create(ctx context.Context, asset Asset) error {
	exifPayload, err := json.Marshal(asset.Exif)
	if err != nil {
		return fmt.Errorf("encode exif: %w", err)
	}
	variantsPayload, err := json.Marshal(asset.Variants)
	if err != nil {
		return fmt.Errorf("encode variants: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO media_assets (
			id, owner_id, purpose, storage_key, original_filename, mime_type, file_size,
			width, height, blurhash, status, batch_id, upload_mode, upload_id, part_size,
			part_count, master_key, exif, variants, error_code, error_message
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17,
			$18::jsonb, $19::jsonb, $20, $21
		)
	`,
		asset.ID, asset.OwnerID, asset.Purpose, asset.StorageKey, asset.OriginalFilename,
		asset.MimeType, asset.FileSize, asset.Width, asset.Height, asset.Blurhash,
		asset.Status, asset.BatchID, asset.UploadMode, asset.UploadID, asset.PartSize,
		asset.PartCount, asset.MasterKey, string(exifPayload), string(variantsPayload),
		asset.ErrorCode, asset.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("create media asset: %w", err)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, id string) (Asset, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+assetColumns+` FROM media_assets WHERE id = $1`, id)
	asset, err := scanAsset(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Asset{}, ErrNotFound
	}
	if err != nil {
		return Asset{}, fmt.Errorf("get media asset: %w", err)
	}
	return asset, nil
}

func (r *Repository) CountBatch(ctx context.Context, ownerID, batchID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM media_assets WHERE owner_id = $1 AND batch_id = $2
	`, ownerID, batchID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count media batch: %w", err)
	}
	return count, nil
}

func (r *Repository) SetUploadedSize(ctx context.Context, id string, size int64) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE media_assets SET file_size = $2, updated_at = now() WHERE id = $1
	`, id, size)
	if err != nil {
		return fmt.Errorf("update media size: %w", err)
	}
	return nil
}

// MarkProcessing moves an upload into processing. It reports false when the
// asset is already ready or another request already owns processing.
func (r *Repository) MarkProcessing(ctx context.Context, id string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE media_assets
		SET status = 'processing', error_code = '', error_message = '', updated_at = now()
		WHERE id = $1 AND status IN ('uploading', 'failed')
	`, id)
	if err != nil {
		return false, fmt.Errorf("mark media processing: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repository) MarkReady(ctx context.Context, id, mimeType string, result ProcessResult) error {
	exifPayload, err := json.Marshal(result.Exif)
	if err != nil {
		return fmt.Errorf("encode exif: %w", err)
	}
	variantsPayload, err := json.Marshal(result.Variants)
	if err != nil {
		return fmt.Errorf("encode variants: %w", err)
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE media_assets
		SET status = 'ready', mime_type = $2, width = $3, height = $4, blurhash = $5,
		    master_key = $6, exif = $7::jsonb, variants = $8::jsonb,
		    error_code = '', error_message = '', processed_at = now(), updated_at = now()
		WHERE id = $1
	`,
		id, mimeType, result.Width, result.Height, result.Blurhash, result.MasterKey,
		string(exifPayload), string(variantsPayload),
	)
	if err != nil {
		return fmt.Errorf("mark media ready: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) MarkFailed(ctx context.Context, id, code, message string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE media_assets
		SET status = 'failed', error_code = $2, error_message = $3, updated_at = now()
		WHERE id = $1
	`, id, code, truncateText(message, 500))
	if err != nil {
		return fmt.Errorf("mark media failed: %w", err)
	}
	return nil
}

func (r *Repository) FindIdempotencyKey(ctx context.Context, ownerID, key string) (string, bool, error) {
	var resourceID string
	err := r.pool.QueryRow(ctx, `
		SELECT resource_id::text FROM idempotency_keys WHERE owner_id = $1 AND key = $2
	`, ownerID, key).Scan(&resourceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read idempotency key: %w", err)
	}
	return resourceID, true, nil
}

func (r *Repository) CreateWithIdempotency(ctx context.Context, asset Asset, idemKey string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin presign transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	exifPayload, _ := json.Marshal(asset.Exif)
	variantsPayload, _ := json.Marshal(asset.Variants)
	_, err = tx.Exec(ctx, `
		INSERT INTO media_assets (
			id, owner_id, purpose, storage_key, original_filename, mime_type, file_size,
			width, height, blurhash, status, batch_id, upload_mode, upload_id, part_size,
			part_count, master_key, exif, variants, error_code, error_message
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17,
			$18::jsonb, $19::jsonb, $20, $21
		)
	`,
		asset.ID, asset.OwnerID, asset.Purpose, asset.StorageKey, asset.OriginalFilename,
		asset.MimeType, asset.FileSize, asset.Width, asset.Height, asset.Blurhash,
		asset.Status, asset.BatchID, asset.UploadMode, asset.UploadID, asset.PartSize,
		asset.PartCount, asset.MasterKey, string(exifPayload), string(variantsPayload),
		asset.ErrorCode, asset.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("create media asset: %w", err)
	}
	if idemKey != "" {
		_, err = tx.Exec(ctx, `
			INSERT INTO idempotency_keys (owner_id, key, operation, resource_id)
			VALUES ($1, $2, 'media.presign', $3)
		`, asset.OwnerID, idemKey, asset.ID)
		if err != nil {
			return fmt.Errorf("store idempotency key: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit presign transaction: %w", err)
	}
	return nil
}

// ListStaleUploading returns uploads that never completed.
func (r *Repository) ListStaleUploading(ctx context.Context, before time.Time, limit int) ([]Asset, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+assetColumns+` FROM media_assets
		WHERE status = 'uploading' AND updated_at < $1
		ORDER BY updated_at ASC
		LIMIT $2
	`, before, limit)
	if err != nil {
		return nil, fmt.Errorf("list stale uploads: %w", err)
	}
	defer rows.Close()
	return scanAssets(rows)
}

// ListFailed returns failed rows older than the cutoff for cleanup.
func (r *Repository) ListFailed(ctx context.Context, before time.Time, limit int) ([]Asset, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+assetColumns+` FROM media_assets
		WHERE status = 'failed' AND updated_at < $1
		ORDER BY updated_at ASC
		LIMIT $2
	`, before, limit)
	if err != nil {
		return nil, fmt.Errorf("list failed media: %w", err)
	}
	defer rows.Close()
	return scanAssets(rows)
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM media_assets WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete media asset: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAsset(row rowScanner) (Asset, error) {
	var asset Asset
	var exifPayload, variantsPayload []byte
	err := row.Scan(
		&asset.ID, &asset.OwnerID, &asset.Purpose, &asset.StorageKey, &asset.OriginalFilename,
		&asset.MimeType, &asset.FileSize, &asset.Width, &asset.Height, &asset.Blurhash,
		&asset.Status, &asset.BatchID, &asset.UploadMode, &asset.UploadID, &asset.PartSize,
		&asset.PartCount, &asset.MasterKey, &exifPayload, &variantsPayload,
		&asset.ErrorCode, &asset.ErrorMessage, &asset.ProcessedAt, &asset.CreatedAt, &asset.UpdatedAt,
	)
	if err != nil {
		return Asset{}, err
	}
	if len(exifPayload) > 0 {
		_ = json.Unmarshal(exifPayload, &asset.Exif)
	}
	if len(variantsPayload) > 0 {
		_ = json.Unmarshal(variantsPayload, &asset.Variants)
	}
	if asset.Variants == nil {
		asset.Variants = map[string]VariantInfo{}
	}
	return asset, nil
}

func scanAssets(rows pgx.Rows) ([]Asset, error) {
	var assets []Asset
	for rows.Next() {
		asset, err := scanAsset(rows)
		if err != nil {
			return nil, fmt.Errorf("scan media asset: %w", err)
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate media assets: %w", err)
	}
	return assets, nil
}

func truncateText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
