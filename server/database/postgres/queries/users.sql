-- name: UpsertUser :one
INSERT INTO users (id, username, email, home, groups, created_at, updated_at)
VALUES (@id, @username, @email, @home, @groups, @created_at, @updated_at)
ON CONFLICT (id) DO UPDATE SET
    username = excluded.username,
    email = excluded.email,
    home = users.home,
    groups = excluded.groups,
    updated_at = excluded.updated_at
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = @id;

-- name: ListUsers :many
SELECT * FROM users ORDER BY username;

-- name: UpdateUserHome :one
UPDATE users
SET home = @home,
    updated_at = @updated_at
WHERE id = @id
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = @id;
