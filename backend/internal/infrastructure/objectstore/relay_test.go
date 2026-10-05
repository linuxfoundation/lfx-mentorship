// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package objectstore

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

const relayCDNPrefix = "https://cdn.example.org/mentorship"

type fakeQueue struct {
	entries      []domain.ObjectDeletion
	referenced   map[string]bool
	done         []string
	retried      []string
	deadLetter   bool
	referenceErr error
}

func (q *fakeQueue) Schedule(context.Context, domain.ObjectBucket, string, time.Duration) (string, error) {
	return "", nil
}

func (q *fakeQueue) Claim(context.Context, int) ([]domain.ObjectDeletion, error) {
	return q.entries, nil
}

func (q *fakeQueue) IsReferenced(_ context.Context, locator string) (bool, error) {
	return q.referenced[locator], q.referenceErr
}

func (q *fakeQueue) MarkDone(_ context.Context, e domain.ObjectDeletion) error {
	q.done = append(q.done, e.ID)
	return nil
}

func (q *fakeQueue) MarkRetry(_ context.Context, e domain.ObjectDeletion, _ error) (bool, error) {
	q.retried = append(q.retried, e.ID)
	return q.deadLetter, nil
}

type fakeDeleter struct {
	keys []string
	err  error
}

func (d *fakeDeleter) DeleteObject(_ context.Context, key string) error {
	d.keys = append(d.keys, key)
	return d.err
}

func newTestRelay(q *fakeQueue, logos, attachments *fakeDeleter) *Relay {
	buckets := map[domain.ObjectBucket]RelayBucket{}
	if logos != nil {
		buckets[domain.ObjectBucketLogos] = RelayBucket{Store: logos, CDNURLPrefix: relayCDNPrefix}
	}
	if attachments != nil {
		buckets[domain.ObjectBucketAttachments] = RelayBucket{Store: attachments}
	}
	return NewRelay(q, buckets, 10, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestRelay_DeletesMintedUnreferencedObjects(t *testing.T) {
	q := &fakeQueue{entries: []domain.ObjectDeletion{
		{ID: "logo", Bucket: domain.ObjectBucketLogos, Locator: relayCDNPrefix + "/abc-logo.png"},
		{ID: "task", Bucket: domain.ObjectBucketAttachments, Locator: "abc-essay.pdf"},
	}}
	logos, attachments := &fakeDeleter{}, &fakeDeleter{}
	if err := newTestRelay(q, logos, attachments).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(logos.keys) != 1 || logos.keys[0] != "abc-logo.png" || len(attachments.keys) != 1 || attachments.keys[0] != "abc-essay.pdf" {
		t.Fatalf("deleted logos=%v attachments=%v", logos.keys, attachments.keys)
	}
	if len(q.done) != 2 || len(q.retried) != 0 {
		t.Fatalf("done=%v retried=%v", q.done, q.retried)
	}
}

func TestRelay_SkipsReferencedAndForeignLocators(t *testing.T) {
	shared := relayCDNPrefix + "/shared-logo.png"
	q := &fakeQueue{
		entries: []domain.ObjectDeletion{
			{ID: "shared", Bucket: domain.ObjectBucketLogos, Locator: shared},
			{ID: "foreign", Bucket: domain.ObjectBucketLogos, Locator: "https://lf.example/project.png"},
			{ID: "legacy", Bucket: domain.ObjectBucketAttachments, Locator: "https://legacy.example/x.pdf"},
		},
		referenced: map[string]bool{shared: true},
	}
	logos, attachments := &fakeDeleter{}, &fakeDeleter{}
	if err := newTestRelay(q, logos, attachments).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(logos.keys)+len(attachments.keys) != 0 {
		t.Fatalf("must not delete: logos=%v attachments=%v", logos.keys, attachments.keys)
	}
	if len(q.done) != 3 {
		t.Fatalf("skipped entries must complete: done=%v", q.done)
	}
}

func TestRelay_RetriesFailures(t *testing.T) {
	tests := []struct {
		name        string
		q           *fakeQueue
		attachments *fakeDeleter
	}{
		{"delete fails", &fakeQueue{}, &fakeDeleter{err: errors.New("s3 down")}},
		{"reference check fails", &fakeQueue{referenceErr: errors.New("db down")}, &fakeDeleter{}},
		{"bucket not configured", &fakeQueue{}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.q.entries = []domain.ObjectDeletion{{ID: "task", Bucket: domain.ObjectBucketAttachments, Locator: "abc-essay.pdf"}}
			tc.q.deadLetter = true
			if err := newTestRelay(tc.q, nil, tc.attachments).RunOnce(context.Background()); err == nil {
				t.Fatal("RunOnce must report the failure")
			}
			if len(tc.q.retried) != 1 || len(tc.q.done) != 0 {
				t.Fatalf("done=%v retried=%v", tc.q.done, tc.q.retried)
			}
		})
	}
}
