package godrive

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/exp/slog"
)

type Config struct {
	Log         LogConfig      `cfg:"log"`
	DevMode     bool           `cfg:"dev_mode"`
	Debug       bool           `cfg:"debug"`
	ListenAddr  string         `cfg:"listen_addr"`
	CacheAssets bool           `cfg:"cache_assets"`
	Database    DatabaseConfig `cfg:"database"`
	Storage     StorageConfig  `cfg:"storage"`
	Auth        *AuthConfig    `cfg:"auth"`
	Otel        *OtelConfig    `cfg:"otel"`
}

func (c Config) String() string {
	return fmt.Sprintf("\n Log: %s\n DevMode: %t\n Debug: %t\n ListenAddr: %s\n CacheAssets: %t\n Database: %s\n Storage: %s\n Auth: %s\n Otel: %s\n",
		c.Log,
		c.DevMode,
		c.Debug,
		c.ListenAddr,
		c.CacheAssets,
		c.Database,
		c.Storage,
		c.Auth,
		c.Otel,
	)
}

type LogConfig struct {
	Level     slog.Level `cfg:"level"`
	Format    string     `cfg:"format"`
	AddSource bool       `cfg:"add_source"`
}

func (c LogConfig) String() string {
	return fmt.Sprintf("\n  Level: %s\n  Format: %s\n  AddSource: %t\n",
		c.Level,
		c.Format,
		c.AddSource,
	)
}

type DatabaseType string

const (
	DatabaseTypePostgres DatabaseType = "postgres"
	DatabaseTypeSQLite   DatabaseType = "sqlite"
)

type DatabaseConfig struct {
	Type  DatabaseType `cfg:"type"`
	Debug bool         `cfg:"debug"`

	// SQLite
	Path string `cfg:"path"`

	// PostgreSQL
	Host     string `cfg:"host"`
	Port     int    `cfg:"port"`
	Username string `cfg:"username"`
	Password string `cfg:"password"`
	Database string `cfg:"database"`
	SSLMode  string `cfg:"ssl_mode"`
}

func (c DatabaseConfig) String() string {
	str := fmt.Sprintf("\n  Type: %s\n  Debug: %t\n  ",
		c.Type,
		c.Debug,
	)
	switch c.Type {
	case "postgres":
		str += fmt.Sprintf("Host: %s\n  Port: %d\n  Username: %s\n  Password: %s\n  Database: %s\n  SSLMode: %s",
			c.Host,
			c.Port,
			c.Username,
			strings.Repeat("*", len(c.Password)),
			c.Database,
			c.SSLMode,
		)
	case "sqlite":
		str += fmt.Sprintf("Path: %s", c.Path)
	default:
		str += "Invalid database type!"
	}
	return str
}

func (c DatabaseConfig) PostgresDataSourceName() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host,
		c.Port,
		c.Username,
		c.Password,
		c.Database,
		c.SSLMode,
	)
}

type StorageType string

const (
	StorageTypeLocal StorageType = "local"
	StorageTypeS3    StorageType = "s3"
)

type StorageConfig struct {
	Type  StorageType `cfg:"type"`
	Debug bool        `cfg:"debug"`

	// Local
	Path  string `cfg:"path"`
	Umask int    `cfg:"umask"`

	// S3
	Endpoint        string `cfg:"endpoint"`
	AccessKeyID     string `cfg:"access_key_id"`
	SecretAccessKey string `cfg:"secret_access_key"`
	Bucket          string `cfg:"bucket"`
	Region          string `cfg:"region"`
	Secure          bool   `cfg:"secure"`
}

func (c StorageConfig) String() string {
	str := fmt.Sprintf("\n  Type: %s\n  Debug: %t\n  ", c.Type, c.Debug)
	switch c.Type {
	case "local":
		str += fmt.Sprintf("Path: %s\n  Umask: %d", c.Path, c.Umask)
	case "s3":
		str += fmt.Sprintf("Endpoint: %s\n  AccessKeyID: %s\n  SecretAccessKey: %s\n  Bucket: %s\n  Region: %s\n  Secure: %t",
			c.Endpoint,
			c.AccessKeyID,
			strings.Repeat("*", len(c.SecretAccessKey)),
			c.Bucket,
			c.Region,
			c.Secure,
		)
	default:
		str += "Invalid storage type!"
	}
	return str
}

type AuthConfig struct {
	Secure               bool          `cfg:"secure"`
	Issuer               string        `cfg:"issuer"`
	ClientID             string        `cfg:"client_id"`
	ClientSecret         string        `cfg:"client_secret"`
	RedirectURL          string        `cfg:"redirect_url"`
	RefreshTokenLifespan time.Duration `cfg:"refresh_token_lifespan"`
	DefaultHome          string        `cfg:"default_home"`
	Groups               AuthGroups    `cfg:"groups"`
}

func (c AuthConfig) String() string {
	return fmt.Sprintf("\n  Secure: %t\n  Issuer: %s\n  ClientID: %s\n  ClientSecret: %s\n  RedirectURL: %s\n  RefreshTokenLifespan: %s\n  DefaultHome: %s\n  Groups: %s",
		c.Secure,
		c.Issuer,
		c.ClientID,
		strings.Repeat("*", len(c.ClientSecret)),
		c.RedirectURL,
		c.RefreshTokenLifespan,
		c.DefaultHome,
		c.Groups,
	)
}

type AuthGroups struct {
	Admin string `cfg:"admin"`
	User  string `cfg:"user"`
	Guest string `cfg:"guest"`
}

func (c AuthGroups) String() string {
	return fmt.Sprintf("\n    Admin: %s\n    User: %s\n    Guest: %s",
		c.Admin,
		c.User,
		c.Guest,
	)
}

type OtelConfig struct {
	InstanceID string         `cfg:"instance_id"`
	Trace      *TraceConfig   `cfg:"trace"`
	Metrics    *MetricsConfig `cfg:"metrics"`
}

func (c OtelConfig) String() string {
	return fmt.Sprintf("\n  InstanceID: %s\n  Trace: %s\n  Metrics: %s",
		c.InstanceID,
		c.Trace,
		c.Metrics,
	)
}

type TraceConfig struct {
	Endpoint string `cfg:"endpoint"`
	Insecure bool   `cfg:"insecure"`
}

func (c TraceConfig) String() string {
	return fmt.Sprintf("\n   Endpoint: %s\n   Insecure: %t",
		c.Endpoint,
		c.Insecure,
	)
}

type MetricsConfig struct {
	ListenAddr string `cfg:"listen_addr"`
}

func (c MetricsConfig) String() string {
	return fmt.Sprintf("\n   ListenAddr: %s",
		c.ListenAddr,
	)
}
