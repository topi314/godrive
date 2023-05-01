package godrive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func NewStorage(ctx context.Context, config StorageConfig) (Storage, error) {
	switch config.Type {
	case StorageTypeLocal:
		return newLocalStorage(config)
	case StorageTypeS3:
		return newS3Storage(ctx, config)
	}
	return nil, errors.New("unknown storage type")
}

type Storage interface {
	GetObject(ctx context.Context, filePath string, start *int64, end *int64) (io.ReadCloser, error)
	MoveObject(ctx context.Context, from string, to string) error
	PutObject(ctx context.Context, filePath string, size uint64, reader io.Reader, contentType string) error
	DeleteObject(ctx context.Context, filePath string) error
}

func newLocalStorage(config StorageConfig) (Storage, error) {
	if err := os.MkdirAll(config.Path, 0777); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}
	return &localStorage{
		path: config.Path,
	}, nil
}

type localStorage struct {
	path string
}

func (l *localStorage) GetObject(_ context.Context, filePath string, start *int64, end *int64) (io.ReadCloser, error) {
	file, err := os.Open(l.path + "/" + filePath)
	if err != nil {
		return nil, err
	}

	if start != nil && end != nil {
		if _, err = file.Seek(*start, io.SeekStart); err != nil {
			return nil, err
		}

		return &limitedReader{
			Reader: io.LimitReader(file, *end-*start),
			closeFunc: func() error {
				return file.Close()
			},
		}, nil
	}

	return file, nil
}

type limitedReader struct {
	io.Reader
	closeFunc func() error
}

func (l *limitedReader) Close() error {
	if l.closeFunc != nil {
		return l.closeFunc()
	}
	return nil
}

func (l *localStorage) PutObject(_ context.Context, filePath string, _ uint64, reader io.Reader, _ string) error {
	file, err := os.Create(l.path + "/" + filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, reader)
	return err
}

func (l *localStorage) MoveObject(_ context.Context, from string, to string) error {
	return os.Rename(l.path+"/"+from, l.path+"/"+to)
}

func (l *localStorage) DeleteObject(_ context.Context, filePath string) error {
	return os.Remove(l.path + "/" + filePath)
}

func newS3Storage(ctx context.Context, config StorageConfig) (Storage, error) {
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKeyID, config.SecretAccessKey, ""),
		Secure: config.Secure,
		Region: config.Region,
	})
	if err != nil {
		return nil, err
	}

	if config.Debug {
		client.TraceOn(nil)
	}
	client.SetAppInfo("godrive", "0.0.1")

	if err = client.MakeBucket(ctx, config.Bucket, minio.MakeBucketOptions{}); err != nil {
		exists, errBucketExists := client.BucketExists(ctx, config.Bucket)
		if errBucketExists != nil || !exists {
			return nil, err
		}
	}

	return &s3Storage{
		client: client,
		bucket: config.Bucket,
	}, nil
}

type s3Storage struct {
	client *minio.Client
	bucket string
}

func (s *s3Storage) GetObject(ctx context.Context, filePath string, start *int64, end *int64) (io.ReadCloser, error) {
	opts := minio.GetObjectOptions{}
	if start != nil && end != nil {
		if err := opts.SetRange(*start, *end); err != nil {
			return nil, fmt.Errorf("failed to set range: %w", err)
		}
	}
	return s.client.GetObject(ctx, s.bucket, filePath, opts)
}

func (s *s3Storage) MoveObject(ctx context.Context, from string, to string) error {
	_, err := s.client.CopyObject(ctx, minio.CopyDestOptions{
		Bucket: s.bucket,
		Object: to,
	}, minio.CopySrcOptions{
		Bucket: s.bucket,
		Object: from,
	})
	if err != nil {
		return err
	}
	return s.client.RemoveObject(ctx, s.bucket, from, minio.RemoveObjectOptions{})
}

func (s *s3Storage) PutObject(ctx context.Context, filePath string, size uint64, reader io.Reader, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, filePath, reader, int64(size), minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

func (s *s3Storage) DeleteObject(ctx context.Context, filePath string) error {
	return s.client.RemoveObject(ctx, s.bucket, filePath, minio.RemoveObjectOptions{})
}
