-- +goose Up
CREATE TABLE roles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL UNIQUE,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE permissions (
    code text PRIMARY KEY,
    description text NOT NULL
);

CREATE TABLE role_permissions (
    role_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_code text NOT NULL REFERENCES permissions(code) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_code)
);

INSERT INTO roles (code, name) VALUES
    ('super_admin', '超级管理员'),
    ('admin', '管理员'),
    ('editor', '编辑'),
    ('user', '普通账号')
ON CONFLICT (code) DO NOTHING;

INSERT INTO permissions (code, description) VALUES
    ('admin.access', '访问管理后台'),
    ('content.read', '读取内容'),
    ('content.write_own', '管理本人内容'),
    ('content.manage_all', '管理全站内容'),
    ('review.manage', '审批内容'),
    ('account.manage', '管理账号权限'),
    ('system.manage', '管理系统配置'),
    ('audit.read', '查看审计日志')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code
FROM roles r
CROSS JOIN permissions p
WHERE r.code IN ('super_admin', 'admin')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code
FROM roles r
CROSS JOIN permissions p
WHERE r.code = 'editor' AND p.code IN ('content.read', 'content.write_own')
ON CONFLICT DO NOTHING;

ALTER TABLE users ADD COLUMN role_id uuid REFERENCES roles(id);
UPDATE users SET role_id = (SELECT id FROM roles WHERE code = 'user') WHERE role_id IS NULL;
ALTER TABLE users ALTER COLUMN role_id SET NOT NULL;
CREATE INDEX users_role_id_idx ON users (role_id);
CREATE UNIQUE INDEX users_username_lower_uidx ON users (lower(username));
CREATE UNIQUE INDEX users_email_lower_uidx ON users (lower(email));

CREATE TABLE auth_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token_hash text NOT NULL,
    remember boolean NOT NULL DEFAULT false,
    user_agent text NOT NULL DEFAULT '',
    ip text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz
);
CREATE UNIQUE INDEX auth_sessions_refresh_hash_uidx ON auth_sessions (refresh_token_hash);
CREATE INDEX auth_sessions_user_id_idx ON auth_sessions (user_id);

CREATE TABLE audit_logs (
    id bigserial PRIMARY KEY,
    actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
    action text NOT NULL,
    target_type text NOT NULL,
    target_id text NOT NULL DEFAULT '',
    result text NOT NULL,
    ip text NOT NULL DEFAULT '',
    user_agent text NOT NULL DEFAULT '',
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_created_at_idx ON audit_logs (created_at DESC);
CREATE INDEX audit_logs_actor_id_idx ON audit_logs (actor_id);

-- +goose Down
DROP TABLE audit_logs;
DROP TABLE auth_sessions;
DROP INDEX IF EXISTS users_email_lower_uidx;
DROP INDEX IF EXISTS users_username_lower_uidx;
DROP INDEX IF EXISTS users_role_id_idx;
ALTER TABLE users DROP COLUMN role_id;
DROP TABLE role_permissions;
DROP TABLE permissions;
DROP TABLE roles;
