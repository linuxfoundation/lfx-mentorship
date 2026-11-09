// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

func TestTermStatusService_CloseEndedTerms(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	now := time.Date(2026, 11, 2, 0, 10, 0, 0, time.UTC)
	closeErr := errors.New("database down")
	var listedAt time.Time
	var attempted []string
	terms := &stubTermRepo{
		listEndedIDs: func(_ context.Context, at time.Time) ([]string, error) {
			listedAt = at
			return []string{"ended", "extended", "deleted", "failing", "ended-too"}, nil
		},
		closeEnded: func(_ context.Context, id string, at time.Time) (*models.ProgramTerm, error) {
			if !at.Equal(now) {
				t.Errorf("closed %s at %s; want %s", id, at, now)
			}
			attempted = append(attempted, id)
			switch id {
			case "extended":
				return nil, domain.ErrInvalidStateTransition
			case "deleted":
				return nil, domain.ErrProgramTermNotFound
			case "failing":
				return nil, closeErr
			}
			return &models.ProgramTerm{ID: id, Status: models.ProgramTermStatusClosed}, nil
		},
	}

	closed, err := service.NewTermStatusService(terms, logger).CloseEndedTerms(context.Background(), now)
	if !errors.Is(err, closeErr) {
		t.Errorf("err = %v; want the failed close reported", err)
	}
	if errors.Is(err, domain.ErrInvalidStateTransition) || errors.Is(err, domain.ErrProgramTermNotFound) {
		t.Errorf("err = %v; want terms that stopped qualifying skipped, not reported", err)
	}
	if closed != 2 {
		t.Errorf("closed = %d; want 2", closed)
	}
	if !listedAt.Equal(now) {
		t.Errorf("listed ended terms at %s; want %s", listedAt, now)
	}
	// A failure must not stop the terms after it from closing.
	if want := []string{"ended", "extended", "deleted", "failing", "ended-too"}; !slices.Equal(attempted, want) {
		t.Errorf("attempted %v; want %v", attempted, want)
	}
}

func TestTermStatusService_CloseEndedTermsListFailure(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	listErr := errors.New("database down")
	closeCalled := false
	terms := &stubTermRepo{
		listEndedIDs: func(context.Context, time.Time) ([]string, error) { return nil, listErr },
		closeEnded: func(context.Context, string, time.Time) (*models.ProgramTerm, error) {
			closeCalled = true
			return nil, nil
		},
	}

	if _, err := service.NewTermStatusService(terms, logger).CloseEndedTerms(context.Background(), time.Now()); !errors.Is(err, listErr) {
		t.Fatalf("err = %v; want %v", err, listErr)
	}
	if closeCalled {
		t.Error("closed a term after the listing failed")
	}
}

func TestTermStatusService_CloseEndedTermsStopsWhenCancelled(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())
	var attempted []string
	terms := &stubTermRepo{
		listEndedIDs: func(context.Context, time.Time) ([]string, error) { return []string{"first", "second"}, nil },
		closeEnded: func(_ context.Context, id string, _ time.Time) (*models.ProgramTerm, error) {
			attempted = append(attempted, id)
			cancel()
			return &models.ProgramTerm{ID: id}, nil
		},
	}

	closed, err := service.NewTermStatusService(terms, logger).CloseEndedTerms(ctx, time.Now())
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; want context.Canceled", err)
	}
	if closed != 1 || !slices.Equal(attempted, []string{"first"}) {
		t.Errorf("closed %d after attempting %v; want 1 after attempting [first]", closed, attempted)
	}
}
