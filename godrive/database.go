package godrive

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgconn"
	"log"
	"modernc.org/sqlite"
	"path"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jackc/pgx/v5/tracelog"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

var (
	ErrFileNotFound      = errors.New("file not found")
	ErrFileAlreadyExists = errors.New("file already exists")
)

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

type File struct {
	Dir         string    `db:"dir"`
	Name        string    `db:"name"`
	Size        uint64    `db:"size"`
	ContentType string    `db:"content_type"`
	Description string    `db:"description"`
	Private     bool      `db:"private"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
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
	err := d.dbx.GetContext(ctx, &file, "SELECT * FROM files WHERE dir = $1 AND name = $2", dir, name)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("error finding file: %w", err)
	} else if err == nil {
		return []File{file}, nil
	}

	var files []File
	err = d.dbx.SelectContext(ctx, &files, "SELECT * FROM files WHERE dir like $1", fullName+"%")
	if err != nil {
		return nil, fmt.Errorf("error finding files: %w", err)
	}

	return files, nil
}

func (d *DB) GetFile(ctx context.Context, path string, name string) (*File, error) {
	file := new(File)
	err := d.dbx.GetContext(ctx, file, "SELECT * FROM files WHERE dir = $1 AND name = $2", path, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrFileNotFound
		}
		return nil, fmt.Errorf("error getting file: %w", err)
	}

	return file, nil
}

func (d *DB) CreateFile(ctx context.Context, dir string, name string, size uint64, contentType string, description string, private bool) (*File, error) {
	file := &File{
		Dir:         dir,
		Name:        name,
		Size:        size,
		ContentType: contentType,
		Description: description,
		Private:     private,
		CreatedAt:   time.Now(),
	}
	_, err := d.dbx.NamedExecContext(ctx, "INSERT INTO files (dir, name, size, content_type, description, private, created_at, updated_at) VALUES (:dir, :name, :size, :content_type, :description, :private, :created_at, :updated_at)", file)
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

func (d *DB) UpdateFile(ctx context.Context, dir string, name string, size uint64, contentType string, description string, private bool) error {
	file := &File{
		Dir:         dir,
		Name:        name,
		Size:        size,
		ContentType: contentType,
		Description: description,
		Private:     private,
		UpdatedAt:   time.Now(),
	}
	_, err := d.dbx.NamedExecContext(ctx, "UPDATE files SET size = :size, content_type = :content_type, description = :description, private = :private, updated_at = :updated_at WHERE dir = :dir AND name = :name", file)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrFileNotFound
		}
		return fmt.Errorf("error updating file: %w", err)
	}
	return nil
}

func (d *DB) DeleteFile(ctx context.Context, dir string, name string) error {
	_, err := d.dbx.ExecContext(ctx, "DELETE FROM files WHERE dir = $1 AND name = $2", dir, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrFileNotFound
		}
		return fmt.Errorf("error deleting file: %w", err)
	}

	return nil
}
