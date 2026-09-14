// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler

import (
	"context"
	"net/http"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

type fundingStatsService interface {
	GetFundingTotals(ctx context.Context) (*models.FundingStatsTotal, error)
}

// FundingStatsHandler holds public funding aggregate handlers.
type FundingStatsHandler struct {
	svc fundingStatsService
}

// NewFundingStatsHandler creates a FundingStatsHandler.
func NewFundingStatsHandler(svc fundingStatsService) *FundingStatsHandler {
	return &FundingStatsHandler{svc: svc}
}

// GetTotal handles GET /v1/funding-stats/total.
func (h *FundingStatsHandler) GetTotal(w http.ResponseWriter, r *http.Request) {
	total, err := h.svc.GetFundingTotals(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, total)
}
