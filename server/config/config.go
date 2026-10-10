package config

import (
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Server   ServerConfig   `toml:"server"`
	Log      LogConfig      `toml:"log"`
	Database DatabaseConfig `toml:"database"`
	Storage  StorageConfig  `toml:"storage"`
	Auth     *AuthConfig    `toml:"auth"`
	Otel     *OtelConfig    `toml:"otel"`
	Upload   UploadConfig   `toml:"upload"`
}

type ServerConfig struct {
	ListenAddr  string `toml:"listen_addr"`
	FrontendURL string `toml:"frontend_url"` // Nuxt origin in -tags dev (e.g. http://localhost:3000)
}

func (c ServerConfig) String() string {
	return fmt.Sprintf("\n  ListenAddr: %s\n  FrontendURL: %s", c.ListenAddr, c.FrontendURL)
}

func LoadConfig(path string) (Config, error) {
	var cfg Config

	configPath := path
	if configPath == "" {
		for _, candidate := range []string{"config.toml", "/etc/godrive/config.toml"} {
			if _, err := os.Stat(candidate); err == nil {
				configPath = candidate
				break
			}
		}
	}
	if configPath == "" {
		return cfg, fmt.Errorf("no config file found (looked for config.toml and /etc/godrive/config.toml)")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	applyEnvOverrides(&cfg)
	applyDefaults(&cfg)
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("GODRIVE_LISTEN_ADDR"); v != "" {
		cfg.Server.ListenAddr = v
	}
	if v := os.Getenv("GODRIVE_FRONTEND_URL"); v != "" {
		cfg.Server.FrontendURL = v
	}
	if v := os.Getenv("GODRIVE_DATABASE_TYPE"); v != "" {
		cfg.Database.Type = DatabaseType(v)
	}
	if v := os.Getenv("GODRIVE_DATABASE_PATH"); v != "" {
		cfg.Database.SQLite.Path = v
	}
	if v := os.Getenv("GODRIVE_DATABASE_HOST"); v != "" {
		cfg.Database.Postgres.Host = v
	}
	if v := os.Getenv("GODRIVE_DATABASE_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Database.Postgres.Port = n
		}
	}
	if v := os.Getenv("GODRIVE_DATABASE_USERNAME"); v != "" {
		cfg.Database.Postgres.Username = v
	}
	if v := os.Getenv("GODRIVE_DATABASE_PASSWORD"); v != "" {
		cfg.Database.Postgres.Password = v
	}
	if v := os.Getenv("GODRIVE_DATABASE_DATABASE"); v != "" {
		cfg.Database.Postgres.Database = v
	}
	if v := os.Getenv("GODRIVE_DATABASE_SSL_MODE"); v != "" {
		cfg.Database.Postgres.SSLMode = v
	}
	if v := os.Getenv("GODRIVE_STORAGE_TYPE"); v != "" {
		cfg.Storage.Type = StorageType(v)
	}
	if v := os.Getenv("GODRIVE_STORAGE_PATH"); v != "" {
		cfg.Storage.Local.Path = v
	}
	if v := os.Getenv("GODRIVE_STORAGE_ENDPOINT"); v != "" {
		cfg.Storage.S3.Endpoint = v
	}
	if v := os.Getenv("GODRIVE_STORAGE_ACCESS_KEY_ID"); v != "" {
		cfg.Storage.S3.AccessKeyID = v
	}
	if v := os.Getenv("GODRIVE_STORAGE_SECRET_ACCESS_KEY"); v != "" {
		cfg.Storage.S3.SecretAccessKey = v
	}
	if v := os.Getenv("GODRIVE_STORAGE_BUCKET"); v != "" {
		cfg.Storage.S3.Bucket = v
	}
	if v := os.Getenv("GODRIVE_STORAGE_REGION"); v != "" {
		cfg.Storage.S3.Region = v
	}
	if v := os.Getenv("GODRIVE_STORAGE_NOTIFY_AUTH_TOKEN"); v != "" {
		cfg.Storage.S3.Notify.AuthToken = v
	} else if v := os.Getenv("GODRIVE_STORAGE_NOTIFY_WEBHOOK_SECRET"); v != "" {
		// Deprecated alias.
		cfg.Storage.S3.Notify.AuthToken = v
	}
	if cfg.Auth != nil {
		if v := os.Getenv("GODRIVE_AUTH_CLIENT_SECRET"); v != "" {
			cfg.Auth.ClientSecret = v
		}
		if v := os.Getenv("GODRIVE_AUTH_CLIENT_ID"); v != "" {
			cfg.Auth.ClientID = v
		}
		if v := os.Getenv("GODRIVE_AUTH_ISSUER"); v != "" {
			cfg.Auth.Issuer = v
		}
	}
}

