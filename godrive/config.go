package godrive

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Log        LogConfig      `toml:"log"`
	DevMode    bool           `toml:"dev_mode"`
	Debug      bool           `toml:"debug"`
	ListenAddr string         `toml:"listen_addr"`
	Database   DatabaseConfig `toml:"database"`
	Storage    StorageConfig  `toml:"storage"`
	Auth       *AuthConfig    `toml:"auth"`
	Otel       *OtelConfig    `toml:"otel"`
}

func DefaultConfig() Config {
	return Config{
		ListenAddr: ":80",
		DevMode:    false,
		Debug:      false,
		Log: LogConfig{
			Level:  slog.LevelInfo,
			Format: "text",
		},
		Database: DatabaseConfig{
			Type:     DatabaseTypeSQLite,
			Path:     "godrive.db",
			Host:     "localhost",
			Port:     5432,
			Username: "godrive",
			Database: "godrive",
			SSLMode:  "disable",
		},
		Storage: StorageConfig{
			Type: StorageTypeLocal,
			Path: "/var/lib/godrive/storage",
		},
	}
}

func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()

	configPath := path
	if configPath == "" {
		for _, candidate := range []string{"godrive.toml", "/etc/godrive/godrive.toml"} {
			if _, err := os.Stat(candidate); err == nil {
				configPath = candidate
				break
			}
		}
	}
	if configPath == "" {
		return cfg, fmt.Errorf("no config file found (looked for godrive.toml and /etc/godrive/godrive.toml)")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	applyEnvOverrides(&cfg)
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("GODRIVE_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("GODRIVE_DEV_MODE"); v != "" {
		cfg.DevMode = v == "true" || v == "1"
	}
	if v := os.Getenv("GODRIVE_DEBUG"); v != "" {
		cfg.Debug = v == "true" || v == "1"
	}
	if v := os.Getenv("GODRIVE_DATABASE_TYPE"); v != "" {
		cfg.Database.Type = DatabaseType(v)
	}
	if v := os.Getenv("GODRIVE_DATABASE_PATH"); v != "" {
		cfg.Database.Path = v
	}
	if v := os.Getenv("GODRIVE_DATABASE_HOST"); v != "" {
		cfg.Database.Host = v
	}
	if v := os.Getenv("GODRIVE_DATABASE_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Database.Port = n
		}
	}
	if v := os.Getenv("GODRIVE_DATABASE_USERNAME"); v != "" {
		cfg.Database.Username = v
	}
	if v := os.Getenv("GODRIVE_DATABASE_PASSWORD"); v != "" {
		cfg.Database.Password = v
	}
	if v := os.Getenv("GODRIVE_DATABASE_DATABASE"); v != "" {
		cfg.Database.Database = v
	}
	if v := os.Getenv("GODRIVE_DATABASE_SSL_MODE"); v != "" {
		cfg.Database.SSLMode = v
	}
	if v := os.Getenv("GODRIVE_STORAGE_TYPE"); v != "" {
		cfg.Storage.Type = StorageType(v)
	}
	if v := os.Getenv("GODRIVE_STORAGE_PATH"); v != "" {
		cfg.Storage.Path = v
	}
	if v := os.Getenv("GODRIVE_STORAGE_ENDPOINT"); v != "" {
		cfg.Storage.Endpoint = v
	}
	if v := os.Getenv("GODRIVE_STORAGE_ACCESS_KEY_ID"); v != "" {
		cfg.Storage.AccessKeyID = v
	}
	if v := os.Getenv("GODRIVE_STORAGE_SECRET_ACCESS_KEY"); v != "" {
		cfg.Storage.SecretAccessKey = v
	}
	if v := os.Getenv("GODRIVE_STORAGE_BUCKET"); v != "" {
		cfg.Storage.Bucket = v
	}
	if v := os.Getenv("GODRIVE_STORAGE_REGION"); v != "" {
		cfg.Storage.Region = v
	}
	if v := os.Getenv("GODRIVE_STORAGE_NOTIFY_WEBHOOK_SECRET"); v != "" {
		cfg.Storage.Notify.WebhookSecret = v
	}
	if v := os.Getenv("GODRIVE_STORAGE_NOTIFY_SQS_QUEUE_URL"); v != "" {
		cfg.Storage.Notify.SQSQueueURL = v
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
	return fmt.Sprintf("\n Log: %s\n DevMode: %t\n Debug: %t\n ListenAddr: %s\n Database: %s\n Storage: %s\n Auth: %s\n Otel: %s\n",
		c.Log, c.DevMode, c.Debug, c.ListenAddr, c.Database, c.Storage, c.Auth, c.Otel,
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
	Type  DatabaseType `toml:"type"`
	Debug bool         `toml:"debug"`

	Path string `toml:"path"`

	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	Username string `toml:"username"`
	Password string `toml:"password"`
	Database string `toml:"database"`
	SSLMode  string `toml:"ssl_mode"`
}

func (c DatabaseConfig) String() string {
	str := fmt.Sprintf("\n  Type: %s\n  Debug: %t\n  ", c.Type, c.Debug)
	switch c.Type {
	case DatabaseTypePostgres:
		str += fmt.Sprintf("Host: %s\n  Port: %d\n  Username: %s\n  Password: %s\n  Database: %s\n  SSLMode: %s",
			c.Host, c.Port, c.Username, strings.Repeat("*", len(c.Password)), c.Database, c.SSLMode)
	case DatabaseTypeSQLite:
		str += fmt.Sprintf("Path: %s", c.Path)
	default:
		str += "Invalid database type!"
	}
	return str
}

func (c DatabaseConfig) PostgresDataSourceName() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.Username, c.Password, c.Database, c.SSLMode)
}

