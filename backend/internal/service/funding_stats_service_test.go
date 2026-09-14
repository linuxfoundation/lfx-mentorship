// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

type stubFundingStatsRepo struct {
	totals func(context.Context) (float64, float64, error)
}

func (s *stubFundingStatsRepo) GetFundingTotals(ctx context.Context) (float64, float64, error) {
	return s.totals(ctx)
}

func TestFundingStatsService_GetTotalAmountSpent(t *testing.T) {
	svc := service.NewFundingStatsService(&stubFundingStatsRepo{
		totals: func(context.Context) (float64, float64, error) { return 500, 165.5, nil },
	})

	got, err := svc.GetFundingTotals(context.Background())
	if err != nil {
		t.Fatalf("GetTotalAmountSpent: %v", err)
	}
	if got.AmountSpent != 165.5 {
		t.Errorf("amount_spent = %v; want 165.5", got.AmountSpent)
	}
	if got.AmountRaised != 500 {
		t.Errorf("amount_raised = %v; want 500", got.AmountRaised)
	}
}

func TestFundingStatsService_GetTotalAmountSpent_RepoError(t *testing.T) {
	sentinel := errors.New("boom")
	svc := service.NewFundingStatsService(&stubFundingStatsRepo{
		totals: func(context.Context) (float64, float64, error) { return 0, 0, sentinel },
	})

	_, err := svc.GetFundingTotals(context.Background())
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v; want wrapped sentinel", err)
	}
}
