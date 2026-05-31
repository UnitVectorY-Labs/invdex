package storage

import (
	"context"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/UnitVectorY-Labs/invdex/internal/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

// Storage provides an interface for file storage operations.
type Storage interface {
	Upload(ctx context.Context, filename string, contentType string, reader io.Reader) (string, error)
	Delete(ctx context.Context, key string) error
	GetURL(ctx context.Context, key string) (string, error)
}

// S3Storage implements Storage using S3-compatible backends.
type S3Storage struct {
	client *s3.Client
	bucket string
}

// NewS3Storage creates a new S3-compatible storage client.
func NewS3Storage(ctx context.Context, cfg *config.Config) (*S3Storage, error) {
	var opts []func(*awsconfig.LoadOptions) error

	opts = append(opts, awsconfig.WithRegion(cfg.S3Region))

	if cfg.S3AccessKey != "" && cfg.S3SecretKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	var s3Opts []func(*s3.Options)
	if cfg.S3Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.S3Endpoint)
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Opts...)

	return &S3Storage{
		client: client,
		bucket: cfg.StorageBucket,
	}, nil
}

// Upload stores a file and returns the storage key.
func (s *S3Storage) Upload(ctx context.Context, filename string, contentType string, reader io.Reader) (string, error) {
	ext := path.Ext(filename)
	key := fmt.Sprintf("uploads/%s/%s%s", time.Now().Format("2006/01/02"), uuid.New().String(), ext)

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        reader,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload file: %w", err)
	}

	return key, nil
}

// Delete removes a file from storage.
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return nil
}

// GetURL returns a presigned URL for accessing the file.
func (s *S3Storage) GetURL(ctx context.Context, key string) (string, error) {
	presignClient := s3.NewPresignClient(s.client)
	presigned, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(1*time.Hour))
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return presigned.URL, nil
}

// GCSStorage implements Storage using Google Cloud Storage via S3-compatible interop.
type GCSStorage struct {
	s3 *S3Storage
}

// NewGCSStorage creates a GCS storage client using the S3-compatible interoperability API.
func NewGCSStorage(ctx context.Context, cfg *config.Config) (*GCSStorage, error) {
	// GCS provides an S3-compatible endpoint
	gcsCfg := *cfg
	gcsCfg.S3Endpoint = "https://storage.googleapis.com"
	gcsCfg.S3Region = "auto"

	s3Store, err := NewS3Storage(ctx, &gcsCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS storage: %w", err)
	}

	return &GCSStorage{s3: s3Store}, nil
}

// Upload stores a file in GCS.
func (g *GCSStorage) Upload(ctx context.Context, filename string, contentType string, reader io.Reader) (string, error) {
	return g.s3.Upload(ctx, filename, contentType, reader)
}

// Delete removes a file from GCS.
func (g *GCSStorage) Delete(ctx context.Context, key string) error {
	return g.s3.Delete(ctx, key)
}

// GetURL returns a URL for accessing the file in GCS.
func (g *GCSStorage) GetURL(ctx context.Context, key string) (string, error) {
	return g.s3.GetURL(ctx, key)
}

// New creates the appropriate storage backend based on configuration.
func New(ctx context.Context, cfg *config.Config) (Storage, error) {
	switch cfg.StorageBackend {
	case "gcs":
		return NewGCSStorage(ctx, cfg)
	case "s3":
		return NewS3Storage(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported storage backend: %s", cfg.StorageBackend)
	}
}
