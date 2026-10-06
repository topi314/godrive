package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/topi314/godrive/server/acl"
	"github.com/topi314/godrive/server/config"
)

type s3Storage struct {
	client *s3.Client
	bucket string
}

func newS3Storage(ctx context.Context, cfg config.StorageConfig) (*s3Storage, error) {
	s3cfg := cfg.S3
	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(s3cfg.Region),
	}
	if s3cfg.AccessKeyID != "" {
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(s3cfg.AccessKeyID, s3cfg.SecretAccessKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if s3cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(s3Endpoint(s3cfg.Endpoint, s3cfg.Secure))
		}
		o.EndpointOptions.DisableHTTPS = !s3cfg.Secure
		o.UsePathStyle = s3cfg.ForcePathStyle || s3cfg.Endpoint != ""
	})

	return &s3Storage{client: client, bucket: s3cfg.Bucket}, nil
}

// s3Endpoint normalizes a custom endpoint to http(s) based on [storage.s3].secure.
func s3Endpoint(endpoint string, secure bool) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	scheme := "http"
	if secure {
		scheme = "https"
	}
	lower := strings.ToLower(endpoint)
	switch {
	case strings.HasPrefix(lower, "https://"):
		return scheme + "://" + endpoint[len("https://"):]
	case strings.HasPrefix(lower, "http://"):
		return scheme + "://" + endpoint[len("http://"):]
	default:
		return scheme + "://" + endpoint
	}
}

func s3Key(path string) string {
	return strings.TrimPrefix(acl.NormalizePath(path), "/")
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
	info := ObjectInfo{Path: acl.NormalizePath(path)}
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

func (s *s3Storage) Mkdir(ctx context.Context, path string) error {
	key := s3Key(path)
	if key != "" && !strings.HasSuffix(key, "/") {
		key += "/"
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        strings.NewReader(""),
		ContentType: aws.String(ContentTypeDirectory),
	})
	return err
}

func (s *s3Storage) DeleteObject(ctx context.Context, path string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key(path)),
	})
	if err != nil {
		return err
	}
	_, _ = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key(path) + "/"),
	})
	return nil
}

func (s *s3Storage) CopyObject(ctx context.Context, from, to string) error {
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		Key:        aws.String(s3Key(to)),
		CopySource: aws.String(s.bucket + "/" + s3Key(from)),
	})
	return err
}

func (s *s3Storage) RenameObject(ctx context.Context, from, to string) error {
	from, to = acl.NormalizePath(from), acl.NormalizePath(to)
	info, err := s.Stat(ctx, from)
	kids, listErr := s.List(ctx, from)
	if listErr != nil {
		kids = nil
	}
	isDir := err == nil && IsDirectory(info.ContentType) || len(kids) > 0
	if !isDir {
		if err := s.CopyObject(ctx, from, to); err != nil {
			return err
		}
		return s.DeleteObject(ctx, from)
	}
	if err := s.Mkdir(ctx, to); err != nil {
		return err
	}
	for _, obj := range kids {
		dest := acl.RemapPrefix(obj.Path, from, to)
		if IsDirectory(obj.ContentType) {
			if err := s.Mkdir(ctx, dest); err != nil {
				return err
			}
			continue
		}
		if err := s.CopyObject(ctx, obj.Path, dest); err != nil {
			return err
		}
	}
	for i := len(kids) - 1; i >= 0; i-- {
		_ = s.DeleteObject(ctx, kids[i].Path)
	}
	return s.DeleteObject(ctx, from)
}

func (s *s3Storage) Stat(ctx context.Context, path string) (ObjectInfo, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key(path)),
	})
	if err != nil {
		return ObjectInfo{}, err
	}
	info := ObjectInfo{Path: acl.NormalizePath(path), ContentType: "application/octet-stream"}
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
	prefix = strings.TrimPrefix(acl.NormalizePath(prefix), "/")
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
			key := *obj.Key
			logical := acl.NormalizePath("/" + strings.TrimSuffix(key, "/"))
			info := ObjectInfo{Path: logical}
			if strings.HasSuffix(key, "/") {
				info.ContentType = ContentTypeDirectory
			} else {
				info.ContentType = "application/octet-stream"
			}
			if obj.Size != nil {
				info.Size = *obj.Size
			}
			if obj.LastModified != nil {
				info.LastModified = *obj.LastModified
			}
			out = append(out, info)
		}
	}
	return out, nil
}

func (s *s3Storage) Watch(ctx context.Context, onChange func(Event)) error {
	// Instant pickup for S3 is via the storage-events webhook (S3 event JSON), not a local watcher.
	return nil
}

func (s *s3Storage) Close() error { return nil }

func (s *s3Storage) uploadKey(tempKey string) string {
	return ".uploads/" + tempKey
}

func (s *s3Storage) CreateUpload(ctx context.Context, tempKey string, size int64) (string, error) {
	out, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.uploadKey(tempKey)),
	})
	if err != nil {
		return "", err
	}
	if out.UploadId == nil {
		return "", fmt.Errorf("missing upload id")
	}
	return *out.UploadId, nil
}

func (s *s3Storage) WriteUpload(ctx context.Context, tempKey string, offset int64, r io.Reader, n int64, uploadID string, partNumber int32) (string, error) {
	out, err := s.client.UploadPart(ctx, &s3.UploadPartInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(s.uploadKey(tempKey)),
		UploadId:      aws.String(uploadID),
		PartNumber:    aws.Int32(partNumber),
		Body:          io.LimitReader(r, n),
		ContentLength: aws.Int64(n),
	})
	if err != nil {
		return "", err
	}
	if out.ETag == nil {
		return "", fmt.Errorf("missing etag")
	}
	return *out.ETag, nil
}

func (s *s3Storage) CommitUpload(ctx context.Context, tempKey, destPath, contentType, uploadID string, parts []CompletedPart) error {
	completed := make([]types.CompletedPart, 0, len(parts))
	for _, p := range parts {
		pn, et := p.PartNumber, p.ETag
		completed = append(completed, types.CompletedPart{PartNumber: &pn, ETag: &et})
	}
	_, err := s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(s.uploadKey(tempKey)),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completed,
		},
	})
	if err != nil {
		return err
	}
	// Copy temp multipart object to final key, then delete temp.
	src := fmt.Sprintf("%s/%s", s.bucket, s.uploadKey(tempKey))
	_, err = s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s3Key(destPath)),
		CopySource:  aws.String(src),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return err
	}
	_, _ = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.uploadKey(tempKey)),
	})
	return nil
}

func (s *s3Storage) AbortUpload(ctx context.Context, tempKey, uploadID string) error {
	if uploadID != "" {
		_, _ = s.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
			Bucket:   aws.String(s.bucket),
			Key:      aws.String(s.uploadKey(tempKey)),
			UploadId: aws.String(uploadID),
		})
	}
	_, _ = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.uploadKey(tempKey)),
	})
	return nil
}