func (c Config) String() string {
	return fmt.Sprintf("\n Server: %s\n Log: %s\n Database: %s\n Storage: %s\n Auth: %s\n Otel: %s\n",
		c.Server, c.Log, c.Database, c.Storage, c.Auth, c.Otel,
	)
}

type LogConfig struct {
	Level     slog.Level `toml:"level"`
	Format    string     `toml:"format"`
	AddSource bool       `toml:"add_source"`
}

func (c LogConfig) String() string {
	return fmt.Sprintf("\n  Level: %s\n  Format: %s\n  AddSource: %t\n", c.Level, c.Format, c.AddSource)
}

type DatabaseType string

const (
	DatabaseTypePostgres DatabaseType = "postgres"
	DatabaseTypeSQLite   DatabaseType = "sqlite"
)

type DatabaseConfig struct {
	Type     DatabaseType           `toml:"type"`
	SQLite   DatabaseSQLiteConfig   `toml:"sqlite"`
	Postgres DatabasePostgresConfig `toml:"postgres"`
}

type DatabaseSQLiteConfig struct {
	Path string `toml:"path"`
}

type DatabasePostgresConfig struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	Username string `toml:"username"`
	Password string `toml:"password"`
	Database string `toml:"database"`
	SSLMode  string `toml:"ssl_mode"`
}

func (c DatabaseConfig) String() string {
	str := fmt.Sprintf("\n  Type: %s\n  ", c.Type)
	switch c.Type {
	case DatabaseTypePostgres:
		str += fmt.Sprintf("Host: %s\n  Port: %d\n  Username: %s\n  Password: %s\n  Database: %s\n  SSLMode: %s",
			c.Postgres.Host, c.Postgres.Port, c.Postgres.Username, strings.Repeat("*", len(c.Postgres.Password)), c.Postgres.Database, c.Postgres.SSLMode)
	case DatabaseTypeSQLite:
		str += fmt.Sprintf("Path: %s", c.SQLite.Path)
	default:
		str += "Invalid database type!"
	}
	return str
}

func (c DatabaseConfig) PostgresDataSourceName() string {
	pg := c.Postgres
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		pg.Host, pg.Port, pg.Username, pg.Password, pg.Database, pg.SSLMode)
}

type StorageType string

const (
	StorageTypeLocal StorageType = "local"
	StorageTypeS3    StorageType = "s3"
)

type StorageConfig struct {
	Type         StorageType        `toml:"type"`
	SyncInterval *Duration          `toml:"sync_interval"` // nil → 15m local / 1m s3; "0" disables
	Local        StorageLocalConfig `toml:"local"`
	S3           StorageS3Config    `toml:"s3"`
}

type StorageLocalConfig struct {
	Path  string `toml:"path"`
	Umask int    `toml:"umask"`
}

type StorageS3Config struct {
	Endpoint        string                 `toml:"endpoint"`
	AccessKeyID     string                 `toml:"access_key_id"`
	SecretAccessKey string                 `toml:"secret_access_key"`
	Bucket          string                 `toml:"bucket"`
	Region          string                 `toml:"region"`
	Secure          bool                   `toml:"secure"`
	ForcePathStyle  bool                   `toml:"force_path_style"`
	Notify          StorageS3NotifyConfig  `toml:"notify"`
}

type StorageS3NotifyConfig struct {
	// AuthToken is checked against the Authorization header (MinIO notify_webhook auth_token).
	// Store the token only; requests must send "Authorization: Bearer <token>".
	AuthToken string `toml:"auth_token"`
}

