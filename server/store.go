package server

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/topi314/godrive/server/database/dbsqlc"
	"github.com/topi314/gomigrate"
	gomigratepg "github.com/topi314/gomigrate/drivers/postgres"
	gomigratesqlite "github.com/topi314/gomigrate/drivers/sqlite"
	_ "modernc.org/sqlite"
)

type Store struct {
	DB *sql.DB
	Q  dbsqlc.Querier
}

func NewStore(ctx context.Context, cfg DatabaseConfig, migrations fs.FS) (*Store, error) {
	var (
		sqlDB *sql.DB
		err   error
	)

	switch cfg.Type {
	case DatabaseTypeSQLite:
		sqlDB, err = sql.Open("sqlite", cfg.Path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
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
	case DatabaseTypePostgres:
		pgCfg, err := pgx.ParseConfig(cfg.PostgresDataSourceName())
		if err != nil {
			return nil, fmt.Errorf("parse postgres config: %w", err)
		}
		sqlDB = stdlib.OpenDB(*pgCfg)
		if err = sqlDB.PingContext(ctx); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("ping postgres: %w", err)
		}
		if err = gomigrate.Migrate(ctx, sqlDB, gomigratepg.New, migrations,
			gomigrate.WithDirectory("migrations/postgres"),
			gomigrate.WithLogger(slog.Default()),
		); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("migrate postgres: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}

	if err = sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Store{DB: sqlDB, Q: dbsqlc.New(sqlDB)}, nil
}

func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}
	return s.DB.Close()
}

func (s *Store) ListACLByPaths(ctx context.Context, paths []string) ([]ACLRow, error) {
	rows, err := s.Q.ListACLByPaths(ctx, paths)
	if err != nil {
		return nil, err
	}
	out := make([]ACLRow, len(rows))
	for i, r := range rows {
		out[i] = ACLRow{
			Path:          r.Path,
			PrincipalType: r.PrincipalType,
			PrincipalID:   r.PrincipalID,
			Allow:         Permissions(r.Allow),
			Deny:          Permissions(r.Deny),
		}
	}
	return out, nil
}

func nullString(s *string) sql.NullString {
	if s == nil || *s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func nullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}
