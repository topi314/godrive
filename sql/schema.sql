CREATE TABLE IF NOT EXISTS files
(
    dir         VARCHAR   NOT NULL,
    name         VARCHAR   NOT NULL,
    size         BIGINT    NOT NULL,
    content_type TEXT      NOT NULL,
    description  TEXT      NOT NULL,
    private      BOOLEAN   NOT NULL,
    created_at   TIMESTAMP NOT NULL,
    updated_at   TIMESTAMP NOT NULL,
    PRIMARY KEY (dir, name)
);