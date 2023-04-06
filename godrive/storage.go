package godrive

import (
	"context"
	"errors"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io"
	"os"
)

func NewStorage(ctx context.Context, config StorageConfig) (Storage, error) {
	switch config.Type {
	case StorageTypeLocal:
		return newLocalStorage(config), nil
	case StorageTypeS3:
		return newS3Storage(ctx, config)
	}
	return nil, errors.New("unknown storage type")
}

type Storage interface {
	GetObject(ctx context.Context, name string) (io.ReadCloser, error)
	PutObject(ctx context.Context, name string, size uint64, reader io.Reader, contentType string) error
	DeleteObject(ctx context.Context, name string) error
}

func newLocalStorage(config StorageConfig) Storage {
	return &localStorage{
		path: config.Path,
	}
}

type localStorage struct {
	path string
}

func (l *localStorage) GetObject(ctx context.Context, name string) (io.ReadCloser, error) {
	return os.Open(l.path + "/" + name)
}

func (l *localStorage) PutObject(ctx context.Context, name string, _ uint64, reader io.Reader, _ string) error {
	file, err := os.Create(l.path + "/" + name)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, reader)
	return err
}

func (l *localStorage) DeleteObject(ctx context.Context, name string) error {
	return os.Remove(l.path + "/" + name)
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

func (s *s3Storage) GetObject(ctx context.Context, name string) (io.ReadCloser, error) {
	return s.client.GetObject(ctx, s.bucket, name, minio.GetObjectOptions{})
}

func (s *s3Storage) PutObject(ctx context.Context, name string, size uint64, reader io.Reader, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, name, reader, int64(size), minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

func (s *s3Storage) DeleteObject(ctx context.Context, name string) error {
	return s.client.RemoveObject(ctx, s.bucket, name, minio.RemoveObjectOptions{})
}