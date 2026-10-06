-- name: ListACLByPath :many
SELECT * FROM path_acl WHERE path = @path ORDER BY principal_type, principal_id;

-- name: ListACLByPaths :many
SELECT * FROM path_acl
WHERE path IN (sqlc.slice('paths'))
ORDER BY path, principal_type, principal_id;

-- name: UpsertACL :one
INSERT INTO path_acl (path, principal_type, principal_id, allow, deny)
VALUES (@path, @principal_type, @principal_id, @allow, @deny)
ON CONFLICT (path, principal_type, principal_id) DO UPDATE SET
    allow = excluded.allow,
    deny = excluded.deny
RETURNING *;

-- name: DeleteACL :exec
DELETE FROM path_acl
WHERE path = @path AND principal_type = @principal_type AND principal_id = @principal_id;

-- name: DeleteACLForPath :exec
DELETE FROM path_acl WHERE path = @path;

-- name: ListAllACL :many
SELECT * FROM path_acl ORDER BY path, principal_type, principal_id;
