-- name: CreateUploadSession :one
INSERT INTO upload_sessions (
    id, user_id, share_id, path, size, content_type, description, offset, replace_file,
    temp_key, s3_upload_id, s3_parts, expires_at, created_at, updated_at
) VALUES (
    @id, @user_id, @share_id, @path, @size, @content_type, @description, @offset, @replace_file,
    @temp_key, @s3_upload_id, @s3_parts, @expires_at, @created_at, @updated_at
)
RETURNING *;

-- name: GetUploadSession :one
SELECT * FROM upload_sessions WHERE id = @id;

-- name: UpdateUploadSessionOffset :one
UPDATE upload_sessions
SET offset = @offset,
    s3_parts = @s3_parts,
    updated_at = @updated_at
WHERE id = @id
RETURNING *;

-- name: DeleteUploadSession :exec
DELETE FROM upload_sessions WHERE id = @id;

-- name: DeleteExpiredUploadSessions :many
DELETE FROM upload_sessions
WHERE expires_at < @now
RETURNING *;
