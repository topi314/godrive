-- name: UpsertSession :one
INSERT INTO sessions (id, user_id, access_token, expiry, refresh_token, id_token, created_at, updated_at)
VALUES (@id, @user_id, @access_token, @expiry, @refresh_token, @id_token, @created_at, @updated_at)
ON CONFLICT (id) DO UPDATE SET
    user_id = excluded.user_id,
    access_token = excluded.access_token,
    expiry = excluded.expiry,
    refresh_token = excluded.refresh_token,
    id_token = excluded.id_token,
    updated_at = excluded.updated_at
RETURNING *;

-- name: GetSession :one
SELECT * FROM sessions WHERE id = @id;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = @id;

-- name: DeleteSessionsForUser :exec
DELETE FROM sessions WHERE user_id = @user_id;
