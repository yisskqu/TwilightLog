-- +goose Up
CREATE TABLE media_assets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose text NOT NULL CHECK (purpose IN ('article', 'photo')),
    storage_key text NOT NULL UNIQUE,
    original_filename text NOT NULL,
    mime_type text NOT NULL,
    file_size bigint NOT NULL DEFAULT 0,
    width integer NOT NULL DEFAULT 0,
    height integer NOT NULL DEFAULT 0,
    blurhash text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'uploading'
        CHECK (status IN ('uploading', 'processing', 'ready', 'failed')),
    batch_id text NOT NULL DEFAULT '',
    upload_mode text NOT NULL DEFAULT 'single'
        CHECK (upload_mode IN ('single', 'multipart')),
    upload_id text NOT NULL DEFAULT '',
    part_size bigint NOT NULL DEFAULT 0,
    part_count integer NOT NULL DEFAULT 0,
    master_key text NOT NULL DEFAULT '',
    exif jsonb NOT NULL DEFAULT '{}'::jsonb,
    variants jsonb NOT NULL DEFAULT '{}'::jsonb,
    error_code text NOT NULL DEFAULT '',
    error_message text NOT NULL DEFAULT '',
    processed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX media_assets_owner_created_idx ON media_assets (owner_id, created_at DESC);
CREATE INDEX media_assets_owner_batch_idx ON media_assets (owner_id, batch_id);
CREATE INDEX media_assets_status_updated_idx ON media_assets (status, updated_at);

CREATE TABLE idempotency_keys (
    owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key text NOT NULL,
    operation text NOT NULL,
    resource_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (owner_id, key)
);

-- +goose Down
DROP TABLE idempotency_keys;
DROP TABLE media_assets;
