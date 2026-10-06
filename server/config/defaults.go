package config

import (
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	DefaultListenAddr   = ":80"
	DefaultFrontendURL  = "http://localhost:3000"
	DefaultSessionTTL   = 15 * time.Minute
	DefaultRefreshTTL   = 30 * 24 * time.Hour
	DefaultS3Region     = "us-east-1"
	DefaultSQLitePath   = "godrive.db"
	DefaultPostgresHost = "localhost"
	DefaultPostgresPort = 5432
	DefaultPostgresUser = "godrive"
	DefaultPostgresDB   = "godrive"
	DefaultPostgresSSL  = "disable"
	DefaultStoragePath  = "/var/lib/godrive/storage"
	DefaultLogFormat    = "text"
	DefaultUploadMax    = int64(50_000_000_000) // 50GB
	DefaultUploadChunk  = int64(16_000_000)     // 16MB
	DefaultUploadTTL    = 72 * time.Hour
	DefaultUploadParallel = 2
)

func applyDefaults(cfg *Config) {
	if s := strings.TrimSpace(cfg.ListenAddr); s != "" {
		cfg.ListenAddr = s
	} else {
		cfg.ListenAddr = DefaultListenAddr
	}

	frontend := strings.TrimRight(strings.TrimSpace(cfg.FrontendURL), "/")
	if cfg.Auth != nil {
		if cfg.Auth.SessionLifespan.Duration <= 0 {
			cfg.Auth.SessionLifespan.Duration = DefaultSessionTTL
		}
		if cfg.Auth.RefreshTokenLifespan.Duration <= 0 {
			cfg.Auth.RefreshTokenLifespan.Duration = DefaultRefreshTTL
		}
		if post := strings.TrimSpace(cfg.Auth.PostLogoutRedirectURL); post != "" {
			cfg.Auth.PostLogoutRedirectURL = post
		} else if frontend != "" {
			cfg.Auth.PostLogoutRedirectURL = frontend + "/"
		} else if u, err := url.Parse(strings.TrimSpace(cfg.Auth.RedirectURL)); err == nil && strings.TrimSpace(cfg.Auth.RedirectURL) != "" {
			u.Path = "/"
			u.RawQuery = ""
			u.Fragment = ""
			cfg.Auth.PostLogoutRedirectURL = u.String()
		} else {
			cfg.Auth.PostLogoutRedirectURL = "/"
		}
	}
	if frontend == "" {
		frontend = DefaultFrontendURL
	}
	cfg.FrontendURL = frontend

	if s := strings.TrimSpace(cfg.Log.Format); s != "" {
		cfg.Log.Format = strings.ToLower(s)
	} else {
		cfg.Log.Format = DefaultLogFormat
	}
	if cfg.Database.Type == "" {
		cfg.Database.Type = DatabaseTypeSQLite
	}
	if strings.TrimSpace(cfg.Database.Path) == "" {
		cfg.Database.Path = DefaultSQLitePath
	}
	if strings.TrimSpace(cfg.Database.Host) == "" {
		cfg.Database.Host = DefaultPostgresHost
	}
	if cfg.Database.Port == 0 {
		cfg.Database.Port = DefaultPostgresPort
	}
	if strings.TrimSpace(cfg.Database.Username) == "" {
		cfg.Database.Username = DefaultPostgresUser
	}
	if strings.TrimSpace(cfg.Database.Database) == "" {
		cfg.Database.Database = DefaultPostgresDB
	}
	if strings.TrimSpace(cfg.Database.SSLMode) == "" {
		cfg.Database.SSLMode = DefaultPostgresSSL
	}
	if cfg.Storage.Type == "" {
		cfg.Storage.Type = StorageTypeLocal
	}
	if strings.TrimSpace(cfg.Storage.Path) == "" {
		cfg.Storage.Path = DefaultStoragePath
	}
	if cfg.Storage.Region == "" {
		cfg.Storage.Region = DefaultS3Region
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
	return normalizeHomePath(repl.Replace(tmpl))
}

func sanitizeHomeComponent(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "/")
	if s == "" || s == "." || s == ".." || strings.ContainsAny(s, "/\\") {
		return ""
	}
	return s
}

func normalizeHomePath(p string) string {
	if p == "" {
		return "/"
	}
	p = path.Clean("/" + strings.TrimPrefix(p, "/"))
	if p == "." {
		return "/"
	}
	return p
}
