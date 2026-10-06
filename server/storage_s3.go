package server

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type s3Storage struct {
	client *s3.Client
	bucket string
}

func newS3Storage(ctx context.Context, cfg StorageConfig) (*s3Storage, error) {
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
	}
	if cfg.AccessKeyID != "" {
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.ForcePathStyle || cfg.Endpoint != ""
	})

	return &s3Storage{client: client, bucket: cfg.Bucket}, nil
}

func s3Key(path string) string {
	return strings.TrimPrefix(NormalizePath(path), "/")
}

func (s *s3Storage) GetObject(ctx context.Context, path string, start, end int64) (io.ReadCloser, ObjectInfo, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key(path)),
	}
	if start > 0 || end > 0 {
		if end > 0 {
			input.Range = aws.String(fmt.Sprintf("bytes=%d-%d", start, end))
		} else {
			input.Range = aws.String(fmt.Sprintf("bytes=%d-", start))
		}
	}
	out, err := s.client.GetObject(ctx, input)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	info := ObjectInfo{Path: NormalizePath(path)}
	if out.ContentLength != nil {
		info.Size = *out.ContentLength
	}
	if out.ContentType != nil {
		info.ContentType = *out.ContentType
	}
	if out.LastModified != nil {
		info.LastModified = *out.LastModified
	}
	return out.Body, info, nil
}

func (s *s3Storage) PutObject(ctx context.Context, path string, size int64, r io.Reader, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(s3Key(path)),
		Body:          r,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	return err
}

func (s *s3Storage) DeleteObject(ctx context.Context, path string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key(path)),
	})
	return err
}

func (s *s3Storage) CopyObject(ctx context.Context, from, to string) error {
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		Key:        aws.String(s3Key(to)),
		CopySource: aws.String(s.bucket + "/" + s3Key(from)),
	})
	return err
}

func (s *s3Storage) Stat(ctx context.Context, path string) (ObjectInfo, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key(path)),
	})
	if err != nil {
		return ObjectInfo{}, err
	}
	info := ObjectInfo{Path: NormalizePath(path), ContentType: "application/octet-stream"}
	if out.ContentLength != nil {
		info.Size = *out.ContentLength
	}
	if out.ContentType != nil {
		info.ContentType = *out.ContentType
	}
	if out.LastModified != nil {
		info.LastModified = *out.LastModified
	}
	return info, nil
}

func (s *s3Storage) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	prefix = strings.TrimPrefix(NormalizePath(prefix), "/")
	if prefix != "" {
		prefix += "/"
	}
	var out []ObjectInfo
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}
			info := ObjectInfo{Path: NormalizePath("/" + *obj.Key)}
			if obj.Size != nil {
				info.Size = *obj.Size
			}
			if obj.LastModified != nil {
				info.LastModified = *obj.LastModified
			}
			info.ContentType = "application/octet-stream"
			out = append(out, info)
		}
	}
	return out, nil
}

func (s *s3Storage) Watch(ctx context.Context, onChange func(StorageEvent)) error {
	// Instant pickup for S3 is via webhook/SQS handlers on the server, not a local watcher.
	return nil
}

func (s *s3Storage) Close() error { return nil }
