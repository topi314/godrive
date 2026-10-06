CREATE TABLE IF NOT EXISTS users
(
    id         TEXT        NOT NULL PRIMARY KEY,
    username   TEXT        NOT NULL,
    email      TEXT        NOT NULL,
    home       TEXT        NOT NULL DEFAULT '/',
    groups     JSONB       NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS files
(
    path         TEXT        NOT NULL PRIMARY KEY,
    size         BIGINT      NOT NULL,
    content_type TEXT        NOT NULL,
    description  TEXT        NOT NULL DEFAULT '',
    user_id      TEXT,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS files_user_id_idx ON files (user_id);

CREATE TABLE IF NOT EXISTS path_acl
(
    path           TEXT   NOT NULL,
    principal_type TEXT   NOT NULL,
    principal_id   TEXT   NOT NULL,
    allow          BIGINT NOT NULL,
    deny           BIGINT NOT NULL,
    PRIMARY KEY (path, principal_type, principal_id)
);

CREATE INDEX IF NOT EXISTS path_acl_path_idx ON path_acl (path);

CREATE TABLE IF NOT EXISTS shares
(
    id         TEXT        NOT NULL PRIMARY KEY,
    path       TEXT        NOT NULL,
    user_id    TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS shares_path_idx ON shares (path);
CREATE INDEX IF NOT EXISTS shares_user_id_idx ON shares (user_id);

CREATE TABLE IF NOT EXISTS sessions
(
    id            TEXT        NOT NULL PRIMARY KEY,
    user_id       TEXT        NOT NULL,
    access_token  TEXT        NOT NULL,
    expiry        TIMESTAMPTZ NOT NULL,
    refresh_token TEXT        NOT NULL,
    id_token      TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions (user_id);

CREATE TABLE IF NOT EXISTS api_tokens
(
    token_hash   TEXT        NOT NULL PRIMARY KEY,
    token_prefix TEXT        NOT NULL,
    user_id      TEXT        NOT NULL,
    description  TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS api_tokens_user_id_idx ON api_tokens (user_id);
