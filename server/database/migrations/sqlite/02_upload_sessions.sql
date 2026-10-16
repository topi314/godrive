-- Add resumable upload sessions.
CREATE TABLE IF NOT EXISTS upload_sessions (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL,
    share_id      TEXT,
    path          TEXT NOT NULL,
    size          INTEGER NOT NULL,
    content_type  TEXT NOT NULL DEFAULT '',
    description   TEXT NOT NULL DEFAULT '',
    upload_offset INTEGER NOT NULL DEFAULT 0,
    replace_file  INTEGER NOT NULL DEFAULT 0,
    temp_key      TEXT NOT NULL,
    s3_upload_id  TEXT,
    s3_parts      TEXT NOT NULL DEFAULT '[]',
    expires_at    TIMESTAMP NOT NULL,
    created_at    TIMESTAMP NOT NULL,
    updated_at    TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS upload_sessions_user_id_idx ON upload_sessions (user_id);
CREATE INDEX IF NOT EXISTS upload_sessions_expires_at_idx ON upload_sessions (expires_at);
