package godrive

import (
	"fmt"
	"log/slog"

	"github.com/topi314/godrive/godrive/auth"
	"github.com/topi314/godrive/godrive/database"
	"github.com/topi314/godrive/godrive/storage"
)

type Config struct {
	Log        LogConfig       `cfg:"log"`
	DevMode    bool            `cfg:"dev_mode"`
	Debug      bool            `cfg:"debug"`
	ListenAddr string          `cfg:"listen_addr"`
	PublicURL  string          `cfg:"public_url"`
	Database   database.Config `cfg:"database"`
	Storage    storage.Config  `cfg:"storage"`
	Auth       *auth.Config    `cfg:"auth"`
	Otel       *OtelConfig     `cfg:"otel"`
}

func (c Config) String() string {
	return fmt.Sprintf("\n Log: %s\n DevMode: %t\n Debug: %t\n ListenAddr: %s\n PublicURL: %s\n Database: %s\n Storage: %s\n Auth: %s\n Otel: %s\n",
		c.Log,
		c.DevMode,
		c.Debug,
		c.ListenAddr,
		c.PublicURL,
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
