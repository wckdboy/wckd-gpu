// Package s3store talks to any S3-compatible bucket (R2, S3, B2, MinIO).
package s3store

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/wckdboy/wckd-gpu/cli/internal/config"
)

// Store is a path-style S3 client bound to one bucket.
type Store struct {
	Bucket string
	Client *s3.Client
}

// New builds a client. It does not dial the network.
func New(cfg config.S3) (*Store, error) {
	region := cfg.Region
	if region == "" {
		region = "auto"
	}
	awsCfg := aws.Config{
		Region:           region,
		Credentials:      credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		RetryMaxAttempts: 2,
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.UsePathStyle()
	})
	return &Store{Bucket: cfg.Bucket, Client: client}, nil
}

// Check confirms the bucket is reachable. It does not create one.
func (s *Store) Check(ctx context.Context) error {
	_, err := s.Client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.Bucket)})
	if err != nil {
		return fmt.Errorf("s3 head bucket %s: %w", s.Bucket, err)
	}
	return nil
}

// Put writes one object, replacing any previous body.
func (s *Store) Put(ctx context.Context, key string, body []byte) error {
	_, err := s.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.Bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(body),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	return nil
}

// Exists reports whether an object is present.
func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}
	if isNotFound(err) {
		return false, nil
	}
	return false, fmt.Errorf("s3 head %s: %w", key, err)
}

func isNotFound(err error) bool {
	var nf *types.NotFound
	var nk *types.NoSuchKey
	if errors.As(err, &nf) || errors.As(err, &nk) {
		return true
	}
	var api smithy.APIError
	if errors.As(err, &api) {
		switch api.ErrorCode() {
		case "NotFound", "NoSuchKey", "404":
			return true
		default:
			return false
		}
	}
	return false
}
