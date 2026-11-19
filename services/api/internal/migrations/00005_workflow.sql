-- +goose Up
-- Review workflow needs an edit timestamp, a submission timestamp and an explicit
-- withdrawal record so the public version stays untouched while a draft is in flight.
ALTER TABLE content_versions
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN submitted_at timestamptz,
    ADD COLUMN withdrawn_at timestamptz;

-- One pending submission per entity keeps the review queue unambiguous.
CREATE UNIQUE INDEX content_versions_one_pending_uidx
    ON content_versions (entity_type, entity_id) WHERE status = 'pending';
CREATE INDEX content_versions_review_idx
    ON content_versions (status, submitted_at DESC);

-- Workspace lists are always scoped by owner.
CREATE INDEX article_columns_owner_idx ON article_columns (created_by);
CREATE INDEX articles_owner_idx ON articles (created_by);
CREATE INDEX photo_columns_owner_idx ON photo_columns (created_by);
CREATE INDEX albums_owner_idx ON albums (created_by);
CREATE INDEX photos_owner_idx ON photos (created_by);

-- Home page recommendation counts and automatic fallback are deployment settings
-- that administrators can change without a code release.
ALTER TABLE system_info
    ADD COLUMN recommendation_n1 integer NOT NULL DEFAULT 3
        CHECK (recommendation_n1 BETWEEN 0 AND 12),
    ADD COLUMN recommendation_n2 integer NOT NULL DEFAULT 3
        CHECK (recommendation_n2 BETWEEN 0 AND 12),
    ADD COLUMN recommendation_auto_fill boolean NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE system_info
    DROP COLUMN recommendation_auto_fill,
    DROP COLUMN recommendation_n2,
    DROP COLUMN recommendation_n1;
DROP INDEX IF EXISTS photos_owner_idx;
DROP INDEX IF EXISTS albums_owner_idx;
DROP INDEX IF EXISTS photo_columns_owner_idx;
DROP INDEX IF EXISTS articles_owner_idx;
DROP INDEX IF EXISTS article_columns_owner_idx;
DROP INDEX IF EXISTS content_versions_review_idx;
DROP INDEX IF EXISTS content_versions_one_pending_uidx;
ALTER TABLE content_versions
    DROP COLUMN withdrawn_at,
    DROP COLUMN submitted_at,
    DROP COLUMN updated_at;
