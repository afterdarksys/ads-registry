package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/ryan/ads-registry/internal/storage"
)

// Store implements storage.Provider for S3-compatible object storage
type Store struct {
	client *s3.Client
	bucket string
}

// New creates a new S3-compatible storage provider
// Works with AWS S3, MinIO, and other S3-compatible services
func New(endpoint, region, bucket, accessKey, secretKey string, usePathStyle bool) (*Store, error) {
	var opts []func(*config.LoadOptions) error

	// Set region
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}

	// Set credentials
	if accessKey != "" && secretKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		))
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client with custom endpoint if provided (for MinIO/OCI)
	clientOpts := []func(*s3.Options){}
	if endpoint != "" {
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = usePathStyle // MinIO typically uses path-style
		})
	}

	client := s3.NewFromConfig(cfg, clientOpts...)

	return &Store{
		client: client,
		bucket: bucket,
	}, nil
}

// Writer returns a WriteCloser for uploading a blob
func (s *Store) Writer(ctx context.Context, path string) (io.WriteCloser, error) {
	return &s3Writer{
		store:  s,
		path:   path,
		buffer: &bytes.Buffer{},
		ctx:    ctx,
	}, nil
}

// Appender returns a WriteCloser for appending to a blob
// Note: S3 doesn't support true appending, so we read existing data and append
func (s *Store) Appender(ctx context.Context, path string) (io.WriteCloser, error) {
	// Read existing data
	var existingData bytes.Buffer
	reader, err := s.Reader(ctx, path, 0)
	if err != nil && err != storage.ErrNotFound {
		return nil, err
	}
	if err == nil {
		defer reader.Close()
		if _, err := io.Copy(&existingData, reader); err != nil {
			return nil, err
		}
	}

	return &s3Writer{
		store:  s,
		path:   path,
		buffer: &existingData,
		ctx:    ctx,
	}, nil
}

// Reader returns a ReadCloser for downloading a blob
func (s *Store) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	}

	if offset > 0 {
		input.Range = aws.String(fmt.Sprintf("bytes=%d-", offset))
	}

	result, err := s.client.GetObject(ctx, input)
	if err != nil {
		return nil, storage.ErrNotFound
	}

	return result.Body, nil
}

// Delete removes a blob
func (s *Store) Delete(ctx context.Context, path string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})
	return err
}

// Move renames a blob (copy + delete)
func (s *Store) Move(ctx context.Context, source string, target string) error {
	// Copy object
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		CopySource: aws.String(fmt.Sprintf("%s/%s", s.bucket, source)),
		Key:        aws.String(target),
	})
	if err != nil {
		return err
	}

	// Delete source
	return s.Delete(ctx, source)
}

// Stat returns the size of a blob
func (s *Store) Stat(ctx context.Context, path string) (int64, error) {
	result, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return 0, storage.ErrNotFound
	}

	return aws.ToInt64(result.ContentLength), nil
}

// s3Writer implements io.WriteCloser for S3 uploads
type s3Writer struct {
	store  *Store
	path   string
	buffer *bytes.Buffer
	ctx    context.Context
}

func (w *s3Writer) Write(p []byte) (n int, err error) {
	return w.buffer.Write(p)
}

func (w *s3Writer) Close() error {
	// Upload the buffered data
	_, err := w.store.client.PutObject(w.ctx, &s3.PutObjectInput{
		Bucket: aws.String(w.store.bucket),
		Key:    aws.String(w.path),
		Body:   bytes.NewReader(w.buffer.Bytes()),
	})
	return err
}
