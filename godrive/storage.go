package godrive

import (
	"context"
	"fmt"
	"io"
	"time"
)

type ObjectInfo struct {
	Path         string
	Size         int64
	ContentType  string
	LastModified time.Time
}

type Storage interface {
	GetObject(ctx context.Context, path string, start, end int64) (io.ReadCloser, ObjectInfo, error)
	PutObject(ctx context.Context, path string, size int64, r io.Reader, contentType string) error
	DeleteObject(ctx context.Context, path string) error
	CopyObject(ctx context.Context, from, to string) error
	Stat(ctx context.Context, path string) (ObjectInfo, error)
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)
	Watch(ctx context.Context, onChange func(event StorageEvent)) error
	Close() error
}

type StorageEventType string

const (
	StorageEventUpsert StorageEventType = "upsert"
	StorageEventDelete StorageEventType = "delete"
)

type StorageEvent struct {
	Type StorageEventType
	Path string
	Info ObjectInfo
}

func NewStorage(ctx context.Context, cfg StorageConfig) (Storage, error) {
	switch cfg.Type {
	case StorageTypeLocal:
		return newLocalStorage(cfg)
	case StorageTypeS3:
		return newS3Storage(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", cfg.Type)
	}
}
