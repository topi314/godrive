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

CREATE TABLE IF NOT EXISTS users
(
    id       VARCHAR NOT NULL,
    username VARCHAR NOT NULL,
    groups   varchar NOT NULL,
    email    VARCHAR NOT NULL,
    home     VARCHAR NOT NULL,
    PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS sessions
(
	id            VARCHAR   NOT NULL,
	access_token  VARCHAR   NOT NULL,
	expiry        TIMESTAMP NOT NULL,
	refresh_token VARCHAR   NOT NULL,
	id_token      VARCHAR   NOT NULL,
	PRIMARY KEY (id)
);