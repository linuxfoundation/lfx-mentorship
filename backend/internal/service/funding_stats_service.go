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

var fundingStatsSvcTracer = otel.Tracer("funding-stats-service")

// FundingStatsService provides public funding aggregate reads.
type FundingStatsService struct {
	repo domain.FundingStatsRepository
}

// NewFundingStatsService creates a FundingStatsService.
func NewFundingStatsService(repo domain.FundingStatsRepository) *FundingStatsService {
	return &FundingStatsService{repo: repo}
}

// GetFundingTotals returns the total raised and spent across all program funding stats.
func (s *FundingStatsService) GetFundingTotals(ctx context.Context) (*models.FundingStatsTotal, error) {
	ctx, span := fundingStatsSvcTracer.Start(ctx, "FundingStatsService.GetFundingTotals")
	defer span.End()

	amountRaised, amountSpent, err := s.repo.GetFundingTotals(ctx)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get funding totals: %w", err)
	}
	return &models.FundingStatsTotal{AmountRaised: amountRaised, AmountSpent: amountSpent}, nil
}
