// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
)

var platformSummarySvcTracer = otel.Tracer("platform-summary-service")

// PlatformSummaryService orchestrates the public landing summary read.
type PlatformSummaryService struct {
	repo domain.PlatformSummaryRepository
}

// NewPlatformSummaryService returns a PlatformSummaryService.
func NewPlatformSummaryService(repo domain.PlatformSummaryRepository) *PlatformSummaryService {
	return &PlatformSummaryService{repo: repo}
}

// Summary returns the aggregated landing counts.
func (s *PlatformSummaryService) Summary(ctx context.Context) (*models.PlatformSummary, error) {
	ctx, span := platformSummarySvcTracer.Start(ctx, "PlatformSummaryService.Summary")
	defer span.End()

	summary, err := s.repo.Summary(ctx)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get platform summary: %w", err)
	}
	return summary, nil
}
