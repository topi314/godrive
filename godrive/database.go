package godrive

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jackc/pgx/v5/tracelog"
	"github.com/jmoiron/sqlx"
	"log"
	"modernc.org/sqlite"
	_ "modernc.org/sqlite"
	"path"
	"time"
)

var (
	ErrFileNotFound      = errors.New("file not found")
	ErrFileAlreadyExists = errors.New("file already exists")
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

type File struct {
	Dir         string    `db:"dir"`
	Name        string    `db:"name"`
	ID          string    `db:"id"`
	Size        uint64    `db:"size"`
	ContentType string    `db:"content_type"`
	Description string    `db:"description"`
	Private     bool      `db:"private"`
	Owner       string    `db:"owner"`
	Username    *string   `db:"username"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

type UpdateFile struct {
	Dir         string    `db:"dir"`
	Name        string    `db:"name"`
	NewDir      string    `db:"new_dir"`
	NewName     string    `db:"new_name"`
	Size        uint64    `db:"size"`
	ContentType string    `db:"content_type"`
	Description string    `db:"description"`
	Private     bool      `db:"private"`
	UpdatedAt   time.Time `db:"updated_at"`
}

type User struct {
	ID       string `db:"id"`
	Username string `db:"username"`
	Email    string `db:"email"`
	Home     string `db:"home"`
}

func NewDB(ctx context.Context, cfg DatabaseConfig, schema string) (*DB, error) {
	var (
		driverName     string
		dataSourceName string
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
					log.Println(msg, data)
				}),
				LogLevel: tracelog.LogLevelDebug,
			}
		}
		dataSourceName = stdlib.RegisterConnConfig(pgCfg)
	case DatabaseTypeSQLite:
		driverName = "sqlite"
		dataSourceName = cfg.Path
	default:
		return nil, errors.New("invalid database type, must be one of: postgres, sqlite")
	}
	dbx, err := sqlx.ConnectContext(ctx, driverName, dataSourceName)
	if err != nil {
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

func (d *DB) FindFiles(ctx context.Context, fullName string) ([]File, error) {
	dir, name := path.Split(fullName)
	dir = path.Clean(dir)

	var file File
	err := d.dbx.GetContext(ctx, &file, "SELECT files.*, users.username FROM files LEFT JOIN users ON files.owner = users.id WHERE files.dir = $1 AND files.name = $2", dir, name)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("error finding file: %w", err)
	} else if err == nil {
		return []File{file}, nil
	}

	var files []File
	err = d.dbx.SelectContext(ctx, &files, "SELECT files.*, users.username FROM files LEFT JOIN users ON files.owner = users.id WHERE files.dir like $1", fullName+"%")
	if err != nil {
		return nil, fmt.Errorf("error finding files: %w", err)
	}

	return files, nil
}

func (d *DB) GetFile(ctx context.Context, path string, name string) (*File, error) {
	file := new(File)
	err := d.dbx.GetContext(ctx, file, "SELECT files.*, users.username FROM files LEFT JOIN users ON files.owner = users.id WHERE files.dir = $1 AND files.name = $2", path, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrFileNotFound
		}
		return nil, fmt.Errorf("error getting file: %w", err)
	}

	return file, nil
}

func (d *DB) CreateFile(ctx context.Context, dir string, name string, id string, size uint64, contentType string, description string, private bool, owner string) (*File, error) {
	file := &File{
		Dir:         dir,
		Name:        name,
		ID:          id,
		Size:        size,
		ContentType: contentType,
		Description: description,
		Private:     private,
		Owner:       owner,
		CreatedAt:   time.Now(),
	}
	_, err := d.dbx.NamedExecContext(ctx, "INSERT INTO files (dir, name, id, size, content_type, description, private, owner, created_at, updated_at) VALUES (:dir, :name, :id, :size, :content_type, :description, :private, :owner, :created_at, :updated_at)", file)
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
		return nil, fmt.Errorf("error creating file: %w", err)
	}

	return file, nil
}

func (d *DB) UpdateFile(ctx context.Context, dir string, name string, newDir string, newName string, size uint64, contentType string, description string, private bool) (string, error) {
	file := &UpdateFile{
		Dir:         dir,
		Name:        name,
		NewDir:      newDir,
		NewName:     newName,
		Size:        size,
		ContentType: contentType,
		Description: description,
		Private:     private,
		UpdatedAt:   time.Now(),
	}
	query := "UPDATE files SET dir = :new_dir, name = :new_name, description = :description, private = :private, updated_at = :updated_at WHERE name = :name AND dir = :dir RETURNING id"
	if size > 0 {
		query = "UPDATE files SET dir = :new_dir, name = :new_name, size = :size, content_type = :content_type, description = :description, private = :private, updated_at = :updated_at WHERE name = :name AND dir = :dir RETURNING id"
	}
	var id string
	if err := d.dbx.GetContext(ctx, &id, query, file); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrFileNotFound
		}
		return "", fmt.Errorf("error updating file: %w", err)
	}
	return id, nil
}

func (d *DB) DeleteFile(ctx context.Context, dir string, name string) (string, error) {
	var id string
	if err := d.dbx.GetContext(ctx, &id, "DELETE FROM files WHERE dir = $1 AND name = $2 RETURNING id", dir, name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrFileNotFound
		}
		return "", fmt.Errorf("error deleting file: %w", err)
	}

	return id, nil
}

func (d *DB) UpsertUser(ctx context.Context, id string, username string, email string, home string) error {
	user := &User{
		ID:       id,
		Username: username,
		Email:    email,
		Home:     home,
	}
	_, err := d.dbx.NamedExecContext(ctx, "INSERT INTO users (id, username, email, home) VALUES (:id, :username, :email, :home) ON CONFLICT (users.id) DO UPDATE SET username = :username, email = :email WHERE id = :id", user)
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
