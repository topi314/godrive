CREATE TABLE IF NOT EXISTS files
(
    path         VARCHAR   NOT NULL,
    size         BIGINT    NOT NULL,
    content_type TEXT      NOT NULL,
    description  TEXT      NOT NULL,
    user_id      VARCHAR   NOT NULL,
    created_at   TIMESTAMP NOT NULL,
    updated_at   TIMESTAMP NOT NULL,
    PRIMARY KEY (path)
);

CREATE TABLE IF NOT EXISTS file_permissions
(
    path   VARCHAR NOT NULL,
    permissions INT     NOT NULL,
    object_type INT     NOT NULL,
    object      VARCHAR NOT NULL,
    PRIMARY KEY (path, object_type, object)
);

CREATE TABLE IF NOT EXISTS users
(
    id       VARCHAR NOT NULL,
    username VARCHAR NOT NULL,
    email    VARCHAR NOT NULL,
    home     VARCHAR NOT NULL,
    PRIMARY KEY (id)
);