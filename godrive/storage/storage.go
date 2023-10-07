package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/topi314/godrive/internal/http_range"
	"go.opentelemetry.io/otel/trace"
)

type Type string

const (
	TypeLocal Type = "local"
	TypeS3    Type = "s3"
)

type Config struct {
	Type  Type `cfg:"type"`
	Debug bool `cfg:"debug"`

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

func (c Config) String() string {
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

func New(ctx context.Context, config Config, tracer trace.Tracer) (Storage, error) {
	switch config.Type {
	case TypeLocal:
		return newLocalStorage(config, tracer)
	case TypeS3:
		return newS3Storage(ctx, config, tracer)
	}
	return nil, errors.New("unknown storage type")
}

type Storage interface {
	GetObject(ctx context.Context, filePath string, ra *http_range.Range) (io.ReadCloser, error)
	MoveObject(ctx context.Context, from string, to string) error
	PutObject(ctx context.Context, filePath string, size int64, reader io.Reader, contentType string) error
	DeleteObject(ctx context.Context, filePath string) error
}
