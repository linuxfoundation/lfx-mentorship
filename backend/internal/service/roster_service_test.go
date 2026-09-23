// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

type rosterRepoStub struct {
}

func (s *rosterRepoStub) ListApprovers(context.Context) ([]*models.RosterMember, error) {
	return nil, nil
}
func (s *rosterRepoStub) AddApprover(context.Context, string) (*models.RosterMember, error) {
	return nil, nil
}
func (s *rosterRepoStub) RemoveApprover(context.Context, string) error { return nil }
func TestRosterService_AddApproverRejectsSelfEscalation(t *testing.T) {
	svc := service.NewRosterService(&rosterRepoStub{})
	_, err := svc.AddApprover(context.Background(), "staff-1", "staff-1")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("got %v; want forbidden", err)
	}
}
