package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/topi314/godrive/internal/http_range"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func newLocalStorage(config Config, tracer trace.Tracer) (Storage, error) {
	if err := os.MkdirAll(config.Path, 0777); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	l := &localStorage{
		path:   config.Path,
		tracer: tracer,
	}
	if err := l.cleanup(); err != nil {
		return nil, fmt.Errorf("failed to cleanup storage directory: %w", err)
	}

	return l, nil
}

type localStorage struct {
	path   string
	tracer trace.Tracer
}

func (l *localStorage) GetObject(ctx context.Context, filePath string, ra *http_range.Range) (io.ReadCloser, error) {
	attrs := []attribute.KeyValue{
		attribute.String("file_path", filePath),
	}
	if ra != nil {
		attrs = append(attrs,
			attribute.Int64("start", ra.Start),
			attribute.Int64("end", ra.End),
		)
	}
	ctx, span := l.tracer.Start(ctx, "localStorage.GetObject", trace.WithAttributes(attrs...))
	defer span.End()
	file, err := os.Open(l.path + filePath)
	if err != nil {
		span.SetStatus(codes.Error, "failed to open file")
		span.RecordError(err)
		return nil, err
	}

	if ra == nil {
		return file, nil
	}

	if ra.Start > 0 {
		if _, err = file.Seek(ra.Start, io.SeekStart); err != nil {
			return nil, err
		}
	}

	limit := ra.Limit()
	if limit == 0 {
		return file, nil
	}

	return &limitedReader{
		Reader:    io.LimitReader(file, limit),
		closeFunc: file.Close,
	}, nil
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

func (l *localStorage) PutObject(ctx context.Context, filePath string, size int64, reader io.Reader, contentType string) error {
	ctx, span := l.tracer.Start(ctx, "localStorage.PutObject", trace.WithAttributes(
		attribute.String("file_path", filePath),
		attribute.Int64("size", size),
		attribute.String("content_type", contentType),
	))
	defer span.End()

	if err := os.MkdirAll(path.Dir(l.path+filePath), 0777); err != nil {
		span.SetStatus(codes.Error, "failed to create directory")
		span.RecordError(err)
		return err
	}
	file, err := os.Create(l.path + filePath)
	if err != nil {
		span.SetStatus(codes.Error, "failed to create file")
		span.RecordError(err)
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, reader)
	if err != nil {
		span.SetStatus(codes.Error, "failed to copy file")
		span.RecordError(err)
	}
	return err
}

func (l *localStorage) MoveObject(ctx context.Context, from string, to string) error {
	ctx, span := l.tracer.Start(ctx, "localStorage.MoveObject", trace.WithAttributes(
		attribute.String("from", from),
		attribute.String("to", to),
	))
	defer span.End()
	if err := os.MkdirAll(path.Dir(l.path+to), 0777); err != nil {
		span.SetStatus(codes.Error, "failed to create directory")
		span.RecordError(err)
		return err
	}
	if err := os.Rename(l.path+from, l.path+to); err != nil {
		span.SetStatus(codes.Error, "failed to rename file")
		span.RecordError(err)
		return err
	}
	return l.cleanup()
}

func (l *localStorage) DeleteObject(ctx context.Context, filePath string) error {
	ctx, span := l.tracer.Start(ctx, "localStorage.DeleteObject", trace.WithAttributes(
		attribute.String("file_path", filePath),
	))
	defer span.End()
	if err := os.Remove(l.path + filePath); err != nil {
		span.SetStatus(codes.Error, "failed to delete file")
		span.RecordError(err)
		return err
	}
	return l.cleanup()
}

func (l *localStorage) cleanup() error {
	return nil
}
