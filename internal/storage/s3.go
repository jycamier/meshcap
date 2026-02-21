package storage

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	appconfig "github.com/jycamier/meshcap/internal/config"
)

type S3Storage struct {
	bucket   string
	uploader *manager.Uploader
	logger   *slog.Logger
}

func NewS3Storage(ctx context.Context, cfg *appconfig.Config, logger *slog.Logger) (*S3Storage, error) {
	opts := []func(*awsconfig.LoadOptions) error{}
	if cfg.S3Region != "" {
		opts = append(opts, awsconfig.WithRegion(cfg.S3Region))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.S3Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.S3Endpoint)
			o.UsePathStyle = true
		}
	})
	uploader := manager.NewUploader(client)

	return &S3Storage{
		bucket:   cfg.S3Bucket,
		uploader: uploader,
		logger:   logger,
	}, nil
}

func (s *S3Storage) Upload(ctx context.Context, key string, data []byte) error {
	_, err := s.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/octet-stream"),
	})
	if err != nil {
		return fmt.Errorf("s3 upload %q: %w", key, err)
	}

	s.logger.Info("uploaded file to S3", "key", key, "size", len(data))
	return nil
}
