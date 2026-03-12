package oci

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
	"github.com/ryan/ads-registry/internal/storage"
)

// Store implements storage.Provider for Oracle Cloud Infrastructure Object Storage
type Store struct {
	client    *objectstorage.ObjectStorageClient
	namespace string
	bucket    string
}

// Config holds OCI Object Storage configuration
type Config struct {
	Namespace      string
	Bucket         string
	Region         string
	TenancyID      string
	UserID         string
	Fingerprint    string
	PrivateKeyPath string
	PrivateKey     string // Inline private key as alternative to file path
}

// New creates a new OCI Object Storage provider
func New(cfg Config) (*Store, error) {
	var privateKey *rsa.PrivateKey
	var err error

	// Load private key from file or inline
	if cfg.PrivateKeyPath != "" {
		keyData, err := os.ReadFile(cfg.PrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key file: %w", err)
		}
		privateKey, err = parsePrivateKey(keyData)
		if err != nil {
			return nil, err
		}
	} else if cfg.PrivateKey != "" {
		privateKey, err = parsePrivateKey([]byte(cfg.PrivateKey))
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("either private_key_path or private_key must be provided")
	}

	// Create OCI configuration provider
	configProvider := common.NewRawConfigurationProvider(
		cfg.TenancyID,
		cfg.UserID,
		cfg.Region,
		cfg.Fingerprint,
		string(pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
		})),
		nil,
	)

	// Create Object Storage client
	client, err := objectstorage.NewObjectStorageClientWithConfigurationProvider(configProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create OCI client: %w", err)
	}

	return &Store{
		client:    &client,
		namespace: cfg.Namespace,
		bucket:    cfg.Bucket,
	}, nil
}

// parsePrivateKey parses PEM-encoded private key
func parsePrivateKey(keyData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Try PKCS1
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	// Try PKCS8
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, fmt.Errorf("key is not RSA private key")
	}

	return nil, fmt.Errorf("failed to parse private key")
}

// Writer returns a WriteCloser for uploading a blob
func (s *Store) Writer(ctx context.Context, path string) (io.WriteCloser, error) {
	return &ociWriter{
		store:  s,
		path:   path,
		buffer: &bytes.Buffer{},
		ctx:    ctx,
	}, nil
}

// Appender returns a WriteCloser for appending to a blob
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

	return &ociWriter{
		store:  s,
		path:   path,
		buffer: &existingData,
		ctx:    ctx,
	}, nil
}

// Reader returns a ReadCloser for downloading a blob
func (s *Store) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	req := objectstorage.GetObjectRequest{
		NamespaceName: common.String(s.namespace),
		BucketName:    common.String(s.bucket),
		ObjectName:    common.String(path),
	}

	if offset > 0 {
		req.Range = common.String(fmt.Sprintf("bytes=%d-", offset))
	}

	resp, err := s.client.GetObject(ctx, req)
	if err != nil {
		return nil, storage.ErrNotFound
	}

	return resp.Content, nil
}

// Delete removes a blob
func (s *Store) Delete(ctx context.Context, path string) error {
	_, err := s.client.DeleteObject(ctx, objectstorage.DeleteObjectRequest{
		NamespaceName: common.String(s.namespace),
		BucketName:    common.String(s.bucket),
		ObjectName:    common.String(path),
	})
	return err
}

// Move renames a blob (rename object in OCI)
func (s *Store) Move(ctx context.Context, source string, target string) error {
	// OCI supports renaming directly
	_, err := s.client.RenameObject(ctx, objectstorage.RenameObjectRequest{
		NamespaceName: common.String(s.namespace),
		BucketName:    common.String(s.bucket),
		RenameObjectDetails: objectstorage.RenameObjectDetails{
			SourceName: common.String(source),
			NewName:    common.String(target),
		},
	})
	return err
}

// Stat returns the size of a blob
func (s *Store) Stat(ctx context.Context, path string) (int64, error) {
	resp, err := s.client.HeadObject(ctx, objectstorage.HeadObjectRequest{
		NamespaceName: common.String(s.namespace),
		BucketName:    common.String(s.bucket),
		ObjectName:    common.String(path),
	})
	if err != nil {
		return 0, storage.ErrNotFound
	}

	return *resp.ContentLength, nil
}

// ociWriter implements io.WriteCloser for OCI uploads
type ociWriter struct {
	store  *Store
	path   string
	buffer *bytes.Buffer
	ctx    context.Context
}

func (w *ociWriter) Write(p []byte) (n int, err error) {
	return w.buffer.Write(p)
}

func (w *ociWriter) Close() error {
	// Upload the buffered data
	contentLength := int64(w.buffer.Len())
	_, err := w.store.client.PutObject(w.ctx, objectstorage.PutObjectRequest{
		NamespaceName: common.String(w.store.namespace),
		BucketName:    common.String(w.store.bucket),
		ObjectName:    common.String(w.path),
		ContentLength: &contentLength,
		PutObjectBody: io.NopCloser(bytes.NewReader(w.buffer.Bytes())),
	})
	return err
}
