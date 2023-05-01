CREATE TABLE IF NOT EXISTS files
(
    path         VARCHAR   NOT NULL,
    size         BIGINT    NOT NULL,
    content_type TEXT      NOT NULL,
    description  TEXT      NOT NULL,
    private      BOOLEAN   NOT NULL,
    user_id      VARCHAR   NOT NULL,
    created_at   TIMESTAMP NOT NULL,
    updated_at   TIMESTAMP NOT NULL,
    PRIMARY KEY (path)
);

CREATE TABLE IF NOT EXISTS users
(
    id       VARCHAR NOT NULL,
    username VARCHAR NOT NULL,
    email    VARCHAR NOT NULL,
    home     VARCHAR NOT NULL,
    PRIMARY KEY (id)
);