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

func newMemberSvc(memberRepo *stubMemberRepo, progRepo *stubProgRepo, notifier *stubNotifier) *service.ProgramMemberService {
	return service.NewProgramMemberService(memberRepo, progRepo, notifier, "test-secret")
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestProgramMemberService_Create_NonPublishedProgram(t *testing.T) {
	progRepo := &stubProgRepo{
		getByID: func(_ context.Context, _ string) (*models.Program, error) {
			return &models.Program{ID: "prog-1", Status: models.ProgramStatusDraft}, nil
		},
	}
	svc := newMemberSvc(&stubMemberRepo{}, progRepo, &stubNotifier{})
	_, err := svc.Create(context.Background(), "prog-1", models.ProgramMemberCreateInput{
		UserID:     "user-1",
		MemberType: "mentor",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for non-published program, got %v", err)
	}
}

func TestProgramMemberService_Create_InvalidMemberType(t *testing.T) {
	progRepo := &stubProgRepo{
		getByID: func(_ context.Context, _ string) (*models.Program, error) {
			return &models.Program{Status: models.ProgramStatusPublished}, nil
		},
	}
	svc := newMemberSvc(&stubMemberRepo{}, progRepo, &stubNotifier{})
	_, err := svc.Create(context.Background(), "prog-1", models.ProgramMemberCreateInput{
		UserID:     "user-1",
		MemberType: "unknown_role",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for invalid member_type, got %v", err)
	}
}

func TestProgramMemberService_Create_InvalidStatus(t *testing.T) {
	progRepo := &stubProgRepo{
		getByID: func(_ context.Context, _ string) (*models.Program, error) {
			return &models.Program{Status: models.ProgramStatusPublished}, nil
		},
	}
	svc := newMemberSvc(&stubMemberRepo{}, progRepo, &stubNotifier{})
	bad := models.ProgramMemberStatus("teleported")
	_, err := svc.Create(context.Background(), "prog-1", models.ProgramMemberCreateInput{
		UserID:     "user-1",
		MemberType: models.MemberTypeMentor,
		Status:     &bad,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for invalid status, got %v", err)
	}
}

func TestProgramMemberService_Create_MissingUserID(t *testing.T) {
	progRepo := &stubProgRepo{
		getByID: func(_ context.Context, _ string) (*models.Program, error) {
			return &models.Program{Status: models.ProgramStatusPublished}, nil
		},
	}
	svc := newMemberSvc(&stubMemberRepo{}, progRepo, &stubNotifier{})
	_, err := svc.Create(context.Background(), "prog-1", models.ProgramMemberCreateInput{
		MemberType: "mentor",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for missing user_id, got %v", err)
	}
}

func TestProgramMemberService_Create_Mentor_SetsInvitedStatus(t *testing.T) {
	var capturedStatus models.ProgramMemberStatus
	progRepo := &stubProgRepo{
		getByID: func(_ context.Context, _ string) (*models.Program, error) {
			return &models.Program{Status: models.ProgramStatusPublished}, nil
		},
	}
	memberRepo := &stubMemberRepo{
		create: func(_ context.Context, _ string, in models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
			if in.Status != nil {
				capturedStatus = *in.Status
			}
			return &models.ProgramMember{Status: in.Status}, nil
		},
	}
	n := &stubNotifier{}
	svc := newMemberSvc(memberRepo, progRepo, n)
	_, err := svc.Create(context.Background(), "prog-1", models.ProgramMemberCreateInput{
		UserID:     "mentor-1",
		MemberType: "mentor",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if capturedStatus != "invited" {
		t.Errorf("mentor status = %q; want %q", capturedStatus, "invited")
	}
	if n.mentorInvitedCalls != 1 {
		t.Errorf("NotifyMentorInvited called %d times; want 1", n.mentorInvitedCalls)
	}
}

func TestProgramMemberService_Create_ProgramAdmin_SetsActiveStatus(t *testing.T) {
	var capturedStatus models.ProgramMemberStatus
	progRepo := &stubProgRepo{
		getByID: func(_ context.Context, _ string) (*models.Program, error) {
			return &models.Program{Status: models.ProgramStatusPublished}, nil
		},
	}
	memberRepo := &stubMemberRepo{
		create: func(_ context.Context, _ string, in models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
			if in.Status != nil {
				capturedStatus = *in.Status
			}
			return &models.ProgramMember{Status: in.Status}, nil
		},
	}
	svc := newMemberSvc(memberRepo, progRepo, &stubNotifier{})
	_, err := svc.Create(context.Background(), "prog-1", models.ProgramMemberCreateInput{
		UserID:     "admin-1",
		MemberType: "program_admin",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if capturedStatus != "active" {
		t.Errorf("program_admin status = %q; want %q", capturedStatus, "active")
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestProgramMemberService_Update_ValidTransition_InvitedToActive(t *testing.T) {
	invited := models.ProgramMemberStatusInvited
	memberRepo := &stubMemberRepo{
		getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
			return &models.ProgramMember{ID: id, ProgramID: "prog-1", UserID: "u1", Status: &invited}, nil
		},
	}
	svc := newMemberSvc(memberRepo, &stubProgRepo{}, &stubNotifier{})
	next := models.ProgramMemberStatusActive
	_, err := svc.Update(context.Background(), "member-1", models.ProgramMemberUpdateInput{Status: &next})
	if err != nil {
		t.Errorf("invited→active should be valid, got %v", err)
	}
}

func TestProgramMemberService_Update_InvalidTransition_DeclinedToActive(t *testing.T) {
	declined := models.ProgramMemberStatusDeclined
	memberRepo := &stubMemberRepo{
		getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
			return &models.ProgramMember{ID: id, ProgramID: "prog-1", UserID: "u1", Status: &declined}, nil
		},
	}
	svc := newMemberSvc(memberRepo, &stubProgRepo{}, &stubNotifier{})
	next := models.ProgramMemberStatusActive
	_, err := svc.Update(context.Background(), "member-1", models.ProgramMemberUpdateInput{Status: &next})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Errorf("expected ErrInvalidStateTransition for declined→active, got %v", err)
	}
}

func TestProgramMemberService_Update_InvalidTransition_WithdrawnTerminal(t *testing.T) {
	withdrawn := models.ProgramMemberStatusWithdrawn
	memberRepo := &stubMemberRepo{
		getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
			return &models.ProgramMember{ID: id, Status: &withdrawn}, nil
		},
	}
	svc := newMemberSvc(memberRepo, &stubProgRepo{}, &stubNotifier{})
	next := models.ProgramMemberStatusActive
	_, err := svc.Update(context.Background(), "member-1", models.ProgramMemberUpdateInput{Status: &next})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Errorf("expected ErrInvalidStateTransition for withdrawn→active, got %v", err)
	}
}

// An unknown status matches no edge in memberTransitions, so it must be
// rejected as invalid input before the lifecycle check turns it into a
// conflict — and before the member is ever fetched.
func TestProgramMemberService_Update_InvalidStatus(t *testing.T) {
	fetched := false
	memberRepo := &stubMemberRepo{
		getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
			fetched = true
			return &models.ProgramMember{ID: id}, nil
		},
	}
	svc := newMemberSvc(memberRepo, &stubProgRepo{}, &stubNotifier{})
	bad := models.ProgramMemberStatus("teleported")
	_, err := svc.Update(context.Background(), "member-1", models.ProgramMemberUpdateInput{Status: &bad})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for invalid status, got %v", err)
	}
	if fetched {
		t.Error("expected validation to reject before fetching the member")
	}
}

func TestProgramMemberService_Update_Decline_NotifiesMentor(t *testing.T) {
	invited := models.ProgramMemberStatusInvited
	n := &stubNotifier{}
	memberRepo := &stubMemberRepo{
		getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
			return &models.ProgramMember{ID: id, ProgramID: "prog-1", UserID: "mentor-1", Status: &invited}, nil
		},
	}
	svc := newMemberSvc(memberRepo, &stubProgRepo{}, n)
	next := models.ProgramMemberStatusDeclined
	_, err := svc.Update(context.Background(), "member-1", models.ProgramMemberUpdateInput{Status: &next})
	if err != nil {
		t.Fatalf("invited→declined should be valid: %v", err)
	}
	if n.mentorDeclinedCalls != 1 {
		t.Errorf("NotifyMentorDeclined called %d times; want 1", n.mentorDeclinedCalls)
	}
}

// ── AcceptInvite / DeclineInvite ─────────────────────────────────────────────

func TestProgramMemberService_AcceptInvite_InvalidToken(t *testing.T) {
	svc := newMemberSvc(&stubMemberRepo{}, &stubProgRepo{}, &stubNotifier{})
	_, err := svc.AcceptInvite(context.Background(), "not-a-valid-token")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for bad token, got %v", err)
	}
}

func TestProgramMemberService_DeclineInvite_InvalidToken(t *testing.T) {
	svc := newMemberSvc(&stubMemberRepo{}, &stubProgRepo{}, &stubNotifier{})
	err := svc.DeclineInvite(context.Background(), "not-a-valid-token")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for bad token, got %v", err)
	}
}
