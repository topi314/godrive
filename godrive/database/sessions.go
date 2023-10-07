package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrSessionNotFound = errors.New("session not found")

type Session struct {
	ID           string    `db:"id"`
	AccessToken  string    `db:"access_token"`
	Expiry       time.Time `db:"expiry"`
	RefreshToken string    `db:"refresh_token"`
	IDToken      string    `db:"id_token"`
}

func (d *DB) CreateSession(ctx context.Context, session Session) error {
	if _, err := d.dbx.NamedExecContext(ctx, "INSERT INTO sessions (id, access_token, expiry, refresh_token, id_token) VALUES (:id, :access_token, :expiry, :refresh_token, :id_token) ", session); err != nil {
		return fmt.Errorf("error creating session: %w", err)
	}
	return nil
}

func (d *DB) DeleteSession(ctx context.Context, id string) error {
	if _, err := d.dbx.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", id); err != nil {
		return fmt.Errorf("error deleting session: %w", err)
	}
	return nil
}

func (d *DB) GetSession(ctx context.Context, id string) (*Session, error) {
	var session Session
	if err := d.dbx.GetContext(ctx, &session, "SELECT * FROM sessions WHERE id = $1", id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrSessionNotFound
		}
		return nil, fmt.Errorf("error getting session: %w", err)
	}

	return &session, nil
}
