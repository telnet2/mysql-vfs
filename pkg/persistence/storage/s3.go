package storage

import (
	"context"
	"fmt"
	"io"
	"os"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/s3blob" // Register S3 driver
)

// Storage provides an abstraction for file storage operations
type Storage interface {
	// Put stores content with the given key
	Put(ctx context.Context, key string, content io.Reader) error

	// Get retrieves content by key
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes content by key
	Delete(ctx context.Context, key string) error

	// Exists checks if content exists
	Exists(ctx context.Context, key string) (bool, error)

	// Close closes the storage bucket
	Close() error
}

// S3Storage implements Storage using gocloud.dev blob abstraction
type S3Storage struct {
	bucket *blob.Bucket
}

// Config holds S3 configuration
type Config struct {
	BucketURL string // e.g., "s3://bucket-name?region=us-east-1" or "mem://" for testing
}

// NewStorage creates a new storage instance
func NewStorage(ctx context.Context, cfg Config) (Storage, error) {
	bucket, err := blob.OpenBucket(ctx, cfg.BucketURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open bucket: %w", err)
	}

	return &S3Storage{
		bucket: bucket,
	}, nil
}

// NewStorageFromEnv creates storage from environment variables
func NewStorageFromEnv(ctx context.Context) (Storage, error) {
	endpoint := os.Getenv("S3_ENDPOINT")
	bucket := os.Getenv("S3_BUCKET")
	region := os.Getenv("AWS_REGION")

	if endpoint == "" || bucket == "" {
		return nil, fmt.Errorf("S3_ENDPOINT and S3_BUCKET must be set")
	}

	return NewStorageWithParams(ctx, endpoint, bucket, region)
}

// NewStorageWithParams creates storage from explicit parameters
func NewStorageWithParams(ctx context.Context, endpoint, bucket, region string) (Storage, error) {
	if endpoint == "" || bucket == "" {
		return nil, fmt.Errorf("endpoint and bucket must be set")
	}

	if region == "" {
		region = "us-east-1"
	}

	// For LocalStack or custom endpoints
	var bucketURL string
	if endpoint == "http://localstack:4566" || endpoint == "http://localhost:4566" {
		// Use awslocal-style URL for LocalStack
		bucketURL = fmt.Sprintf("s3://%s?region=%s&endpoint=%s",
			bucket, region, endpoint)
	} else {
		bucketURL = fmt.Sprintf("s3://%s?region=%s", bucket, region)
	}

	return NewStorage(ctx, Config{BucketURL: bucketURL})
}

// Put stores content with the given key
func (s *S3Storage) Put(ctx context.Context, key string, content io.Reader) error {
	writer, err := s.bucket.NewWriter(ctx, key, nil)
	if err != nil {
		return fmt.Errorf("failed to create writer: %w", err)
	}

	if _, err := io.Copy(writer, content); err != nil {
		writer.Close()
		return fmt.Errorf("failed to write content: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close writer: %w", err)
	}

	return nil
}

// Get retrieves content by key
func (s *S3Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	reader, err := s.bucket.NewReader(ctx, key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create reader: %w", err)
	}

	return reader, nil
}

// Delete removes content by key
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	if err := s.bucket.Delete(ctx, key); err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	return nil
}

// Exists checks if content exists
func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	return s.bucket.Exists(ctx, key)
}

// Close closes the storage bucket
func (s *S3Storage) Close() error {
	return s.bucket.Close()
}
