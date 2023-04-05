package godrive

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"path"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jackc/pgx/v5/tracelog"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
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
	Path        string    `db:"path"`
	Name        string    `db:"name"`
	Size        int64     `db:"size"`
	ContentType string    `db:"content_type"`
	Description string    `db:"description"`
	Private     bool      `db:"private"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

func (f File) FullName() string {
	return path.Join(f.Path, f.Name)
}

type DB struct {
	dbx *sqlx.DB
}

func (d *DB) Close() error {
	return d.dbx.Close()
}

func (d *DB) GetFiles(ctx context.Context, path string) ([]File, error) {
	pathRegex := fmt.Sprintf("^%s[a-zA-Z0-9_-]*$", path)
	var files []File
	err := d.dbx.SelectContext(ctx, &files, "SELECT * FROM files WHERE path ~ $1", pathRegex)
	if err != nil {
		return nil, fmt.Errorf("error getting files: %w", err)
	}

	return files, nil
}

func (d *DB) GetFile(ctx context.Context, path string, name string) (*File, error) {
	file := new(File)
	err := d.dbx.GetContext(ctx, file, "SELECT * FROM files WHERE path = $1 AND name = $2", path, name)
	if err != nil {
		return nil, fmt.Errorf("error getting file: %w", err)
	}

	return file, nil
}

func (d *DB) CreateFile(ctx context.Context, path string, name string, size int64, contentType string, description string, private bool) (*File, error) {
	file := &File{
		Path:        path,
		Name:        name,
		Size:        size,
		ContentType: contentType,
		Description: description,
		Private:     private,
		CreatedAt:   time.Now(),
	}
	_, err := d.dbx.NamedExecContext(ctx, "INSERT INTO files (path, name, size, content_type, description, private, created_at, updated_at) VALUES (:path, :name, :size, :content_type, :description, :private, :created_at, :updated_at)", file)
	if err != nil {
		return nil, fmt.Errorf("error creating file: %w", err)
	}

	return file, nil
}

func (d *DB) UpdateFile(ctx context.Context, path string, name string, size int64, contentType string, description string, private bool) error {
	file := &File{
		Path:        path,
		Name:        name,
		Size:        size,
		ContentType: contentType,
		Description: description,
		Private:     private,
		UpdatedAt:   time.Now(),
	}
	_, err := d.dbx.NamedExecContext(ctx, "UPDATE files SET size = :size, content_type = :content_type, description = :description, private = :private, updated_at = :updated_at WHERE path = :path AND name = :name", file)
	if err != nil {
		return fmt.Errorf("error updating file: %w", err)
	}
	return nil
}

func (d *DB) DeleteFile(ctx context.Context, path string, name string) error {
	_, err := d.dbx.ExecContext(ctx, "DELETE FROM files WHERE path = $1 AND name = $2", path, name)
	if err != nil {
		return fmt.Errorf("error deleting file: %w", err)
	}

	return nil
}
