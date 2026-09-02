// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler

import (
	"context"
	"net/http"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

type platformSummaryService interface {
	Summary(ctx context.Context) (*models.PlatformSummary, error)
}

// PlatformSummaryHandler holds the Chi handler for the public landing summary.
type PlatformSummaryHandler struct {
	svc platformSummaryService
}

// NewPlatformSummaryHandler creates a PlatformSummaryHandler.
func NewPlatformSummaryHandler(svc platformSummaryService) *PlatformSummaryHandler {
	return &PlatformSummaryHandler{svc: svc}
}

// Get handles GET /v1/summary.
func (h *PlatformSummaryHandler) Get(w http.ResponseWriter, r *http.Request) {
	summary, err := h.svc.Summary(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, summary)
}
