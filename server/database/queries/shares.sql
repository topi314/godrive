-- name: CreateShare :one
INSERT INTO shares (id, path, user_id, created_at, expires_at)
VALUES (@id, @path, @user_id, @created_at, @expires_at)
RETURNING *;

-- name: GetShare :one
SELECT * FROM shares WHERE id = @id;

-- name: ListSharesByUser :many
SELECT * FROM shares WHERE user_id = @user_id ORDER BY created_at DESC;

-- name: ListSharesByPath :many
SELECT * FROM shares WHERE path = @path OR path LIKE @path_like ORDER BY created_at DESC;

-- name: DeleteShare :exec
DELETE FROM shares WHERE id = @id;

-- name: DeleteSharesForPath :exec
DELETE FROM shares WHERE path = @path;
