// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// RosterService manages platform-authorized FGA rosters.
type RosterService struct{ repo domain.RosterRepository }

func NewRosterService(repo domain.RosterRepository) *RosterService { return &RosterService{repo: repo} }

func (s *RosterService) ListApprovers(ctx context.Context) ([]*models.RosterMember, error) {
	return s.repo.ListApprovers(ctx)
}
func (s *RosterService) AddApprover(ctx context.Context, actorID, userID string) (*models.RosterMember, error) {
	if actorID == "" || actorID == userID {
		return nil, fmt.Errorf("%w: approver self-escalation is forbidden", domain.ErrForbidden)
	}
	return s.repo.AddApprover(ctx, userID)
}
func (s *RosterService) RemoveApprover(ctx context.Context, actorID, userID string) error {
	if actorID == "" || actorID == userID {
		return fmt.Errorf("%w: approver self-escalation is forbidden", domain.ErrForbidden)
	}
	return s.repo.RemoveApprover(ctx, userID)
}