func (c StorageConfig) String() string {
	str := fmt.Sprintf("\n  Type: %s\n  ", c.Type)
	switch c.Type {
	case StorageTypeLocal:
		str += fmt.Sprintf("Path: %s\n  Umask: %d", c.Local.Path, c.Local.Umask)
	case StorageTypeS3:
		str += fmt.Sprintf("Endpoint: %s\n  AccessKeyID: %s\n  SecretAccessKey: %s\n  Bucket: %s\n  Region: %s\n  Secure: %t",
			c.S3.Endpoint, c.S3.AccessKeyID, strings.Repeat("*", len(c.S3.SecretAccessKey)), c.S3.Bucket, c.S3.Region, c.S3.Secure)
	default:
		str += "Invalid storage type!"
	}
	return str
}

type AuthConfig struct {
	Enabled               bool       `toml:"enabled"`
	Secure                bool       `toml:"secure"`
	Issuer                string     `toml:"issuer"`
	ClientID              string     `toml:"client_id"`
	ClientSecret          string     `toml:"client_secret"`
	RedirectURL           string     `toml:"redirect_url"`
	SessionLifespan       Duration   `toml:"session_lifespan"`
	RefreshTokenLifespan  Duration   `toml:"refresh_token_lifespan"`
	PostLogoutRedirectURL string     `toml:"post_logout_redirect_url"`
	DefaultHome           string     `toml:"default_home"`
	Groups                AuthGroups `toml:"groups"`
}

// AuthEnabled reports whether OIDC auth and ACLs are active.
func (c Config) AuthEnabled() bool {
	return c.Auth != nil && c.Auth.Enabled
}

func (c AuthConfig) String() string {
	return fmt.Sprintf("\n  Enabled: %t\n  Secure: %t\n  Issuer: %s\n  ClientID: %s\n  ClientSecret: %s\n  RedirectURL: %s\n  SessionLifespan: %s\n  RefreshTokenLifespan: %s\n  DefaultHome: %s\n  Groups: %s",
		c.Enabled, c.Secure, c.Issuer, c.ClientID, strings.Repeat("*", len(c.ClientSecret)), c.RedirectURL, c.SessionLifespan.Duration, c.RefreshTokenLifespan.Duration, c.DefaultHome, c.Groups)
}

type AuthGroups struct {
	Admin  string            `toml:"admin"`  // OIDC group that grants full access
	Access string            `toml:"access"` // OIDC group required to use the app (admin also counts)
	Guest  bool              `toml:"guest"`
	Map    map[string]string `toml:"map"` // OIDC group → godrive group (ACL principals)
}

func (c AuthGroups) String() string {
	return fmt.Sprintf("\n    Admin: %s\n    Access: %s\n    Guest: %t\n    Map: %v", c.Admin, c.Access, c.Guest, c.Map)
}

// oidcMapping returns OIDC group → godrive group. admin and access identity-map
// unless overridden in Map.
func (c AuthGroups) oidcMapping() map[string]string {
	m := map[string]string{}
	if c.Admin != "" {
		m[c.Admin] = c.Admin
	}
	if c.Access != "" {
		m[c.Access] = c.Access
	}
	for oidc, gd := range c.Map {
		oidc = strings.TrimSpace(oidc)
		gd = strings.TrimSpace(gd)
		if oidc == "" {
			continue
		}
		m[oidc] = gd
	}
	return m
}

