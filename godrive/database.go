package godrive

import (
	"context"
	"database/sql"
	"database/sql/driver"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jackc/pgx/v5/tracelog"
	"github.com/jmoiron/sqlx"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.18.0"
	"modernc.org/sqlite"
	_ "modernc.org/sqlite"
)

var (
	ErrFileNotFound      = errors.New("file not found")
	ErrFileAlreadyExists = errors.New("file already exists")
	ErrUserNotFound      = errors.New("user not found")
	ErrSessionNotFound   = errors.New("session not found")
	ErrUnauthorized      = errors.New("unauthorized")
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

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

type User struct {
	ID       string `db:"id"`
	Username string `db:"username"`
	Groups   Groups `db:"groups"`
	Email    string `db:"email"`
	Home     string `db:"home"`
}

type Session struct {
	ID           string    `db:"id"`
	AccessToken  string    `db:"access_token"`
	Expiry       time.Time `db:"expiry"`
	RefreshToken string    `db:"refresh_token"`
	IDToken      string    `db:"id_token"`
}

type Groups []string

func (g *Groups) Scan(src any) error {
	if v, ok := src.(string); ok {
		*g = strings.Split(v, ",")
		return nil
	}
	return errors.New("invalid type for groups")

}

func (g Groups) Value() (driver.Value, error) {
	return strings.Join(g, ","), nil
}

func NewDB(ctx context.Context, cfg DatabaseConfig, schema string) (*DB, error) {
	var (
		driverName     string
		dataSourceName string
		dbSystem       attribute.KeyValue
	)
	switch cfg.Type {
	case DatabaseTypePostgres:
		driverName = "pgx"
		pgCfg, err := pgx.ParseConfig(cfg.PostgresDataSourceName())
		if err != nil {
			return nil, err
		}

		if cfg.Debug {
			pgCfg.Tracer = &tracelog.TraceLog{
				Logger: tracelog.LoggerFunc(func(ctx context.Context, level tracelog.LogLevel, msg string, data map[string]any) {
					args := make([]any, 0, len(data))
					for k, v := range data {
						args = append(args, slog.Any(k, v))
					}
					slog.DebugContext(ctx, msg, slog.Group("data", args...))
				}),
				LogLevel: tracelog.LogLevelDebug,
			}
		}
		dataSourceName = stdlib.RegisterConnConfig(pgCfg)
		dbSystem = semconv.DBSystemPostgreSQL
	case DatabaseTypeSQLite:
		driverName = "sqlite"
		dataSourceName = cfg.Path
		dbSystem = semconv.DBSystemSqlite
	default:
		return nil, errors.New("invalid database type, must be one of: postgres, sqlite")
	}

	sqlDB, err := otelsql.Open(driverName, dataSourceName,
		otelsql.WithAttributes(dbSystem),
		otelsql.WithSQLCommenter(true),
	)
	if err != nil {
		return nil, err
	}

	if err = otelsql.RegisterDBStatsMetrics(sqlDB, otelsql.WithAttributes(dbSystem)); err != nil {
		return nil, err
	}

	dbx := sqlx.NewDb(sqlDB, driverName)
	if err = dbx.PingContext(ctx); err != nil {
		return nil, err
	}
	// execute schema
	if _, err = dbx.ExecContext(ctx, schema); err != nil {
		return nil, err
	}

	db := &DB{
		dbx: dbx,
	}

	return db, nil
}

type DB struct {
	dbx *sqlx.DB
}

func (d *DB) Close() error {
	return d.dbx.Close()
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
