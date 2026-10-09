package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/topi314/godrive/server/acl"
	"github.com/topi314/godrive/server/config"
	"github.com/topi314/godrive/server/database/postgres"
	"github.com/topi314/godrive/server/database/sqlite"
	"github.com/topi314/gomigrate"
	gomigratepg "github.com/topi314/gomigrate/drivers/postgres"
	gomigratesqlite "github.com/topi314/gomigrate/drivers/sqlite"
	_ "modernc.org/sqlite"
)

//go:generate sqlc generate
//go:generate go run ../../tools/gen_db_bridge.go

type Store struct {
	Q        Querier
	sql      *sql.DB
	pg       *pgxpool.Pool
	postgres bool
}

func NewStore(ctx context.Context, cfg config.DatabaseConfig, migrations fs.FS) (*Store, error) {
	switch cfg.Type {
	case config.DatabaseTypeSQLite:
		sqlDB, err := sql.Open("sqlite", cfg.SQLite.Path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
		if err != nil {
			return nil, fmt.Errorf("open sqlite: %w", err)
		}
		sqlDB.SetMaxOpenConns(1)
		if err = gomigrate.Migrate(ctx, sqlDB, gomigratesqlite.New, migrations,
			gomigrate.WithDirectory("migrations/sqlite"),
			gomigrate.WithLogger(slog.Default()),
		); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("migrate sqlite: %w", err)
		}
		if err = sqlDB.PingContext(ctx); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("ping database: %w", err)
		}
		return &Store{sql: sqlDB, Q: sqlite.New(sqlDB)}, nil

	case config.DatabaseTypePostgres:
		pool, err := pgxpool.New(ctx, cfg.PostgresDataSourceName())
		if err != nil {
			return nil, fmt.Errorf("open postgres: %w", err)
		}
		if err = pool.Ping(ctx); err != nil {
			pool.Close()
			return nil, fmt.Errorf("ping postgres: %w", err)
		}
		// gomigrate still speaks database/sql; bridge the pool for migrations only.
		sqlDB := stdlib.OpenDBFromPool(pool)
		if err = gomigrate.Migrate(ctx, sqlDB, gomigratepg.New, migrations,
			gomigrate.WithDirectory("migrations/postgres"),
			gomigrate.WithLogger(slog.Default()),
		); err != nil {
			_ = sqlDB.Close()
			pool.Close()
			return nil, fmt.Errorf("migrate postgres: %w", err)
		}
		_ = sqlDB.Close()
		return &Store{pg: pool, postgres: true, Q: postgres.New(pool)}, nil

	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}
}

// WithTx runs fn inside a transaction bound to the active database engine.
func (s *Store) WithTx(ctx context.Context, fn func(q Querier) error) error {
	if s == nil {
		return fmt.Errorf("nil store")
	}
	if s.postgres {
		tx, err := s.pg.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if err := fn(postgres.New(tx)); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(sqlite.New(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	if s.pg != nil {
		s.pg.Close()
		return nil
	}
	if s.sql != nil {
		return s.sql.Close()
	}
	return nil
}

func (s *Store) ListACLByPaths(ctx context.Context, paths []string) ([]acl.ACLRow, error) {
	rows, err := s.Q.ListACLByPaths(ctx, paths)
	if err != nil {
		return nil, err
	}
	out := make([]acl.ACLRow, len(rows))
	for i, r := range rows {
		out[i] = acl.ACLRow{
			Path:          r.Path,
			PrincipalType: r.PrincipalType,
			PrincipalID:   r.PrincipalID,
			Allow:         acl.Permissions(r.Allow),
			Deny:          acl.Permissions(r.Deny),
		}
	}
	return out, nil
}

func NullString(s *string) sql.NullString {
	if s == nil || *s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func NullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}
