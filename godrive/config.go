package godrive

import (
	"fmt"
	"strings"
)

type Config struct {
	DevMode    bool           `cfg:"dev_mode"`
	Debug      bool           `cfg:"debug"`
	ListenAddr string         `cfg:"listen_addr"`
	Database   DatabaseConfig `cfg:"database"`
	Storage    StorageConfig  `cfg:"storage"`
	JWTSecret  string         `cfg:"jwt_secret"`
}

func (c Config) String() string {
	return fmt.Sprintf("\n DevMode: %t\n Debug: %t\n ListenAddr: %s\n Database: %s\n Storage: %s\n JWTSecret: %s\n",
		c.DevMode,
		c.Debug,
		c.ListenAddr,
		c.Database,
		c.Storage,
		strings.Repeat("*", len(c.JWTSecret)),
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
	Path string `cfg:"path"`

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
		str += fmt.Sprintf("Path: %s", c.Path)
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