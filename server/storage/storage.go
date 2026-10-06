package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/topi314/godrive/server/config"
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
	Mkdir(ctx context.Context, path string) error
	DeleteObject(ctx context.Context, path string) error
	CopyObject(ctx context.Context, from, to string) error
	RenameObject(ctx context.Context, from, to string) error
	Stat(ctx context.Context, path string) (ObjectInfo, error)
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)
	Watch(ctx context.Context, onChange func(event Event)) error
	Close() error

	// Resumable upload staging (temp keys are opaque session ids, not user paths).
	CreateUpload(ctx context.Context, tempKey string, size int64) (uploadID string, err error)
	WriteUpload(ctx context.Context, tempKey string, offset int64, r io.Reader, n int64, uploadID string, partNumber int32) (etag string, err error)
	CommitUpload(ctx context.Context, tempKey, destPath, contentType, uploadID string, parts []CompletedPart) error
	AbortUpload(ctx context.Context, tempKey, uploadID string) error
}

// CompletedPart is one finished S3 multipart part (unused for local storage).
type CompletedPart struct {
	PartNumber int32
	ETag       string
}

const ContentTypeDirectory = "inode/directory"

func IsDirectory(contentType string) bool {
	return contentType == ContentTypeDirectory || contentType == "application/x-directory"
}

type EventType string

const (
	EventUpsert EventType = "upsert"
	EventDelete EventType = "delete"
)

type Event struct {
	Type EventType
	Path string
	Info ObjectInfo
}

func New(ctx context.Context, cfg config.StorageConfig) (Storage, error) {
	switch cfg.Type {
	case config.StorageTypeLocal:
		return newLocalStorage(cfg)
	case config.StorageTypeS3:
		return newS3Storage(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", cfg.Type)
	}
}
