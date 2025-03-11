package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

var ErrApiTokenNotFound = errors.New("api token not found")

type APIToken struct {
	Token       string `db:"token"`
	UserID      string `db:"user_id"`
	Description string `db:"description"`
}

func (d *DB) CreateToken(ctx context.Context, token string, userId string, description string) (*APIToken, error) {
	apiToken := &APIToken{
		Token:       token,
		UserID:      userId,
		Description: description,
	}
	_, err := d.dbx.NamedExecContext(ctx, "INSERT INTO api_tokens (token, user_id,  description) VALUES (:token, :user_id, :description)", apiToken)
	if err != nil {
		return nil, fmt.Errorf("error inserting token: %w", err)
	}
	return apiToken, nil
}

func (d *DB) GetApiToken(ctx context.Context, token string) (*APIToken, error) {
	var apiToken APIToken
	if err := d.dbx.GetContext(ctx, &apiToken, "SELECT * FROM api_tokens WHERE token = $1", token); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrApiTokenNotFound
		}
		return nil, fmt.Errorf("error getting apiToken: %w", err)
	}

	return &apiToken, nil
}

func (d *DB) GetApiTokens(ctx context.Context, userId string) ([]APIToken, error) {
	var apiTokens []APIToken
	query, args, err := sqlx.In("SELECT * FROM api_tokens WHERE user_id = $1", userId)
	if err != nil {
		return nil, err
	}

	if err = d.dbx.SelectContext(ctx, &apiTokens, d.dbx.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("error getting apiTokens: %w", err)
	}

	return apiTokens, nil
}

// DeleteApiToken return true iff >= 1 row has been deleted.
func (d *DB) DeleteApiToken(ctx context.Context, token string) (bool, error) {
	res, err := d.dbx.ExecContext(ctx, "DELETE FROM api_tokens WHERE token = $1", token)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 0, nil
}
