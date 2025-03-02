package database

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type Permissions struct {
	Path       string  `db:"path"`
	Allow      int64   `db:"allow"`
	Deny       int64   `db:"deny"`
	ObjectType int     `db:"object_type"`
	Object     string  `db:"object"`
	ObjectName *string `db:"object_name"`
}

func (d *DB) GetAllPermissions(ctx context.Context) ([]Permissions, error) {
	var permissions []Permissions
	if err := d.dbx.SelectContext(ctx, &permissions, "SELECT * FROM permissions"); err != nil {
		return nil, fmt.Errorf("error getting all permissions: %w", err)
	}
	return permissions, nil
}

func (d *DB) GetPermissions(ctx context.Context, paths []string) ([]Permissions, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	query, args, err := sqlx.In("SELECT permissions.*, users.username as object_name FROM permissions LEFT JOIN users ON permissions.object_type = 0 AND permissions.object = users.id WHERE permissions.path IN (?)", paths)
	if err != nil {
		return nil, err
	}
	permissions := make([]Permissions, 0)
	if err = d.dbx.SelectContext(ctx, &permissions, d.dbx.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("error getting path permissions: %w", err)
	}
	return permissions, nil
}

func (d *DB) UpsertPermission(ctx context.Context, path string, permissions int, objectType int, object string) error {
	return nil
}

func (d *DB) DeletePermission(ctx context.Context, path string, objectType int, object string) error {
	return nil
}

func (d *DB) DeletePermissions(ctx context.Context, path string) error {
	return nil
}

func (d *DB) DeletePermissionsForObject(ctx context.Context, objectType int, object string) error {
	return nil
}
