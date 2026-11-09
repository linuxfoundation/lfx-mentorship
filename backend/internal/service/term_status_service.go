// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"go.opentelemetry.io/otel"
)

var termStatusSvcTracer = otel.Tracer("term-status-service")

// TermStatusService closes program terms that have ended, as legacy's update-program-terms-active
// job did.
type TermStatusService struct {
	terms  domain.ProgramTermRepository
	logger *slog.Logger
}

// NewTermStatusService returns a TermStatusService.
func NewTermStatusService(terms domain.ProgramTermRepository, logger *slog.Logger) *TermStatusService {
	return &TermStatusService{terms: terms, logger: logger}
}

// CloseEndedTerms closes every open term that ended before now and has no pending or accepted mentee
// application left, and returns how many it closed. Each term closes in its own transaction, so one
// failure does not hold back the rest; the failures are returned together. A term that stopped
// qualifying after it was listed, such as one whose end date an admin moved, is skipped. Closing is
// idempotent, so a failed run can be rerun.
func (s *TermStatusService) CloseEndedTerms(ctx context.Context, now time.Time) (int, error) {
	ctx, span := termStatusSvcTracer.Start(ctx, "TermStatusService.CloseEndedTerms")
	defer span.End()

	ids, err := s.terms.ListEndedIDs(ctx, now)
	if err != nil {
		span.RecordError(err)
		return 0, fmt.Errorf("list ended terms: %w", err)
	}
	closed := 0
	var errs []error
	for _, id := range ids {
		if ctx.Err() != nil {
			errs = append(errs, ctx.Err())
			break
		}
		term, err := s.terms.CloseEnded(ctx, id, now)
		switch {
		case errors.Is(err, domain.ErrInvalidStateTransition), errors.Is(err, domain.ErrProgramTermNotFound):
			s.logger.Info("term no longer due to close; skipped", "term_id", id, "reason", err)
		case err != nil:
			span.RecordError(err)
			errs = append(errs, fmt.Errorf("close term %s: %w", id, err))
		default:
			closed++
			s.logger.Info("closed ended term", "term_id", term.ID, "program_id", term.ProgramID, "end_date_time", term.EndDateTime)
		}
	}
	return closed, errors.Join(errs...)
}
