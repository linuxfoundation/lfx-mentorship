// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

const (
	rosterActorID  = "550e8400-e29b-41d4-a716-446655440000"
	rosterTargetID = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
)

type rosterRepoStub struct {
	added, removed []string
}

func (s *rosterRepoStub) ListApprovers(context.Context) ([]*models.RosterMember, error) {
	return nil, nil
}
func (s *rosterRepoStub) AddApprover(_ context.Context, userID string) (*models.RosterMember, error) {
	s.added = append(s.added, userID)
	return &models.RosterMember{UserID: userID}, nil
}
func (s *rosterRepoStub) RemoveApprover(_ context.Context, userID string) error {
	s.removed = append(s.removed, userID)
	return nil
}

// rosterCalls runs both roster mutations so every case covers add and remove.
var rosterCalls = map[string]func(*service.RosterService, string, string) error{
	"add": func(svc *service.RosterService, actorID, userID string) error {
		_, err := svc.AddApprover(context.Background(), actorID, userID)
		return err
	},
	"remove": func(svc *service.RosterService, actorID, userID string) error {
		return svc.RemoveApprover(context.Background(), actorID, userID)
	},
}

func TestRosterService_RejectsSelfEscalation(t *testing.T) {
	spellings := map[string]string{
		"canonical":    rosterActorID,
		"uppercase":    strings.ToUpper(rosterActorID),
		"one upper":    "550E8400-e29b-41d4-a716-446655440000",
		"braced":       "{" + rosterActorID + "}",
		"no hyphens":   strings.ReplaceAll(rosterActorID, "-", ""),
		"urn":          "urn:uuid:" + rosterActorID,
		"upper braced": "{" + strings.ToUpper(rosterActorID) + "}",
	}
	for op, call := range rosterCalls {
		for name, userID := range spellings {
			t.Run(op+"/"+name, func(t *testing.T) {
				repo := &rosterRepoStub{}
				err := call(service.NewRosterService(repo), rosterActorID, userID)
				if !errors.Is(err, domain.ErrForbidden) {
					t.Fatalf("got %v; want forbidden", err)
				}
				if len(repo.added)+len(repo.removed) != 0 {
					t.Fatalf("repository was called: added=%v removed=%v", repo.added, repo.removed)
				}
			})
		}
	}
}

func TestRosterService_RejectsInvalidActor(t *testing.T) {
	for op, call := range rosterCalls {
		for _, actorID := range []string{"", "staff-1"} {
			t.Run(op+"/"+actorID, func(t *testing.T) {
				repo := &rosterRepoStub{}
				if err := call(service.NewRosterService(repo), actorID, rosterTargetID); !errors.Is(err, domain.ErrForbidden) {
					t.Fatalf("got %v; want forbidden", err)
				}
				if len(repo.added)+len(repo.removed) != 0 {
					t.Fatalf("repository was called: added=%v removed=%v", repo.added, repo.removed)
				}
			})
		}
	}
}

func TestRosterService_RejectsInvalidUserID(t *testing.T) {
	for op, call := range rosterCalls {
		for _, userID := range []string{"", "not-a-uuid", rosterTargetID + "0"} {
			t.Run(op+"/"+userID, func(t *testing.T) {
				repo := &rosterRepoStub{}
				if err := call(service.NewRosterService(repo), rosterActorID, userID); !errors.Is(err, domain.ErrInvalidInput) {
					t.Fatalf("got %v; want invalid input", err)
				}
				if len(repo.added)+len(repo.removed) != 0 {
					t.Fatalf("repository was called: added=%v removed=%v", repo.added, repo.removed)
				}
			})
		}
	}
}

func TestRosterService_PassesCanonicalUserID(t *testing.T) {
	userID := "{" + strings.ToUpper(rosterTargetID) + "}"

	repo := &rosterRepoStub{}
	svc := service.NewRosterService(repo)
	if _, err := svc.AddApprover(context.Background(), rosterActorID, userID); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := svc.RemoveApprover(context.Background(), rosterActorID, userID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(repo.added) != 1 || repo.added[0] != rosterTargetID {
		t.Fatalf("added %v; want [%s]", repo.added, rosterTargetID)
	}
	if len(repo.removed) != 1 || repo.removed[0] != rosterTargetID {
		t.Fatalf("removed %v; want [%s]", repo.removed, rosterTargetID)
	}
}
