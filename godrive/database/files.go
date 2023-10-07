package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"modernc.org/sqlite"
)

var (
	ErrFileNotFound      = errors.New("file not found")
	ErrFileAlreadyExists = errors.New("file already exists")
)

type File struct {
	Path        string    `db:"path"`
	Size        int64     `db:"size"`
	ContentType string    `db:"content_type"`
	Description string    `db:"description"`
	UserID      string    `db:"user_id"`
	Username    *string   `db:"username"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

type UpdateFile struct {
	Path        string    `db:"path"`
	NewPath     string    `db:"new_path"`
	Size        int64     `db:"size"`
	ContentType string    `db:"content_type"`
	Description string    `db:"description"`
	UpdatedAt   time.Time `db:"updated_at"`
}

func (d *DB) FindFiles(ctx context.Context, path string) ([]File, error) {
	var file File
	err := d.dbx.GetContext(ctx, &file, "SELECT files.*, users.username FROM files LEFT JOIN users ON files.user_id = users.id WHERE files.path = $1", path)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("error finding file: %w", err)
	} else if err == nil {
		return []File{file}, nil
	}

	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	var files []File
	err = d.dbx.SelectContext(ctx, &files, "SELECT files.*, users.username FROM files LEFT JOIN users ON files.user_id = users.id WHERE files.path like $1", path+"%")
	if err != nil {
		return nil, fmt.Errorf("error finding files: %w", err)
	}

	return files, nil
}

func (d *DB) GetFile(ctx context.Context, path string) (*File, error) {
	file := new(File)
	err := d.dbx.GetContext(ctx, file, "SELECT files.*, users.username FROM files LEFT JOIN users ON files.user_id = users.id WHERE files.path = $1", path)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrFileNotFound
		}
		return nil, fmt.Errorf("error getting file: %w", err)
	}

	return file, nil
}

func (d *DB) CreateFile(ctx context.Context, path string, size int64, contentType string, description string, userID string) (*File, *sqlx.Tx, error) {
	tx, err := d.dbx.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating file: %w", err)
	}

	file := &File{
		Path:        path,
		Size:        size,
		ContentType: contentType,
		Description: description,
		UserID:      userID,
		CreatedAt:   time.Now(),
	}
	_, err = tx.NamedExecContext(ctx, "INSERT INTO files (path, size, content_type, description, user_id, created_at, updated_at) VALUES (:path, :size, :content_type, :description, :user_id, :created_at, :updated_at)", file)
	if err != nil {
		var (
			sqliteErr *sqlite.Error
			pgErr     *pgconn.PgError
		)
		if errors.As(err, &sqliteErr) || errors.As(err, &pgErr) {
			if (sqliteErr != nil && sqliteErr.Code() == 1555) || (pgErr != nil && pgErr.Code == "23505") {
				err = ErrFileAlreadyExists
			}
		}
		if txErr := tx.Rollback(); txErr != nil {
			slog.ErrorContext(ctx, "error rolling back transaction", slog.Any("err", txErr))
		}
		return nil, nil, fmt.Errorf("error creating file: %w", err)
	}

	return file, tx, nil
}

func (d *DB) CreateOrUpdateFile(ctx context.Context, path string, size int64, contentType string, description string, userID string) (*File, *sqlx.Tx, error) {
	tx, err := d.dbx.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating file: %w", err)
	}

	file := &File{
		Path:        path,
		Size:        size,
		ContentType: contentType,
		Description: description,
		UserID:      userID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	_, err = tx.NamedExecContext(ctx, "INSERT INTO files (path, size, content_type, description, user_id, created_at, updated_at) VALUES (:path, :size, :content_type, :description, :user_id, :created_at, :updated_at) ON CONFLICT (path) DO UPDATE SET size = :size, content_type = :content_type, description = :description, user_id = :user_id, updated_at = :updated_at", file)
	if err != nil {
		if txErr := tx.Rollback(); txErr != nil {
			slog.ErrorContext(ctx, "error rolling back transaction", slog.Any("err", txErr))
		}
		return nil, nil, fmt.Errorf("error creating file: %w", err)
	}

	return file, tx, nil
}

func (d *DB) UpdateFile(ctx context.Context, path string, newPath string, size int64, contentType string, description string) (*sqlx.Tx, error) {
	tx, err := d.dbx.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("error updating file: %w", err)
	}

	file := &UpdateFile{
		Path:        path,
		NewPath:     newPath,
		Size:        size,
		ContentType: contentType,
		Description: description,
		UpdatedAt:   time.Now(),
	}
	query := "UPDATE files SET path = :new_path, description = :description, updated_at = :updated_at WHERE path = :path"
	if size > 0 {
		query = "UPDATE files SET path = :new_path, size = :size, content_type = :content_type, description = :description, updated_at = :updated_at WHERE path = :path"
	}

	res, err := d.dbx.NamedExecContext(ctx, query, file)
	if err != nil {
		if txErr := tx.Rollback(); txErr != nil {
			slog.ErrorContext(ctx, "error rolling back transaction", slog.Any("err", txErr))
		}
		return nil, fmt.Errorf("error updating file: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		if txErr := tx.Rollback(); txErr != nil {
			slog.ErrorContext(ctx, "error rolling back transaction", slog.Any("err", txErr))
		}
		return nil, ErrFileNotFound
	}
	return tx, nil
}

func (d *DB) DeleteFile(ctx context.Context, path string) (*sql.Tx, error) {
	tx, err := d.dbx.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("error deleting file: %w", err)
	}

	res, err := tx.ExecContext(ctx, "DELETE FROM files WHERE path = $1", path)
	if err != nil {
		if txErr := tx.Rollback(); txErr != nil {
			slog.ErrorContext(ctx, "error rolling back transaction", slog.Any("err", txErr))
		}
		return nil, fmt.Errorf("error deleting file: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		if txErr := tx.Rollback(); txErr != nil {
			slog.ErrorContext(ctx, "error rolling back transaction", slog.Any("err", txErr))
		}
		return nil, ErrFileNotFound
	}

	return tx.Tx, nil
}
