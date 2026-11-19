-- +goose Up
-- Chinese full-text search fallback: standard PostgreSQL images do not ship
-- pg_jieba/zhparser, so public search uses pg_trgm over a payload field that
-- already excludes Markdown markup and fenced code blocks.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE content_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type text NOT NULL
        CHECK (entity_type IN ('article_column', 'article', 'photo_column', 'album', 'photo')),
    entity_id uuid NOT NULL,
    version_no integer NOT NULL CHECK (version_no > 0),
    status text NOT NULL CHECK (status IN ('draft', 'pending', 'approved', 'rejected')),
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    -- Set when an administrator approves the version and it becomes public.
    published_at timestamptz,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    reviewed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at timestamptz,
    review_comment text NOT NULL DEFAULT '',
    UNIQUE (entity_type, entity_id, version_no)
);
CREATE INDEX content_versions_entity_idx ON content_versions (entity_type, entity_id, version_no DESC);
CREATE INDEX content_versions_published_idx
    ON content_versions (entity_type, published_at DESC) WHERE published_at IS NOT NULL;
CREATE INDEX content_versions_title_trgm_idx
    ON content_versions USING gin ((payload ->> 'title') gin_trgm_ops);
CREATE INDEX content_versions_summary_trgm_idx
    ON content_versions USING gin ((payload ->> 'summary') gin_trgm_ops);
CREATE INDEX content_versions_body_trgm_idx
    ON content_versions USING gin ((payload ->> 'bodyIndex') gin_trgm_ops);

CREATE TABLE article_columns (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'published', 'offline', 'trashed')),
    published_version_id uuid REFERENCES content_versions(id) ON DELETE SET NULL,
    deleted_at timestamptz,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX article_columns_status_idx ON article_columns (status) WHERE deleted_at IS NULL;

CREATE TABLE articles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    column_id uuid NOT NULL REFERENCES article_columns(id) ON DELETE CASCADE,
    slug text NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'published', 'offline', 'trashed')),
    published_version_id uuid REFERENCES content_versions(id) ON DELETE SET NULL,
    deleted_at timestamptz,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX articles_column_status_idx ON articles (column_id, status) WHERE deleted_at IS NULL;

CREATE TABLE photo_columns (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'published', 'offline', 'trashed')),
    published_version_id uuid REFERENCES content_versions(id) ON DELETE SET NULL,
    deleted_at timestamptz,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX photo_columns_status_idx ON photo_columns (status) WHERE deleted_at IS NULL;

CREATE TABLE albums (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    column_id uuid NOT NULL REFERENCES photo_columns(id) ON DELETE CASCADE,
    slug text NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'published', 'offline', 'trashed')),
    published_version_id uuid REFERENCES content_versions(id) ON DELETE SET NULL,
    deleted_at timestamptz,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX albums_column_status_idx ON albums (column_id, status) WHERE deleted_at IS NULL;

CREATE TABLE photos (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    album_id uuid NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
    status text NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'published', 'offline', 'trashed')),
    published_version_id uuid REFERENCES content_versions(id) ON DELETE SET NULL,
    sort_order integer NOT NULL DEFAULT 0,
    deleted_at timestamptz,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX photos_album_sort_idx ON photos (album_id, sort_order, id) WHERE deleted_at IS NULL;

-- A public album must contain at least one published photo with a ready derivative.
CREATE FUNCTION album_has_public_photo(target_album uuid) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT EXISTS (
        SELECT 1
        FROM photos p
        JOIN content_versions cv ON cv.id = p.published_version_id
        JOIN media_assets m
             ON m.id::text = NULLIF(cv.payload ->> 'mediaId', '') AND m.status = 'ready'
        WHERE p.album_id = target_album
          AND p.status = 'published' AND p.deleted_at IS NULL
    )
$$;

CREATE TABLE tags (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    scope text NOT NULL CHECK (scope IN ('article', 'photo')),
    name text NOT NULL,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (scope, name)
);
CREATE INDEX tags_scope_active_idx ON tags (scope) WHERE active;

CREATE TABLE content_tags (
    content_version_id uuid NOT NULL REFERENCES content_versions(id) ON DELETE CASCADE,
    tag_id uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (content_version_id, tag_id)
);
CREATE INDEX content_tags_tag_idx ON content_tags (tag_id, content_version_id);

CREATE TABLE reviews (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    version_id uuid NOT NULL REFERENCES content_versions(id) ON DELETE CASCADE,
    reviewer_id uuid REFERENCES users(id) ON DELETE SET NULL,
    decision text NOT NULL CHECK (decision IN ('approved', 'rejected')),
    comment text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX reviews_version_idx ON reviews (version_id);

CREATE TABLE recommendations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type text NOT NULL CHECK (entity_type IN ('article', 'album')),
    entity_id uuid NOT NULL,
    position integer NOT NULL UNIQUE,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX recommendations_entity_idx ON recommendations (entity_type, entity_id);

CREATE TABLE notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    recipient_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type text NOT NULL,
    severity text NOT NULL DEFAULT 'info' CHECK (severity IN ('info', 'warning', 'error')),
    title text NOT NULL,
    body text NOT NULL DEFAULT '',
    link text NOT NULL DEFAULT '',
    read_at timestamptz,
    dedupe_key text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notifications_recipient_idx ON notifications (recipient_id, created_at DESC);
CREATE UNIQUE INDEX notifications_dedupe_uidx
    ON notifications (recipient_id, dedupe_key) WHERE dedupe_key <> '';

CREATE TABLE feature_flags (
    flag text PRIMARY KEY,
    enabled boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE system_info (
    id smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    version text NOT NULL DEFAULT '',
    build_status text NOT NULL DEFAULT 'unknown',
    storage_usage bigint NOT NULL DEFAULT 0,
    job_status jsonb NOT NULL DEFAULT '{}'::jsonb,
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO system_info (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

-- +goose Down
DROP TABLE system_info;
DROP TABLE feature_flags;
DROP TABLE notifications;
DROP TABLE recommendations;
DROP TABLE reviews;
DROP TABLE content_tags;
DROP TABLE tags;
DROP FUNCTION IF EXISTS album_has_public_photo(uuid);
DROP TABLE photos;
DROP TABLE albums;
DROP TABLE photo_columns;
DROP TABLE articles;
DROP TABLE article_columns;
DROP TABLE content_versions;
