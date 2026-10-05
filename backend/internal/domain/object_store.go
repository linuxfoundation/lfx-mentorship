// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import (
	"context"
	"io"
	"net/url"
	"strings"
	"time"
)

// ObjectStore reads and writes file payloads in a single bucket.
type ObjectStore interface {
	// PutObject writes data under key, setting the object's Content-Type and Cache-Control.
	PutObject(ctx context.Context, key string, data []byte, contentType, cacheControl string) error
	// GetObject opens key for reading. byteRange is an HTTP Range header value, or empty
	// for the whole object. A missing key returns ErrFileNotFound.
	GetObject(ctx context.Context, key, byteRange string) (*StoredObject, error)
}

// StoredObject is an open object body plus the metadata a download response needs.
// The caller must close Body.
type StoredObject struct {
	Body          io.ReadCloser
	ContentType   string
	CacheControl  string
	ContentLength int64
	// ContentRange is set when only part of the object was returned.
	ContentRange string
	ETag         string
	LastModified time.Time
}

// ObjectBucket names a bucket by file class; the physical bucket name is per-environment config.
type ObjectBucket string

const (
	// ObjectBucketLogos is the public, CDN-fronted bucket; its locators are CDN URLs.
	ObjectBucketLogos ObjectBucket = "logos"
	// ObjectBucketAttachments is the private bucket; its locators are object keys.
	ObjectBucketAttachments ObjectBucket = "attachments"
)

// IsValid reports whether the bucket is one of the allowed enum members.
func (b ObjectBucket) IsValid() bool {
	return b == ObjectBucketLogos || b == ObjectBucketAttachments
}

// ObjectKeyFromLocator returns the object key a file column value points at. It reports
// false for a locator this service did not mint — a foreign URL such as an LF project
// logo, or a legacy URL in a private column — which must never be fetched or deleted.
func ObjectKeyFromLocator(bucket ObjectBucket, locator, cdnURLPrefix string) (string, bool) {
	switch bucket {
	case ObjectBucketLogos:
		if cdnURLPrefix == "" {
			return "", false
		}
		escaped, ok := strings.CutPrefix(locator, cdnURLPrefix+"/")
		if !ok {
			return "", false
		}
		key, err := url.PathUnescape(escaped)
		return key, err == nil && key != ""
	case ObjectBucketAttachments:
		if locator == "" || strings.Contains(locator, "://") {
			return "", false
		}
		return locator, true
	}
	return "", false
}

// ObjectDeletion is a claimed object_deletions entry.
type ObjectDeletion struct {
	ID        string
	Bucket    ObjectBucket
	Locator   string
	Attempts  int
	ClaimedAt *time.Time
}

// ObjectDeletionRepository is the transactional queue every dropped file locator goes through.
type ObjectDeletionRepository interface {
	// Schedule queues locator for deletion after delay in its own transaction, returning the entry ID.
	Schedule(ctx context.Context, bucket ObjectBucket, locator string, delay time.Duration) (string, error)
	// Claim moves due entries to in_flight.
	Claim(ctx context.Context, limit int) ([]ObjectDeletion, error)
	// IsReferenced reports whether any file column still holds locator.
	IsReferenced(ctx context.Context, locator string) (bool, error)
	MarkDone(ctx context.Context, entry ObjectDeletion) error
	// MarkRetry reschedules the entry, dead-lettering it once attempts are exhausted.
	MarkRetry(ctx context.Context, entry ObjectDeletion, cause error) (deadLettered bool, err error)
}
