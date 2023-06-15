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
	path        VARCHAR NOT NULL,
	permissions INT     NOT NULL,
	object_type INT     NOT NULL,
	object      VARCHAR NOT NULL,
	PRIMARY KEY (path, object_type, object)
);

CREATE TABLE IF NOT EXISTS users
(
	id       VARCHAR NOT NULL,
	username VARCHAR NOT NULL,
	groups   VARCHAR NOT NULL,
	email    VARCHAR NOT NULL,
	home     VARCHAR NOT NULL,
	PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS shares
(
	id          VARCHAR NOT NULL,
	path        VARCHAR NOT NULL,
	permissions INT     NOT NULL,
	user_id     VARCHAR NOT NULL,
	PRIMARY KEY (id),
	UNIQUE (path, permissions, user_id)
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