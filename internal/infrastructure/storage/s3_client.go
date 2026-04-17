package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"brokle/internal/config"
)

// S3Client wraps AWS S3 SDK for blob storage operations
type S3Client struct {
	client     *s3.Client
	logger     *slog.Logger
	bucketName string
}

// NewS3Client creates a new S3 client instance
func NewS3Client(cfg *config.BlobStorageConfig, logger *slog.Logger) (*S3Client, error) {
	var awsCfg aws.Config
	var err error

	if cfg.Endpoint != "" {
		awsCfg, err = awsConfig.LoadDefaultConfig(context.Background(),
			awsConfig.WithRegion(cfg.Region),
			awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				cfg.AccessKeyID,
				cfg.SecretAccessKey,
				"",
			)),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load AWS config: %w", err)
		}
		awsCfg.BaseEndpoint = aws.String(cfg.Endpoint)
	} else {
		if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
			awsCfg, err = awsConfig.LoadDefaultConfig(context.Background(),
				awsConfig.WithRegion(cfg.Region),
				awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
					cfg.AccessKeyID,
					cfg.SecretAccessKey,
					"",
				)),
			)
		} else {
			awsCfg, err = awsConfig.LoadDefaultConfig(context.Background(),
				awsConfig.WithRegion(cfg.Region),
			)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to load AWS config: %w", err)
		}
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.UsePathStyle
	})

	logger.Info("S3 client initialized", "provider", cfg.Provider, "bucket", cfg.BucketName, "region", cfg.Region, "endpoint", cfg.Endpoint, "path_style", cfg.UsePathStyle)

	return &S3Client{
		client:     s3Client,
		bucketName: cfg.BucketName,
		logger:     logger,
	}, nil
}

// Upload uploads content to S3 using the configured bucket
func (c *S3Client) Upload(ctx context.Context, key string, content []byte, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(c.bucketName),
		Key:         aws.String(key),
		Body:        bytes.NewReader(content),
		ContentType: aws.String(contentType),
	}

	_, err := c.client.PutObject(ctx, input)
	if err != nil {
		c.logger.Error("Failed to upload to S3", "error", err, "bucket", c.bucketName, "key", key)
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	c.logger.Debug("Successfully uploaded to S3", "bucket", c.bucketName, "key", key, "size", len(content))

	return nil
}

// GetBucketName returns the configured bucket name
func (c *S3Client) GetBucketName() string {
	return c.bucketName
}

// Download downloads content from S3
func (c *S3Client) Download(ctx context.Context, key string) ([]byte, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	}

	result, err := c.client.GetObject(ctx, input)
	if err != nil {
		c.logger.Error("Failed to download from S3", "error", err, "bucket", c.bucketName, "key", key)
		return nil, fmt.Errorf("failed to download from S3: %w", err)
	}
	defer result.Body.Close()

	content, err := io.ReadAll(result.Body)
	if err != nil {
		c.logger.Error("Failed to read S3 object body", "error", err)
		return nil, fmt.Errorf("failed to read S3 object body: %w", err)
	}

	c.logger.Debug("Successfully downloaded from S3", "bucket", c.bucketName, "key", key, "size", len(content))

	return content, nil
}

// Delete deletes an object from S3
func (c *S3Client) Delete(ctx context.Context, key string) error {
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	}

	_, err := c.client.DeleteObject(ctx, input)
	if err != nil {
		c.logger.Error("Failed to delete from S3", "error", err, "bucket", c.bucketName, "key", key)
		return fmt.Errorf("failed to delete from S3: %w", err)
	}

	c.logger.Debug("Successfully deleted from S3", "bucket", c.bucketName, "key", key)

	return nil
}

// Exists reports whether an object exists at the given key.
//
// Only "missing object" responses from S3 are collapsed to (false, nil). Every
// other error (permission denied, throttling, 5xx, network failure) is
// propagated so callers can distinguish "key absent" from "we can't tell right
// now." Treating transient failures as "missing" silently breaks uniqueness
// checks that assume a false result means safe-to-create.
//
// aws-sdk-go-v2 quirk: HeadObject returns an un-modeled smithy.GenericAPIError
// with code "NotFound" — it does not return *types.NoSuchKey (that typed error
// is emitted only by GetObject).
func (c *S3Client) Exists(ctx context.Context, key string) (bool, error) {
	_, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey":
			return false, nil
		}
	}
	return false, fmt.Errorf("s3 head object %q: %w", key, err)
}

// GetS3URI returns the full S3 URI for a key
func (c *S3Client) GetS3URI(key string) string {
	return fmt.Sprintf("s3://%s/%s", c.bucketName, key)
}
