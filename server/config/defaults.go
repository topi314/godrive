package config

import (
	"strings"
	"time"

	"github.com/topi314/godrive/server/acl"
)

const (
	DefaultListenAddr     = ":80"
	DefaultFrontendURL    = "http://localhost:3000"
	DefaultSessionTTL     = 15 * time.Minute
	DefaultRefreshTTL     = 30 * 24 * time.Hour
	DefaultS3Region       = "us-east-1"
	DefaultSQLitePath     = "godrive.db"
	DefaultPostgresHost   = "localhost"
	DefaultPostgresPort   = 5432
	DefaultPostgresUser   = "godrive"
	DefaultPostgresDB     = "godrive"
	DefaultPostgresSSL    = "disable"
	DefaultStoragePath    = "/var/lib/godrive/storage"
	DefaultSyncInterval   = 15 * time.Minute
	DefaultS3SyncInterval = 1 * time.Minute
	DefaultLogFormat      = "text"
	DefaultUploadMax      = int64(50_000_000_000) // 50GB
	DefaultUploadChunk    = int64(16_000_000)     // 16MB
	DefaultUploadTTL      = 2 * time.Hour
	DefaultUploadParallel = 6
)

func applyDefaults(cfg *Config) {
	if s := strings.TrimSpace(cfg.Server.ListenAddr); s != "" {
		cfg.Server.ListenAddr = s
	} else {
		cfg.Server.ListenAddr = DefaultListenAddr
	}

	if frontend := strings.TrimRight(strings.TrimSpace(cfg.Server.FrontendURL), "/"); frontend != "" {
		cfg.Server.FrontendURL = frontend
	} else {
		cfg.Server.FrontendURL = DefaultFrontendURL
	}

	if cfg.AuthEnabled() {
		if cfg.Auth.SessionLifespan.Duration <= 0 {
			cfg.Auth.SessionLifespan.Duration = DefaultSessionTTL
		}
		if cfg.Auth.RefreshTokenLifespan.Duration <= 0 {
			cfg.Auth.RefreshTokenLifespan.Duration = DefaultRefreshTTL
		}
		cfg.Auth.PostLogoutRedirectURL = strings.TrimSpace(cfg.Auth.PostLogoutRedirectURL)
		cfg.Auth.RedirectURL = strings.TrimSpace(cfg.Auth.RedirectURL)
		cfg.Auth.Issuer = strings.TrimSpace(cfg.Auth.Issuer)
	}

	if s := strings.TrimSpace(cfg.Log.Format); s != "" {
		cfg.Log.Format = strings.ToLower(s)
	} else {
		cfg.Log.Format = DefaultLogFormat
	}
	if cfg.Database.Type == "" {
		cfg.Database.Type = DatabaseTypeSQLite
	}
	if strings.TrimSpace(cfg.Database.SQLite.Path) == "" {
		cfg.Database.SQLite.Path = DefaultSQLitePath
	}
	if strings.TrimSpace(cfg.Database.Postgres.Host) == "" {
		cfg.Database.Postgres.Host = DefaultPostgresHost
	}
	if cfg.Database.Postgres.Port == 0 {
		cfg.Database.Postgres.Port = DefaultPostgresPort
	}
	if strings.TrimSpace(cfg.Database.Postgres.Username) == "" {
		cfg.Database.Postgres.Username = DefaultPostgresUser
	}
	if strings.TrimSpace(cfg.Database.Postgres.Database) == "" {
		cfg.Database.Postgres.Database = DefaultPostgresDB
	}
	if strings.TrimSpace(cfg.Database.Postgres.SSLMode) == "" {
		cfg.Database.Postgres.SSLMode = DefaultPostgresSSL
	}
	if cfg.Storage.Type == "" {
		cfg.Storage.Type = StorageTypeLocal
	}
	if strings.TrimSpace(cfg.Storage.Local.Path) == "" {
		cfg.Storage.Local.Path = DefaultStoragePath
	}
	if cfg.Storage.S3.Region == "" {
		cfg.Storage.S3.Region = DefaultS3Region
	}
	if cfg.Storage.SyncInterval == nil {
		d := DefaultSyncInterval
		if cfg.Storage.Type == StorageTypeS3 {
			d = DefaultS3SyncInterval
		}
		cfg.Storage.SyncInterval = &Duration{Duration: d}
	}
	if cfg.Upload.MaxSize.Bytes <= 0 {
		cfg.Upload.MaxSize.Bytes = DefaultUploadMax
	}
	if cfg.Upload.ChunkSize.Bytes <= 0 {
		cfg.Upload.ChunkSize.Bytes = DefaultUploadChunk
	}
	if cfg.Upload.SessionTTL.Duration <= 0 {
		cfg.Upload.SessionTTL.Duration = DefaultUploadTTL
	}
	if cfg.Upload.MaxParallel <= 0 {
		cfg.Upload.MaxParallel = DefaultUploadParallel
	}
}

func ExpandHome(tmpl, username, email, subject string) string {
	tmpl = strings.TrimSpace(tmpl)
	if tmpl == "" {
		return "/"
	}
	user := sanitizeHomeComponent(username)
	if user == "" {
		user = sanitizeHomeComponent(email)
	}
	if user == "" {
		user = sanitizeHomeComponent(subject)
	}
	if user == "" {
		user = "user"
	}
	repl := strings.NewReplacer(
		"{user}", user,
		"{username}", user,
		"{email}", sanitizeHomeComponent(email),
		"{id}", sanitizeHomeComponent(subject),
	)
	return acl.NormalizePath(repl.Replace(tmpl))
}

func sanitizeHomeComponent(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "/")
	if s == "" || s == "." || s == ".." || strings.ContainsAny(s, "/\\") {
		return ""
	}
	return s
}
