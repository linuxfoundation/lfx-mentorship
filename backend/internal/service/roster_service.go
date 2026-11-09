// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
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
	target, err := rosterTarget(actorID, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.AddApprover(ctx, target)
}
func (s *RosterService) RemoveApprover(ctx context.Context, actorID, userID string) error {
	target, err := rosterTarget(actorID, userID)
	if err != nil {
		return err
	}
	return s.repo.RemoveApprover(ctx, target)
}

// rosterTarget validates a roster change and returns the target user's
// canonical UUID. Both IDs are compared as UUID values, not strings:
// Postgres accepts uppercase, braced and un-hyphenated spellings of the
// same UUID, so a string comparison would let an actor add themselves.
func rosterTarget(actorID, userID string) (string, error) {
	actor, err := uuid.Parse(actorID)
	if err != nil {
		return "", fmt.Errorf("%w: approver self-escalation is forbidden", domain.ErrForbidden)
	}
	target, err := uuid.Parse(userID)
	if err != nil {
		return "", fmt.Errorf("%w: user_id must be a UUID", domain.ErrInvalidInput)
	}
	if actor == target {
		return "", fmt.Errorf("%w: approver self-escalation is forbidden", domain.ErrForbidden)
	}
	return target.String(), nil
}
