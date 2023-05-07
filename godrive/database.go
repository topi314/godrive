package godrive

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"path"
	"strings"
	"time"

	"golang.org/x/exp/slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jackc/pgx/v5/tracelog"
	"github.com/jmoiron/sqlx"
	"modernc.org/sqlite"
	_ "modernc.org/sqlite"
)

var (
	ErrFileNotFound      = errors.New("file not found")
	ErrFileAlreadyExists = errors.New("file already exists")
	ErrUserNotFound      = errors.New("user not found")
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

type File struct {
	Path        string    `db:"path"`
	Size        uint64    `db:"size"`
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
	Size        uint64    `db:"size"`
	ContentType string    `db:"content_type"`
	Description string    `db:"description"`
	UpdatedAt   time.Time `db:"updated_at"`
}

type FilePermissions struct {
	Path        string      `db:"path"`
	Permissions Permissions `db:"permissions"`
	ObjectType  ObjectType  `db:"object_type"`
	Object      string      `db:"object"`
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

func (d *DB) CreateFile(ctx context.Context, path string, size uint64, contentType string, description string, userID string) (*File, error) {
	file := &File{
		Path:        path,
		Size:        size,
		ContentType: contentType,
		Description: description,
		UserID:      userID,
		CreatedAt:   time.Now(),
	}
	_, err := d.dbx.NamedExecContext(ctx, "INSERT INTO files (path, size, content_type, description, user_id, created_at, updated_at) VALUES (:path, :size, :content_type, :description, :user_id, :created_at, :updated_at)", file)
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

func (d *DB) UpdateFile(ctx context.Context, path string, newPath string, size uint64, contentType string, description string) error {
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
		return fmt.Errorf("error updating file: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return ErrFileNotFound
	}
	return nil
}

func (d *DB) DeleteFile(ctx context.Context, path string) error {
	res, err := d.dbx.ExecContext(ctx, "DELETE FROM files WHERE path = $1", path)
	if err != nil {
		return fmt.Errorf("error deleting file: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return ErrFileNotFound
	}

	return nil
}

func (d *DB) GetFilePermissions(ctx context.Context, filePaths []string) ([]FilePermissions, error) {
	var paths []string
	for _, filePath := range filePaths {
		for {
			if !slices.Contains(paths, filePath) {
				paths = append(paths, filePath)
			}
			if filePath == "/" {
				break
			}
			filePath = path.Dir(filePath)
		}
	}

	query, args, err := sqlx.In("SELECT * FROM file_permissions WHERE path IN (?)", paths)
	if err != nil {
		return nil, err
	}
	var permissions []FilePermissions
	if err = d.dbx.SelectContext(ctx, &permissions, d.dbx.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("error getting path permissions: %w", err)
	}
	return permissions, nil
}

func (d *DB) GetAllFilePermissions(ctx context.Context) ([]FilePermissions, error) {
	var perms []FilePermissions
	if err := d.dbx.SelectContext(ctx, &perms, "SELECT * FROM file_permissions"); err != nil {
		return nil, fmt.Errorf("error getting all file permissions: %w", err)
	}
	return perms, nil
}

func (d *DB) UpsertFilePermission(ctx context.Context, path string, permissions Permissions, objectType ObjectType, object string) error {
	permission := FilePermissions{
		Path:        path,
		Permissions: permissions,
		ObjectType:  objectType,
		Object:      object,
	}
	if _, err := d.dbx.NamedExecContext(ctx, "INSERT INTO file_permissions (path, permissions, object_type, object) VALUES (:path, :permissions, :object_type, :object) ON CONFLICT (path, object_type, object) DO UPDATE SET permissions = :permissions", permission); err != nil {
		return fmt.Errorf("error upserting file permissions: %w", err)
	}
	return nil
}

func (d *DB) DeleteFilePermissions(ctx context.Context, path string, objectType ObjectType, object string) error {
	if _, err := d.dbx.ExecContext(ctx, "DELETE FROM file_permissions WHERE path = $1 AND object_type = $2 AND object = $3", path, objectType, object); err != nil {
		return fmt.Errorf("error deleting file permissions: %w", err)
	}
	return nil
}

func (d *DB) DeleteAllFilePermissions(ctx context.Context) error {
	if _, err := d.dbx.ExecContext(ctx, "DELETE FROM file_permissions"); err != nil {
		return fmt.Errorf("error deleting all file permissions: %w", err)
	}
	return nil
}

func (d *DB) UpsertUser(ctx context.Context, id string, username string, email string, home string) error {
	user := &User{
		ID:       id,
		Username: username,
		Email:    email,
		Home:     home,
	}
	query, args, err := sqlx.Named("INSERT INTO users (id, username, email, home) VALUES (:id, :username, :email, :home) ON CONFLICT (id) DO UPDATE SET username = :username, email = :email RETURNING home", user)
	if err != nil {
		return fmt.Errorf("error upserting user: %w", err)
	}
	if err = d.dbx.GetContext(ctx, &home, d.dbx.Rebind(query), args...); err != nil {
		return fmt.Errorf("error upserting user: %w", err)
	}
	return d.UpsertFilePermission(ctx, home, PermissionsAll, ObjectTypeUser, user.ID)
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
