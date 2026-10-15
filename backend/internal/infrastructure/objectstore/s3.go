// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package objectstore implements domain.ObjectStore against S3-compatible storage.
package objectstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var tracer = otel.Tracer("objectstore")

const (
	ensureBucketAttempts = 10
	ensureBucketDelay    = 3 * time.Second
)

// Config selects one bucket. Credentials come from the default AWS chain: static
// env credentials locally, the IRSA web-identity token when deployed.
type Config struct {
	Bucket string
	Region string
	// EndpointURL overrides the S3 endpoint; empty means AWS S3.
	EndpointURL string
	// CreateMissingBucket lets startup create the bucket; local development only.
	CreateMissingBucket bool
}

// Store is an S3 bucket client.
type Store struct {
	client       *s3.Client
	bucket       string
	region       string
	createBucket bool
}

// New builds a Store. It makes no network calls; call EnsureBucket before serving traffic.
func New(ctx context.Context, cfg Config) (*Store, error) {
	opts := []func(*config.LoadOptions) error{config.WithRegion(cfg.Region)}
	if cfg.EndpointURL != "" {
		opts = append(opts,
			config.WithBaseEndpoint(cfg.EndpointURL),
			// S3-compatible backends such as nats-s3 reject the SDK's default flexible checksums.
			config.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
			config.WithResponseChecksumValidation(aws.ResponseChecksumValidationWhenRequired),
		)
	}
	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})
	return &Store{client: client, bucket: cfg.Bucket, region: cfg.Region, createBucket: cfg.CreateMissingBucket}, nil
}

// EnsureBucket verifies the bucket exists. With CreateMissingBucket it creates the
// bucket and retries while a local sidecar starts; otherwise it is a single HeadBucket,
// so a permissions error is never mistaken for a backend that is not ready yet.
func (s *Store) EnsureBucket(ctx context.Context, logger *slog.Logger) error {
	if !s.createBucket {
		if err := s.Ping(ctx); err != nil {
			return fmt.Errorf("bucket %q: %w", s.bucket, err)
		}
		return nil
	}
	var err error
	for attempt := 1; attempt <= ensureBucketAttempts; attempt++ {
		if err = s.createIfMissing(ctx); err == nil {
			return nil
		}
		logger.Warn("object store not ready", "bucket", s.bucket, "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(ensureBucketDelay):
		}
	}
	return fmt.Errorf("bucket %q: %w", s.bucket, err)
}

func (s *Store) createIfMissing(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	if err == nil {
		return nil
	}
	var notFound *types.NotFound
	if !errors.As(err, &notFound) {
		return err
	}
	input := &s3.CreateBucketInput{Bucket: aws.String(s.bucket)}
	if s.region != "us-east-1" {
		input.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(s.region),
		}
	}
	_, err = s.client.CreateBucket(ctx, input)
	var owned *types.BucketAlreadyOwnedByYou
	if errors.As(err, &owned) {
		return nil
	}
	return err
}

// Ping checks that the bucket is reachable.
func (s *Store) Ping(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	return err
}

// PutObject implements domain.ObjectStore.
func (s *Store) PutObject(ctx context.Context, key string, data []byte, contentType, cacheControl string) error {
	ctx, span := tracer.Start(ctx, "objectstore.PutObject")
	defer span.End()
	span.SetAttributes(attribute.String("s3.bucket", s.bucket), attribute.Int("s3.size", len(data)))

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
		ContentType:   aws.String(contentType),
		CacheControl:  aws.String(cacheControl),
	})
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("%w: put object: %w", domain.ErrUpstreamUnavailable, err)
	}
	return nil
}

// GetObject implements domain.ObjectStore.
func (s *Store) GetObject(ctx context.Context, key, byteRange string) (*domain.StoredObject, error) {
	ctx, span := tracer.Start(ctx, "objectstore.GetObject")
	defer span.End()
	span.SetAttributes(attribute.String("s3.bucket", s.bucket))

	input := &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}
	if byteRange != "" {
		input.Range = aws.String(byteRange)
	}
	out, err := s.client.GetObject(ctx, input)
	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return nil, domain.ErrFileNotFound
		}
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "InvalidRange" {
			return nil, domain.ErrRangeNotSatisfiable
		}
		span.RecordError(err)
		return nil, fmt.Errorf("%w: get object: %w", domain.ErrUpstreamUnavailable, err)
	}
	return &domain.StoredObject{
		Body:          out.Body,
		ContentType:   aws.ToString(out.ContentType),
		CacheControl:  aws.ToString(out.CacheControl),
		ContentLength: aws.ToInt64(out.ContentLength),
		ContentRange:  aws.ToString(out.ContentRange),
		ETag:          aws.ToString(out.ETag),
		LastModified:  aws.ToTime(out.LastModified),
	}, nil
}

// DeleteObject removes key; deleting a missing key succeeds.
func (s *Store) DeleteObject(ctx context.Context, key string) error {
	ctx, span := tracer.Start(ctx, "objectstore.DeleteObject")
	defer span.End()
	span.SetAttributes(attribute.String("s3.bucket", s.bucket))

	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}); err != nil {
		span.RecordError(err)
		return fmt.Errorf("%w: delete object: %w", domain.ErrUpstreamUnavailable, err)
	}
	return nil
}
