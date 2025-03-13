package database

import (
	"context"
	"database/sql"
	"errors"
)

var ErrShareNotFound = errors.New("share not found")

type Share struct {
	ID     string `db:"id"`
	Path   string `db:"path"`
	UserID string `db:"user_id"`
}

func (d *DB) GetShare(ctx context.Context, id string) (*Share, error) {
	var share Share
	if err := d.dbx.GetContext(ctx, &share, "SELECT * FROM shares WHERE id = $1", id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrShareNotFound
		}
		return nil, err
	}

	return &share, nil
}

func (d *DB) CreateShare(ctx context.Context, share Share) error {
	_, err := d.dbx.NamedExecContext(ctx, "INSERT INTO shares (id, path, user_id) VALUES (:id, :path, :user_id)", share)
	return err
}

func (d *DB) DeleteShare(ctx context.Context, id string) error {
	_, err := d.dbx.ExecContext(ctx, "DELETE FROM shares WHERE id = $1", id)
	return err
}