type StorageType string

const (
	StorageTypeLocal StorageType = "local"
	StorageTypeS3    StorageType = "s3"
)

type StorageConfig struct {
	Type         StorageType       `toml:"type"`
	Debug        bool              `toml:"debug"`
	Path         string            `toml:"path"`
	Umask        int               `toml:"umask"`
	Endpoint     string            `toml:"endpoint"`
	AccessKeyID  string            `toml:"access_key_id"`
	SecretAccessKey string         `toml:"secret_access_key"`
	Bucket       string            `toml:"bucket"`
	Region       string            `toml:"region"`
	Secure       bool              `toml:"secure"`
	ForcePathStyle bool            `toml:"force_path_style"`
	SyncInterval Duration          `toml:"sync_interval"`
	Notify       StorageNotifyConfig `toml:"notify"`
}

type StorageNotifyConfig struct {
	WebhookSecret string `toml:"webhook_secret"`
	SQSQueueURL   string `toml:"sqs_queue_url"`
}

func (c StorageConfig) String() string {
	str := fmt.Sprintf("\n  Type: %s\n  Debug: %t\n  ", c.Type, c.Debug)
	switch c.Type {
	case StorageTypeLocal:
		str += fmt.Sprintf("Path: %s\n  Umask: %d", c.Path, c.Umask)
	case StorageTypeS3:
		str += fmt.Sprintf("Endpoint: %s\n  AccessKeyID: %s\n  SecretAccessKey: %s\n  Bucket: %s\n  Region: %s\n  Secure: %t",
			c.Endpoint, c.AccessKeyID, strings.Repeat("*", len(c.SecretAccessKey)), c.Bucket, c.Region, c.Secure)
	default:
		str += "Invalid storage type!"
	}
	return str
}

type AuthConfig struct {
	Secure               bool          `toml:"secure"`
	Issuer               string        `toml:"issuer"`
	ClientID             string        `toml:"client_id"`
	ClientSecret         string        `toml:"client_secret"`
	RedirectURL          string        `toml:"redirect_url"`
	RefreshTokenLifespan time.Duration `toml:"refresh_token_lifespan"`
	DefaultHome          string        `toml:"default_home"`
	Groups               AuthGroups    `toml:"groups"`
}

func (c AuthConfig) String() string {
	return fmt.Sprintf("\n  Secure: %t\n  Issuer: %s\n  ClientID: %s\n  ClientSecret: %s\n  RedirectURL: %s\n  RefreshTokenLifespan: %s\n  DefaultHome: %s\n  Groups: %s",
		c.Secure, c.Issuer, c.ClientID, strings.Repeat("*", len(c.ClientSecret)), c.RedirectURL, c.RefreshTokenLifespan, c.DefaultHome, c.Groups)
}

type AuthGroups struct {
	Admin  string `toml:"admin"`
	User   string `toml:"user"`
	Viewer string `toml:"viewer"`
	Guest  bool   `toml:"guest"`
}

func (c AuthGroups) String() string {
	return fmt.Sprintf("\n    Admin: %s\n    User: %s\n    Viewer: %s\n    Guest: %t", c.Admin, c.User, c.Viewer, c.Guest)
}

type OtelConfig struct {
	InstanceID string         `toml:"instance_id"`
	Trace      *TraceConfig   `toml:"trace"`
	Metrics    *MetricsConfig `toml:"metrics"`
}

func (c OtelConfig) String() string {
	return fmt.Sprintf("\n  InstanceID: %s\n  Trace: %s\n  Metrics: %s", c.InstanceID, c.Trace, c.Metrics)
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