// MapOIDCGroups keeps only configured mappings, returning unique godrive names.
// Names that are already godrive group values (stored after a previous login) are kept.
func (c AuthGroups) MapOIDCGroups(oidcGroups []string) []string {
	mapping := c.oidcMapping()
	known := map[string]struct{}{}
	for _, gd := range mapping {
		if gd != "" {
			known[gd] = struct{}{}
		}
	}
	out := make([]string, 0, len(oidcGroups))
	seen := map[string]struct{}{}
	add := func(name string) {
		if name == "" {
			return
		}
		if _, dup := seen[name]; dup {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	for _, g := range oidcGroups {
		if mapped, ok := mapping[g]; ok {
			add(mapped)
			continue
		}
		if _, ok := known[g]; ok {
			add(g)
		}
	}
	return out
}

// AvailableGroups is the set of godrive group names usable in ACLs.
func (c AuthGroups) AvailableGroups() []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	add(c.AdminGroup())
	add(c.AccessGroup())
	var extra []string
	for _, v := range c.Map {
		extra = append(extra, v)
	}
	sort.Strings(extra)
	for _, v := range extra {
		add(v)
	}
	return out
}

func (c AuthGroups) AdminGroup() string {
	if c.Admin == "" {
		return ""
	}
	if v, ok := c.oidcMapping()[c.Admin]; ok && v != "" {
		return v
	}
	return c.Admin
}

func (c AuthGroups) AccessGroup() string {
	if c.Access == "" {
		return ""
	}
	if v, ok := c.oidcMapping()[c.Access]; ok && v != "" {
		return v
	}
	return c.Access
}

func (c AuthGroups) IsAvailableGroup(name string) bool {
	for _, g := range c.AvailableGroups() {
		if g == name {
			return true
		}
	}
	return false
}

type OtelConfig struct {
	Enabled    bool           `toml:"enabled"`
	InstanceID string         `toml:"instance_id"`
	Trace      *TraceConfig   `toml:"trace"`
	Metrics    *MetricsConfig `toml:"metrics"`
}

// OtelEnabled reports whether OpenTelemetry is active.
func (c Config) OtelEnabled() bool {
	return c.Otel != nil && c.Otel.Enabled
}

func (c OtelConfig) String() string {
	return fmt.Sprintf("\n  Enabled: %t\n  InstanceID: %s\n  Trace: %s\n  Metrics: %s", c.Enabled, c.InstanceID, c.Trace, c.Metrics)
}

type TraceConfig struct {
	Endpoint string `toml:"endpoint"`
	Insecure bool   `toml:"insecure"`
}

func (c TraceConfig) String() string {
	return fmt.Sprintf("\n   Endpoint: %s\n   Insecure: %t", c.Endpoint, c.Insecure)
}

type MetricsConfig struct {
	ListenAddr string `toml:"listen_addr"`
}

func (c MetricsConfig) String() string {
	return fmt.Sprintf("\n   ListenAddr: %s", c.ListenAddr)
}

// Duration wraps time.Duration for TOML string decoding ("15m", "0", etc.).
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalText(text []byte) error {
	s := strings.TrimSpace(string(text))
	if s == "" || s == "0" {
		d.Duration = 0
		return nil
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = parsed
	return nil
}

// ByteSize wraps int64 for TOML sizes like "50GB", "16MB", "1024".
type ByteSize struct {
	Bytes int64
}

func (b *ByteSize) UnmarshalText(text []byte) error {
	s := strings.TrimSpace(strings.ToUpper(string(text)))
	if s == "" || s == "0" {
		b.Bytes = 0
		return nil
	}
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "KB"):
		mult = 1000
		s = strings.TrimSpace(strings.TrimSuffix(s, "KB"))
	case strings.HasSuffix(s, "MB"):
		mult = 1000 * 1000
		s = strings.TrimSpace(strings.TrimSuffix(s, "MB"))
	case strings.HasSuffix(s, "GB"):
		mult = 1000 * 1000 * 1000
		s = strings.TrimSpace(strings.TrimSuffix(s, "GB"))
	case strings.HasSuffix(s, "TB"):
		mult = 1000 * 1000 * 1000 * 1000
		s = strings.TrimSpace(strings.TrimSuffix(s, "TB"))
	case strings.HasSuffix(s, "KIB"):
		mult = 1024
		s = strings.TrimSpace(strings.TrimSuffix(s, "KIB"))
	case strings.HasSuffix(s, "MIB"):
		mult = 1024 * 1024
		s = strings.TrimSpace(strings.TrimSuffix(s, "MIB"))
	case strings.HasSuffix(s, "GIB"):
		mult = 1024 * 1024 * 1024
		s = strings.TrimSpace(strings.TrimSuffix(s, "GIB"))
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n < 0 {
		return fmt.Errorf("invalid size %q", string(text))
	}
	b.Bytes = int64(n * float64(mult))
	return nil
}

// UploadConfig controls resumable / large-file uploads.
type UploadConfig struct {
	MaxSize     ByteSize `toml:"max_size"`
	ChunkSize   ByteSize `toml:"chunk_size"`
	SessionTTL  Duration `toml:"session_ttl"`
	MaxParallel int      `toml:"max_parallel"`
}
