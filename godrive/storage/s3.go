package storage

import (
	"context"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/topi314/godrive/internal/http_range"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func newS3Storage(ctx context.Context, config Config, tracer trace.Tracer) (Storage, error) {
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(config.AccessKeyID, config.SecretAccessKey, ""),
		Secure:    config.Secure,
		Transport: otelhttp.NewTransport(nil),
		Region:    config.Region,
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
		tracer: tracer,
	}, nil
}

type s3Storage struct {
	client *minio.Client
	bucket string
	tracer trace.Tracer
}

func (s *s3Storage) GetObject(ctx context.Context, filePath string, ra *http_range.Range) (io.ReadCloser, error) {
	attrs := []attribute.KeyValue{
		attribute.String("file_path", filePath),
	}
	if ra != nil {
		attrs = append(attrs,
			attribute.Int64("start", ra.Start),
			attribute.Int64("end", ra.End),
		)
	}
	ctx, span := s.tracer.Start(ctx, "s3Storage.GetObject", trace.WithAttributes(attrs...))
	defer span.End()
	opts := minio.GetObjectOptions{}
	if ra != nil {
		if err := opts.SetRange(ra.Start, ra.End); err != nil {
			span.SetStatus(codes.Error, "failed to set range")
			span.RecordError(err)
			return nil, err
		}

	}
	r, err := s.client.GetObject(ctx, s.bucket, filePath, opts)
	if err != nil {
		span.SetStatus(codes.Error, "failed to get object")
		span.RecordError(err)
		return nil, err
	}

	return r, nil
}

func (s *s3Storage) MoveObject(ctx context.Context, from string, to string) error {
	ctx, span := s.tracer.Start(ctx, "s3Storage.MoveObject", trace.WithAttributes(
		attribute.String("from", from),
		attribute.String("to", to),
	))
	defer span.End()
	_, err := s.client.CopyObject(ctx, minio.CopyDestOptions{
		Bucket: s.bucket,
		Object: to,
	}, minio.CopySrcOptions{
		Bucket: s.bucket,
		Object: from,
	})
	if err != nil {
		span.SetStatus(codes.Error, "failed to copy object")
		span.RecordError(err)
		return err
	}
	err = s.client.RemoveObject(ctx, s.bucket, from, minio.RemoveObjectOptions{})
	if err != nil {
		span.SetStatus(codes.Error, "failed to remove object")
		span.RecordError(err)
	}
	return err
}

func (s *s3Storage) PutObject(ctx context.Context, filePath string, size int64, reader io.Reader, contentType string) error {
	ctx, span := s.tracer.Start(ctx, "s3Storage.PutObject", trace.WithAttributes(
		attribute.String("file_path", filePath),
		attribute.Int64("size", size),
		attribute.String("content_type", contentType),
	))
	defer span.End()
	_, err := s.client.PutObject(ctx, s.bucket, filePath, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		span.SetStatus(codes.Error, "failed to put object")
		span.RecordError(err)
	}
	return err
}

func (s *s3Storage) DeleteObject(ctx context.Context, filePath string) error {
	ctx, span := s.tracer.Start(ctx, "s3Storage.DeleteObject", trace.WithAttributes(
		attribute.String("file_path", filePath),
	))
	defer span.End()
	err := s.client.RemoveObject(ctx, s.bucket, filePath, minio.RemoveObjectOptions{})
	if err != nil {
		span.SetStatus(codes.Error, "failed to remove object")
		span.RecordError(err)
	}
	return err
}
