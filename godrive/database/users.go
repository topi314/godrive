package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

var ErrUserNotFound = errors.New("user not found")

type User struct {
	ID       string `db:"id"`
	Username string `db:"username"`
	Groups   Groups `db:"groups"`
	Email    string `db:"email"`
	Home     string `db:"home"`
}

type Groups []string

func (g *Groups) Scan(src any) error {
	if v, ok := src.(string); ok {
		*g = strings.Split(v, ",")
		return nil
	}
	return fmt.Errorf("invalid type '%T' for groups", src)

}

func (g Groups) Value() (driver.Value, error) {
	return strings.Join(g, ","), nil
}

func (d *DB) UpsertUser(ctx context.Context, id string, username string, groups []string, email string, home string) error {
	user := &User{
		ID:       id,
		Username: username,
		Groups:   groups,
		Email:    email,
		Home:     home,
	}
	_, err := d.dbx.NamedExecContext(ctx, "INSERT INTO users (id, username, groups, email, home) VALUES (:id, :username, :groups, :email, :home) ON CONFLICT (id) DO UPDATE SET username = :username, groups = :groups, email = :email", user)
	if err != nil {
		return fmt.Errorf("error upserting user: %w", err)
	}
	return nil
}

func (d *DB) GetUsers(ctx context.Context, ids []string) ([]User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var users []User
	query, args, err := sqlx.In("SELECT * FROM users WHERE id IN (?)", ids)
	if err != nil {
		return nil, err
	}

	if err = d.dbx.SelectContext(ctx, &users, d.dbx.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("error getting users: %w", err)
	}

	return users, nil
}

func (d *DB) GetUser(ctx context.Context, id string) (*User, error) {
	var user User
	if err := d.dbx.GetContext(ctx, &user, "SELECT * FROM users WHERE id = $1", id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrUserNotFound
		}
		return nil, fmt.Errorf("error getting user: %w", err)
	}

	return &user, nil
}

func (d *DB) GetUserByName(ctx context.Context, username string) (*User, error) {
	var user User
	if err := d.dbx.GetContext(ctx, &user, "SELECT * FROM users WHERE username = $1", username); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrUserNotFound
		}
		return nil, fmt.Errorf("error getting user: %w", err)
	}

	return &user, nil
}

func (d *DB) GetAllUsers(ctx context.Context) ([]User, error) {
	var users []User
	if err := d.dbx.SelectContext(ctx, &users, "SELECT * FROM users"); err != nil {
		return nil, fmt.Errorf("error getting all users: %w", err)
	}

	return users, nil
}
