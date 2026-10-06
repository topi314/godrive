-- Add resumable upload sessions.
CREATE TABLE IF NOT EXISTS upload_sessions (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL,
    share_id      TEXT,
    path          TEXT NOT NULL,
    size          BIGINT NOT NULL,
    content_type  TEXT NOT NULL DEFAULT '',
    description   TEXT NOT NULL DEFAULT '',
    offset        BIGINT NOT NULL DEFAULT 0,
    replace_file  BOOLEAN NOT NULL DEFAULT FALSE,
    temp_key      TEXT NOT NULL,
    s3_upload_id  TEXT,
    s3_parts      TEXT NOT NULL DEFAULT '[]',
    expires_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS upload_sessions_user_id_idx ON upload_sessions (user_id);
CREATE INDEX IF NOT EXISTS upload_sessions_expires_at_idx ON upload_sessions (expires_at);
