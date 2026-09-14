// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
)

type stubFundingStatsService struct {
	totals func(context.Context) (*models.FundingStatsTotal, error)
}

func (s *stubFundingStatsService) GetFundingTotals(ctx context.Context) (*models.FundingStatsTotal, error) {
	return s.totals(ctx)
}

func TestFundingStatsHandler_GetTotal_OK(t *testing.T) {
	h := handler.NewFundingStatsHandler(&stubFundingStatsService{
		totals: func(context.Context) (*models.FundingStatsTotal, error) {
			return &models.FundingStatsTotal{AmountRaised: 500, AmountSpent: 165.5}, nil
		},
	})
	w := httptest.NewRecorder()
	h.GetTotal(w, httptest.NewRequest(http.MethodGet, "/v1/funding-stats/total", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	var got models.FundingStatsTotal
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AmountSpent != 165.5 {
		t.Errorf("amount_spent = %v; want 165.5", got.AmountSpent)
	}
	if got.AmountRaised != 500 {
		t.Errorf("amount_raised = %v; want 500", got.AmountRaised)
	}
}

func TestFundingStatsHandler_GetTotal_ServiceError(t *testing.T) {
	h := handler.NewFundingStatsHandler(&stubFundingStatsService{
		totals: func(context.Context) (*models.FundingStatsTotal, error) {
			return nil, errors.New("boom")
		},
	})
	w := httptest.NewRecorder()
	h.GetTotal(w, httptest.NewRequest(http.MethodGet, "/v1/funding-stats/total", nil))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d; want 500", w.Code)
	}
}
