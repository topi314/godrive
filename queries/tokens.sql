-- name: CreateAPIToken :one
INSERT INTO api_tokens (token_hash, token_prefix, user_id, description, created_at)
VALUES (@token_hash, @token_prefix, @user_id, @description, @created_at)
RETURNING *;

-- name: GetAPITokenByHash :one
SELECT * FROM api_tokens WHERE token_hash = @token_hash;

-- name: ListAPITokensByUser :many
SELECT * FROM api_tokens WHERE user_id = @user_id ORDER BY created_at DESC;

-- name: DeleteAPIToken :exec
DELETE FROM api_tokens WHERE token_hash = @token_hash AND user_id = @user_id;
