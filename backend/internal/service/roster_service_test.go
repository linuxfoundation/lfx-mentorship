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
	addProject func(context.Context, string, string) (*models.RosterMember, error)
}

func (s *rosterRepoStub) ListApprovers(context.Context) ([]*models.RosterMember, error) {
	return nil, nil
}
func (s *rosterRepoStub) AddApprover(context.Context, string) (*models.RosterMember, error) {
	return nil, nil
}
func (s *rosterRepoStub) RemoveApprover(context.Context, string) error { return nil }
func (s *rosterRepoStub) ListProjectAdmins(context.Context, string) ([]*models.RosterMember, error) {
	return nil, nil
}
func (s *rosterRepoStub) AddProjectAdmin(ctx context.Context, project, user string) (*models.RosterMember, error) {
	if s.addProject != nil {
		return s.addProject(ctx, project, user)
	}
	return &models.RosterMember{}, nil
}
func (s *rosterRepoStub) RemoveProjectAdmin(context.Context, string, string) error { return nil }

func TestRosterService_AddApproverRejectsSelfEscalation(t *testing.T) {
	svc := service.NewRosterService(&rosterRepoStub{})
	_, err := svc.AddApprover(context.Background(), "staff-1", "staff-1")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("got %v; want forbidden", err)
	}
}

func TestRosterService_AddProjectAdminRejectsSelfEscalation(t *testing.T) {
	svc := service.NewRosterService(&rosterRepoStub{})
	_, err := svc.AddProjectAdmin(context.Background(), "staff-1", "project-1", "staff-1")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("got %v; want forbidden", err)
	}
}

func TestRosterService_AddProjectAdminPassesProjectAndUser(t *testing.T) {
	called := false
	svc := service.NewRosterService(&rosterRepoStub{addProject: func(_ context.Context, project, user string) (*models.RosterMember, error) {
		called = project == "project-1" && user == "user-2"
		return &models.RosterMember{}, nil
	}})
	if _, err := svc.AddProjectAdmin(context.Background(), "staff-1", "project-1", "user-2"); err != nil {
		t.Fatalf("AddProjectAdmin: %v", err)
	}
	if !called {
		t.Fatal("repository did not receive project and user")
	}
}
