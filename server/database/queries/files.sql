-- name: GetFile :one
SELECT * FROM files WHERE path = @path;

-- name: ListFilesUnder :many
SELECT * FROM files
WHERE path = @path OR path LIKE @path_like
ORDER BY path;

-- name: ListAllFilePaths :many
SELECT path FROM files ORDER BY path;

-- name: UpsertFile :one
INSERT INTO files (path, size, content_type, description, user_id, created_at, updated_at)
VALUES (@path, @size, @content_type, @description, @user_id, @created_at, @updated_at)
ON CONFLICT (path) DO UPDATE SET
    size = excluded.size,
    content_type = excluded.content_type,
    description = excluded.description,
    user_id = excluded.user_id,
    updated_at = excluded.updated_at
RETURNING *;

-- name: UpdateFileMeta :one
UPDATE files
SET path = @new_path,
    size = @size,
    content_type = @content_type,
    description = @description,
    updated_at = @updated_at
WHERE path = @path
RETURNING *;

-- name: DeleteFile :exec
DELETE FROM files WHERE path = @path;

-- name: DeleteFilesUnder :exec
DELETE FROM files WHERE path = @path OR path LIKE @path_like;
