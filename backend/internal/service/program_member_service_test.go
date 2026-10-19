// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

func newMemberSvc(memberRepo *stubMemberRepo, progRepo *stubProgRepo, notifier *stubNotifier) *service.ProgramMemberService {
	if memberRepo.findByProgramUser == nil {
		approved := models.ProgramMemberStatusApproved
		memberRepo.findByProgramUser = func(_ context.Context, programID, userID string) (*models.ProgramMember, error) {
			return &models.ProgramMember{ProgramID: programID, UserID: userID, MemberType: models.MemberTypeProgramAdmin, Status: &approved}, nil
		}
	}
	return service.NewProgramMemberService(memberRepo, progRepo, &stubLFIDUsers{ids: map[string]string{"mentor-lfid": "mentor-1"}}, nil, notifier, "test-secret")
}

type stubLFIDUsers struct {
	stubUserRepository
	ids    map[string]string
	emails map[string]string
}

func (s *stubLFIDUsers) GetByLFID(_ context.Context, lfid string) (*models.User, error) {
	if id, ok := s.ids[lfid]; ok {
		u := &models.User{ID: id, LFID: &lfid}
		if e, ok := s.emails[lfid]; ok {
			u.Email = &e
		}
		return u, nil
	}
	return nil, domain.ErrUserNotFound
}

func TestProgramMemberService_Create_ResolvesLFID(t *testing.T) {
	var captured models.ProgramMemberCreateInput
	memberRepo := &stubMemberRepo{
		create: func(_ context.Context, _ string, in models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
			captured = in
			return &models.ProgramMember{UserID: in.UserID, Status: in.Status}, nil
		},
	}
	n := &stubNotifier{}
	_, err := newMemberSvc(memberRepo, publishedProgRepo(), n).Create(context.Background(), "prog-1", models.ProgramMemberCreateInput{LFID: " mentor-lfid ", MemberType: models.MemberTypeMentor})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if captured.UserID != "mentor-1" {
		t.Errorf("user_id = %q; want mentor-1", captured.UserID)
	}
	if n.mentorInvitedCalls != 1 {
		t.Errorf("NotifyMentorInvited called %d times; want 1", n.mentorInvitedCalls)
	}
}

func TestProgramMemberService_Create_LFIDErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		input models.ProgramMemberCreateInput
		want  error
	}{
		"unknown lfid": {models.ProgramMemberCreateInput{LFID: "nobody", MemberType: models.MemberTypeMentor}, domain.ErrIneligible},
		"blank lfid":   {models.ProgramMemberCreateInput{LFID: "   ", MemberType: models.MemberTypeMentor}, domain.ErrInvalidInput},
		"both ids":     {models.ProgramMemberCreateInput{UserID: "mentor-1", LFID: "mentor-lfid", MemberType: models.MemberTypeMentor}, domain.ErrInvalidInput},
	} {
		created := false
		memberRepo := &stubMemberRepo{
			create: func(context.Context, string, models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
				created = true
				return &models.ProgramMember{}, nil
			},
		}
		_, err := newMemberSvc(memberRepo, publishedProgRepo(), &stubNotifier{}).Create(context.Background(), "prog-1", tc.input)
		if !errors.Is(err, tc.want) || created {
			t.Errorf("%s: err = %v, created = %v; want %v and no row", name, err, created, tc.want)
		}
	}
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestProgramMemberService_Create_NonPublishedProgram(t *testing.T) {
	progRepo := &stubProgRepo{
		getByID: func(_ context.Context, _ string) (*models.Program, error) {
			return &models.Program{ID: "prog-1", Status: models.ProgramStatusPending}, nil
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
		ActorID:    "admin-1",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if capturedStatus != "invited" {
		t.Errorf("mentor status = %q; want %q", capturedStatus, "invited")
	}
	if n.mentorInvitedCalls != 1 || !slices.Equal(n.mentorInviters, []string{"admin-1"}) {
		t.Errorf("NotifyMentorInvited called %d times from %v; want once from admin-1", n.mentorInvitedCalls, n.mentorInviters)
	}
}

func TestProgramMemberService_Create_ReinvitesExistingMentor(t *testing.T) {
	for _, status := range []models.ProgramMemberStatus{
		models.ProgramMemberStatusInvited,
		models.ProgramMemberStatusDeclined,
		models.ProgramMemberStatusWithdrawn,
		models.ProgramMemberStatusApproved,
		models.ProgramMemberStatusRequested,
		models.ProgramMemberStatusPending,
	} {
		reinvitable := status == models.ProgramMemberStatusInvited || status == models.ProgramMemberStatusDeclined || status == models.ProgramMemberStatusWithdrawn
		var updatedTo models.ProgramMemberStatus
		var guard []models.ProgramMemberStatus
		memberRepo := &stubMemberRepo{
			findByUserAndType: func(context.Context, string, string, models.MemberType) (*models.ProgramMember, error) {
				return &models.ProgramMember{ID: "member-1", Status: &status}, nil
			},
			updateIfStatus: func(_ context.Context, id string, from []models.ProgramMemberStatus, in models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
				updatedTo, guard = *in.Status, from
				return &models.ProgramMember{ID: id, Status: in.Status}, nil
			},
			create: func(context.Context, string, models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
				t.Fatalf("%s: an existing mentor row must not be inserted again", status)
				return nil, nil
			},
		}
		n := &stubNotifier{}
		m, err := newMemberSvc(memberRepo, publishedProgRepo(), n).Create(context.Background(), "prog-1", models.ProgramMemberCreateInput{UserID: "mentor-1", MemberType: models.MemberTypeMentor, ActorID: "admin-1"})
		if reinvitable {
			if err != nil || m.ID != "member-1" || updatedTo != models.ProgramMemberStatusInvited || !slices.Equal(n.mentorInviters, []string{"admin-1"}) {
				t.Errorf("%s: err = %v, row = %v, status = %q, inviters = %v; want the row back to invited and one email from admin-1", status, err, m, updatedTo, n.mentorInviters)
			}
			// A concurrent answer between the read and the write must fail the update, not be overwritten.
			if len(guard) != 1 || guard[0] != status {
				t.Errorf("%s: update guarded by %v; want only the observed status", status, guard)
			}
			continue
		}
		if !errors.Is(err, domain.ErrConflict) || updatedTo != "" || n.mentorInvitedCalls != 0 {
			t.Errorf("%s: err = %v, status = %q, invites = %d; want ErrConflict and nothing changed", status, err, updatedTo, n.mentorInvitedCalls)
		}
	}
}

func TestProgramMemberService_Create_ReinviteErrors(t *testing.T) {
	lookupFailed := errors.New("db down")
	for name, tc := range map[string]struct {
		find   func(context.Context, string, string, models.MemberType) (*models.ProgramMember, error)
		update func(context.Context, string, []models.ProgramMemberStatus, models.ProgramMemberUpdateInput) (*models.ProgramMember, error)
		want   error
	}{
		"lookup fails": {
			find: func(context.Context, string, string, models.MemberType) (*models.ProgramMember, error) {
				return nil, lookupFailed
			},
			update: func(context.Context, string, []models.ProgramMemberStatus, models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
				t.Fatal("repo.UpdateIfStatus must not be called")
				return nil, nil
			},
			want: lookupFailed,
		},
		"concurrent change": {
			find: func(context.Context, string, string, models.MemberType) (*models.ProgramMember, error) {
				return &models.ProgramMember{ID: "member-1", Status: memberStatus(models.ProgramMemberStatusDeclined)}, nil
			},
			update: func(context.Context, string, []models.ProgramMemberStatus, models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
				return nil, domain.ErrInvalidStateTransition
			},
			want: domain.ErrInvalidStateTransition,
		},
	} {
		memberRepo := &stubMemberRepo{
			findByUserAndType: tc.find,
			updateIfStatus:    tc.update,
			create: func(context.Context, string, models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
				t.Fatalf("%s: a failed re-invite must not insert a row", name)
				return nil, nil
			},
		}
		n := &stubNotifier{}
		_, err := newMemberSvc(memberRepo, publishedProgRepo(), n).Create(context.Background(), "prog-1", models.ProgramMemberCreateInput{UserID: "mentor-1", MemberType: models.MemberTypeMentor})
		if !errors.Is(err, tc.want) || n.mentorInvitedCalls != 0 {
			t.Errorf("%s: err = %v, invites = %d; want %v and no email", name, err, n.mentorInvitedCalls, tc.want)
		}
	}
}

func TestProgramMemberService_Create_ProgramAdmin_SetsApprovedStatus(t *testing.T) {
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
	if capturedStatus != "approved" {
		t.Errorf("program_admin status = %q; want %q", capturedStatus, "approved")
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestProgramMemberService_Update_ValidTransition_InvitedToApproved(t *testing.T) {
	invited := models.ProgramMemberStatusInvited
	approved := models.ProgramMemberStatusApproved
	memberRepo := &stubMemberRepo{
		findByProgramUser: func(_ context.Context, _, _ string) (*models.ProgramMember, error) {
			return &models.ProgramMember{MemberType: models.MemberTypeProgramAdmin, Status: &approved}, nil
		},
		getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
			return &models.ProgramMember{ID: id, ProgramID: "prog-1", UserID: "u1", Status: &invited}, nil
		},
	}
	svc := newMemberSvc(memberRepo, &stubProgRepo{}, &stubNotifier{})
	next := models.ProgramMemberStatusApproved
	_, err := svc.Update(context.Background(), "prog-1", "member-1", models.ProgramMemberUpdateInput{Status: &next}, "admin-1")
	if err != nil {
		t.Errorf("invited→approved should be valid, got %v", err)
	}
}

func TestProgramMemberService_Update_RejectsMemberFromDifferentProgram_WithoutRepoWrite(t *testing.T) {
	invited := models.ProgramMemberStatusInvited
	approved := models.ProgramMemberStatusApproved
	updateCalled := false
	memberRepo := &stubMemberRepo{
		findByProgramUser: func(_ context.Context, _, _ string) (*models.ProgramMember, error) {
			return &models.ProgramMember{MemberType: models.MemberTypeProgramAdmin, Status: &approved}, nil
		},
		getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
			return &models.ProgramMember{ID: id, ProgramID: "prog-other", UserID: "u1", Status: &invited}, nil
		},
		update: func(_ context.Context, _ string, _ models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
			updateCalled = true
			return &models.ProgramMember{}, nil
		},
		updateIfStatus: func(context.Context, string, []models.ProgramMemberStatus, models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
			updateCalled = true
			return &models.ProgramMember{}, nil
		},
	}

	svc := newMemberSvc(memberRepo, &stubProgRepo{}, &stubNotifier{})
	next := models.ProgramMemberStatusApproved
	_, err := svc.Update(context.Background(), "prog-1", "member-1", models.ProgramMemberUpdateInput{Status: &next}, "admin-1")
	if !errors.Is(err, domain.ErrProgramMemberNotFound) {
		t.Fatalf("expected ErrProgramMemberNotFound, got %v", err)
	}
	if updateCalled {
		t.Fatal("expected repo.Update not to be called for cross-program member")
	}
}

func TestProgramMemberService_Update_InvalidTransition_DeclinedToApproved(t *testing.T) {
	declined := models.ProgramMemberStatusDeclined
	memberRepo := &stubMemberRepo{
		getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
			return &models.ProgramMember{ID: id, ProgramID: "prog-1", UserID: "u1", Status: &declined}, nil
		},
	}
	svc := newMemberSvc(memberRepo, &stubProgRepo{}, &stubNotifier{})
	next := models.ProgramMemberStatusApproved
	_, err := svc.Update(context.Background(), "prog-1", "member-1", models.ProgramMemberUpdateInput{Status: &next}, "admin-1")
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Errorf("expected ErrInvalidStateTransition for declined→approved, got %v", err)
	}
}

func TestProgramMemberService_Update_InvalidTransition_WithdrawnTerminal(t *testing.T) {
	withdrawn := models.ProgramMemberStatusWithdrawn
	memberRepo := &stubMemberRepo{
		getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
			return &models.ProgramMember{ID: id, ProgramID: "prog-1", Status: &withdrawn}, nil
		},
	}
	svc := newMemberSvc(memberRepo, &stubProgRepo{}, &stubNotifier{})
	next := models.ProgramMemberStatusApproved
	_, err := svc.Update(context.Background(), "prog-1", "member-1", models.ProgramMemberUpdateInput{Status: &next}, "admin-1")
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Errorf("expected ErrInvalidStateTransition for withdrawn→approved, got %v", err)
	}
}

// An unknown status matches no edge in memberTransitions, so it must be
// rejected as invalid input before the lifecycle check turns it into a
// conflict — and before the member is ever fetched.
// "active" was the stored spelling before the rename to "approved"; it must now be rejected.
func TestProgramMemberService_Update_InvalidStatus(t *testing.T) {
	for _, bad := range []models.ProgramMemberStatus{"teleported", "active"} {
		t.Run(string(bad), func(t *testing.T) {
			fetched := false
			memberRepo := &stubMemberRepo{
				getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
					fetched = true
					return &models.ProgramMember{ID: id}, nil
				},
			}
			svc := newMemberSvc(memberRepo, &stubProgRepo{}, &stubNotifier{})
			_, err := svc.Update(context.Background(), "prog-1", "member-1", models.ProgramMemberUpdateInput{Status: &bad}, "admin-1")
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("expected ErrInvalidInput for invalid status, got %v", err)
			}
			if fetched {
				t.Error("expected validation to reject before fetching the member")
			}
		})
	}
}

func TestProgramMemberService_Update_Decline_NotifiesMentor(t *testing.T) {
	tests := []struct {
		from      models.ProgramMemberStatus
		wantCalls int
	}{
		{from: models.ProgramMemberStatusRequested, wantCalls: 1},
		{from: models.ProgramMemberStatusInvited, wantCalls: 0},
	}
	for _, tt := range tests {
		t.Run(string(tt.from), func(t *testing.T) {
			from := tt.from
			n := &stubNotifier{}
			memberRepo := &stubMemberRepo{
				getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
					return &models.ProgramMember{ID: id, ProgramID: "prog-1", UserID: "mentor-1", Status: &from}, nil
				},
			}
			svc := newMemberSvc(memberRepo, &stubProgRepo{}, n)
			next := models.ProgramMemberStatusDeclined
			if _, err := svc.Update(context.Background(), "prog-1", "member-1", models.ProgramMemberUpdateInput{Status: &next}, "admin-1"); err != nil {
				t.Fatalf("%s→declined should be valid: %v", from, err)
			}
			if n.mentorDeclinedCalls != tt.wantCalls {
				t.Errorf("NotifyMentorDeclined called %d times; want %d", n.mentorDeclinedCalls, tt.wantCalls)
			}
		})
	}
}

// ── AcceptInvite / DeclineInvite ─────────────────────────────────────────────

func TestProgramMemberService_AcceptInvite_InvalidToken(t *testing.T) {
	svc := newMemberSvc(&stubMemberRepo{}, &stubProgRepo{}, &stubNotifier{})
	_, err := svc.AcceptInvite(context.Background(), "not-a-valid-token", "user-1")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for bad token, got %v", err)
	}
}

func TestProgramMemberService_DeclineInvite_InvalidToken(t *testing.T) {
	svc := newMemberSvc(&stubMemberRepo{}, &stubProgRepo{}, &stubNotifier{})
	err := svc.DeclineInvite(context.Background(), "not-a-valid-token", "user-1")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for bad token, got %v", err)
	}
}

func TestProgramMemberService_AcceptInvite_NotifiesAdminsOnlyAfterWrite(t *testing.T) {
	token, err := auth.GenerateInviteToken("prog-1", "mentor-1", "test-secret")
	if err != nil {
		t.Fatalf("generate invite token: %v", err)
	}
	invited := models.ProgramMemberStatusInvited
	for name, writeErr := range map[string]error{"write succeeds": nil, "write fails": errors.New("db down")} {
		t.Run(name, func(t *testing.T) {
			memberRepo := &stubMemberRepo{
				findByUserAndType: func(context.Context, string, string, models.MemberType) (*models.ProgramMember, error) {
					return &models.ProgramMember{ID: "member-1", UserID: "mentor-1", Status: &invited}, nil
				},
				updateIfStatus: func(_ context.Context, id string, _ []models.ProgramMemberStatus, _ models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
					if writeErr != nil {
						return nil, writeErr
					}
					return &models.ProgramMember{ID: id}, nil
				},
			}
			n := &stubNotifier{}
			_, err := newMemberSvc(memberRepo, &stubProgRepo{}, n).AcceptInvite(context.Background(), token, "mentor-1")
			want := 1
			if writeErr != nil {
				want = 0
				if err == nil {
					t.Fatal("expected error")
				}
			} else if err != nil {
				t.Fatalf("AcceptInvite: %v", err)
			}
			if n.adminMentorAcceptedCalls != want {
				t.Errorf("NotifyAdminMentorAccepted calls = %d; want %d", n.adminMentorAcceptedCalls, want)
			}
		})
	}
}

func TestProgramMemberService_InviteRejectsMismatchedPrincipal(t *testing.T) {
	token, err := auth.GenerateInviteToken("prog-1", "mentor-1", "test-secret")
	if err != nil {
		t.Fatalf("generate invite token: %v", err)
	}
	svc := newMemberSvc(&stubMemberRepo{}, &stubProgRepo{}, &stubNotifier{})

	if _, err := svc.AcceptInvite(context.Background(), token, "different-user"); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("accept should reject mismatched principal with ErrForbidden, got %v", err)
	}
	if err := svc.DeclineInvite(context.Background(), token, "different-user"); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("decline should reject mismatched principal with ErrForbidden, got %v", err)
	}
}

func TestProgramMemberService_DeclineInvite_NotifiesAdmins(t *testing.T) {
	token, err := auth.GenerateInviteToken("prog-1", "mentor-1", "test-secret")
	if err != nil {
		t.Fatalf("generate invite token: %v", err)
	}
	invited := models.ProgramMemberStatusInvited
	memberRepo := &stubMemberRepo{
		findByUserAndType: func(context.Context, string, string, models.MemberType) (*models.ProgramMember, error) {
			return &models.ProgramMember{ID: "member-1", UserID: "mentor-1", Status: &invited}, nil
		},
	}
	n := &stubNotifier{}
	svc := newMemberSvc(memberRepo, &stubProgRepo{}, n)

	if err := svc.DeclineInvite(context.Background(), token, "mentor-1"); err != nil {
		t.Fatalf("DeclineInvite: %v", err)
	}
	if n.adminMentorDeclinedCalls != 1 || n.mentorDeclinedCalls != 0 {
		t.Errorf("admin/mentor declined notifications = %d/%d; want 1/0", n.adminMentorDeclinedCalls, n.mentorDeclinedCalls)
	}
}

func TestProgramMemberService_Update_AdminLookupFailurePropagates(t *testing.T) {
	opErr := errors.New("membership lookup failed")
	memberRepo := &stubMemberRepo{
		findByProgramUser: func(_ context.Context, _, _ string) (*models.ProgramMember, error) {
			return nil, opErr
		},
	}
	svc := newMemberSvc(memberRepo, &stubProgRepo{}, &stubNotifier{})

	approved := models.ProgramMemberStatusApproved
	_, err := svc.Update(context.Background(), "prog-1", "member-1", models.ProgramMemberUpdateInput{Status: &approved}, "admin-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, opErr) {
		t.Fatalf("expected wrapped lookup error, got %v", err)
	}
}

func TestProgramMemberService_Delete_AdminLookupFailurePropagates(t *testing.T) {
	opErr := errors.New("membership lookup failed")
	memberRepo := &stubMemberRepo{
		findByProgramUser: func(_ context.Context, _, _ string) (*models.ProgramMember, error) {
			return nil, opErr
		},
	}
	svc := newMemberSvc(memberRepo, &stubProgRepo{}, &stubNotifier{})

	err := svc.Delete(context.Background(), "prog-1", "member-1", "admin-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, opErr) {
		t.Fatalf("expected wrapped lookup error, got %v", err)
	}
}

// ── Self-service: /me/program-memberships ────────────────────────────────────

const selfProgramID = "6f1c2b7e-4a8d-4f0e-9b3a-2d5c8e1f7a90"

// requester is the signed-in caller asking to mentor in the RequestMentorship tests.
var requester = models.UserPrincipal{UserID: "user-1", LFID: "user-1-lfid", Email: "user-1@example.org"}

const selfMemberID = "0b9e4d2a-7c61-4e35-8f1d-3a2b6c9d4e17"

func publishedProgRepo() *stubProgRepo {
	return &stubProgRepo{
		getByID: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Status: models.ProgramStatusPublished}, nil
		},
	}
}

func memberStatus(s models.ProgramMemberStatus) *models.ProgramMemberStatus { return &s }

func TestProgramMemberService_ListMine_ScopesToCaller(t *testing.T) {
	var gotUser string
	var gotFilter models.ProgramMemberFilter
	memberRepo := &stubMemberRepo{
		listByUser: func(_ context.Context, userID string, f models.ProgramMemberFilter) ([]*models.ProgramMembership, *models.PaginationMeta, error) {
			gotUser, gotFilter = userID, f
			return []*models.ProgramMembership{{ID: "m1"}}, &models.PaginationMeta{Total: 1}, nil
		},
	}
	svc := newMemberSvc(memberRepo, &stubProgRepo{}, &stubNotifier{})
	got, _, err := svc.ListMine(context.Background(), "user-1", models.ProgramMemberFilter{MemberType: string(models.MemberTypeMentor), Limit: 5})
	if err != nil {
		t.Fatalf("ListMine: %v", err)
	}
	if gotUser != "user-1" || gotFilter.MemberType != string(models.MemberTypeMentor) || gotFilter.Limit != 5 {
		t.Errorf("ListByUser(%q, %+v); want user-1 with member_type=mentor limit=5", gotUser, gotFilter)
	}
	if len(got) != 1 {
		t.Errorf("got %d memberships; want 1", len(got))
	}
}

func TestProgramMemberService_ListMine_InvalidMemberType(t *testing.T) {
	memberRepo := &stubMemberRepo{
		listByUser: func(context.Context, string, models.ProgramMemberFilter) ([]*models.ProgramMembership, *models.PaginationMeta, error) {
			t.Fatal("repo must not be called for an invalid member_type")
			return nil, nil, nil
		},
	}
	svc := newMemberSvc(memberRepo, &stubProgRepo{}, &stubNotifier{})
	_, _, err := svc.ListMine(context.Background(), "user-1", models.ProgramMemberFilter{MemberType: "mentee"})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestProgramMemberService_RequestMentorship_CreatesRequestedRowWithoutInvite(t *testing.T) {
	var captured models.ProgramMemberCreateInput
	memberRepo := &stubMemberRepo{
		create: func(_ context.Context, _ string, in models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
			captured = in
			return &models.ProgramMember{ID: in.ID, UserID: in.UserID, MemberType: in.MemberType, Status: in.Status}, nil
		},
	}
	n := &stubNotifier{}
	svc := newMemberSvc(memberRepo, publishedProgRepo(), n)
	m, err := svc.RequestMentorship(context.Background(), selfProgramID, requester)
	if err != nil {
		t.Fatalf("RequestMentorship: %v", err)
	}
	if captured.UserID != "user-1" || captured.MemberType != models.MemberTypeMentor || captured.ID == "" {
		t.Errorf("create input = %+v; want a new mentor row for user-1", captured)
	}
	if captured.Email == nil || *captured.Email != requester.Email {
		t.Errorf("email = %v; want the caller's email", captured.Email)
	}
	if m.Status == nil || *m.Status != models.ProgramMemberStatusRequested {
		t.Errorf("status = %v; want requested", m.Status)
	}
	if n.mentorInvitedCalls != 0 {
		t.Errorf("NotifyMentorInvited called %d times; want 0", n.mentorInvitedCalls)
	}
	if n.adminMentorRequestedCalls != 1 {
		t.Errorf("NotifyAdminMentorRequested called %d times; want 1", n.adminMentorRequestedCalls)
	}
}

func TestProgramMemberService_RequestMentorship_ReopensWithdrawnRow(t *testing.T) {
	createCalled := false
	var updatedID string
	var gotFrom []models.ProgramMemberStatus
	var updatedStatus models.ProgramMemberStatus
	var updatedEmail *string
	memberRepo := &stubMemberRepo{
		findByUserAndType: func(_ context.Context, programID, userID string, mt models.MemberType) (*models.ProgramMember, error) {
			if mt != models.MemberTypeMentor {
				t.Errorf("member type = %q; want mentor", mt)
			}
			return &models.ProgramMember{ID: "existing", ProgramID: programID, UserID: userID, MemberType: mt, Status: memberStatus(models.ProgramMemberStatusWithdrawn)}, nil
		},
		create: func(context.Context, string, models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
			createCalled = true
			return nil, nil
		},
		updateIfStatus: func(_ context.Context, id string, from []models.ProgramMemberStatus, in models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
			updatedID, gotFrom, updatedStatus, updatedEmail = id, from, *in.Status, in.Email
			return &models.ProgramMember{ID: id, Status: in.Status}, nil
		},
	}
	n := &stubNotifier{}
	svc := newMemberSvc(memberRepo, publishedProgRepo(), n)
	if _, err := svc.RequestMentorship(context.Background(), selfProgramID, requester); err != nil {
		t.Fatalf("RequestMentorship: %v", err)
	}
	if createCalled {
		t.Error("a withdrawn row must be reopened, not duplicated")
	}
	if updatedID != "existing" || updatedStatus != models.ProgramMemberStatusRequested {
		t.Errorf("UpdateIfStatus(%q, _, %q); want (existing, requested)", updatedID, updatedStatus)
	}
	// The reopened row records the caller's current email, as a new row would.
	if updatedEmail == nil || *updatedEmail != requester.Email {
		t.Errorf("email = %v; want the caller's email", updatedEmail)
	}
	if !slices.Equal(gotFrom, []models.ProgramMemberStatus{models.ProgramMemberStatusWithdrawn}) {
		t.Errorf("from = %v; want only withdrawn, so a concurrent change is not overwritten", gotFrom)
	}
	if n.mentorInvitedCalls != 0 {
		t.Errorf("NotifyMentorInvited called %d times; want 0", n.mentorInvitedCalls)
	}
	if n.adminMentorRequestedCalls != 1 {
		t.Errorf("NotifyAdminMentorRequested called %d times; want 1", n.adminMentorRequestedCalls)
	}
}

func TestProgramMemberService_RequestMentorship_ConflictsWithExistingRow(t *testing.T) {
	for _, status := range []models.ProgramMemberStatus{
		models.ProgramMemberStatusInvited,
		models.ProgramMemberStatusRequested,
		models.ProgramMemberStatusPending,
		models.ProgramMemberStatusApproved,
		models.ProgramMemberStatusDeclined,
	} {
		t.Run(string(status), func(t *testing.T) {
			memberRepo := &stubMemberRepo{
				findByUserAndType: func(_ context.Context, _, _ string, mt models.MemberType) (*models.ProgramMember, error) {
					return &models.ProgramMember{ID: "existing", MemberType: mt, Status: memberStatus(status)}, nil
				},
				updateIfStatus: func(context.Context, string, []models.ProgramMemberStatus, models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
					t.Fatal("an existing non-withdrawn row must not be changed")
					return nil, nil
				},
			}
			n := &stubNotifier{}
			svc := newMemberSvc(memberRepo, publishedProgRepo(), n)
			_, err := svc.RequestMentorship(context.Background(), selfProgramID, requester)
			if !errors.Is(err, domain.ErrConflict) {
				t.Errorf("expected ErrConflict, got %v", err)
			}
			if n.adminMentorRequestedCalls != 0 {
				t.Errorf("NotifyAdminMentorRequested called %d times; want 0", n.adminMentorRequestedCalls)
			}
		})
	}
}

func TestProgramMemberService_RequestMentorship_ProgramVisibility(t *testing.T) {
	for _, tc := range []struct {
		status models.ProgramStatus
		want   error
	}{
		{models.ProgramStatusPending, domain.ErrInvalidInput},
		{models.ProgramStatusRejected, domain.ErrProgramNotFound},
		{models.ProgramStatusArchived, domain.ErrProgramNotFound},
		{models.ProgramStatusHidden, domain.ErrProgramNotFound},
	} {
		t.Run(string(tc.status), func(t *testing.T) {
			progRepo := &stubProgRepo{
				getByID: func(_ context.Context, id string) (*models.Program, error) {
					return &models.Program{ID: id, Status: tc.status}, nil
				},
			}
			memberRepo := &stubMemberRepo{
				create: func(context.Context, string, models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
					t.Fatal("no row may be created for an unpublished program")
					return nil, nil
				},
			}
			svc := newMemberSvc(memberRepo, progRepo, &stubNotifier{})
			_, err := svc.RequestMentorship(context.Background(), selfProgramID, requester)
			if !errors.Is(err, tc.want) {
				t.Errorf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func TestProgramMemberService_RequestMentorship_MissingProgram(t *testing.T) {
	progRepo := &stubProgRepo{
		getByID: func(context.Context, string) (*models.Program, error) { return nil, domain.ErrProgramNotFound },
	}
	svc := newMemberSvc(&stubMemberRepo{}, progRepo, &stubNotifier{})
	_, err := svc.RequestMentorship(context.Background(), selfProgramID, requester)
	if !errors.Is(err, domain.ErrProgramNotFound) {
		t.Errorf("expected ErrProgramNotFound, got %v", err)
	}
}

func TestProgramMemberService_RequestMentorship_InvalidProgramID(t *testing.T) {
	for _, id := range []string{"", "not-a-uuid"} {
		svc := newMemberSvc(&stubMemberRepo{}, publishedProgRepo(), &stubNotifier{})
		_, err := svc.RequestMentorship(context.Background(), id, requester)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("program_id %q: expected ErrInvalidInput, got %v", id, err)
		}
	}
}

func TestProgramMemberService_RequestMentorship_RequiresCaller(t *testing.T) {
	for name, user := range map[string]models.UserPrincipal{
		"no user ID": {LFID: "user-1-lfid"},
		"no LFID":    {UserID: "user-1"},
	} {
		t.Run(name, func(t *testing.T) {
			svc := newMemberSvc(&stubMemberRepo{}, publishedProgRepo(), &stubNotifier{})
			_, err := svc.RequestMentorship(context.Background(), selfProgramID, user)
			if !errors.Is(err, domain.ErrUnauthorized) {
				t.Errorf("expected ErrUnauthorized, got %v", err)
			}
		})
	}
}

// A caller whose token carries no email leaves the member email unset, so listings use the user's address.
func TestProgramMemberService_RequestMentorship_BlankEmailStaysUnset(t *testing.T) {
	var captured models.ProgramMemberCreateInput
	memberRepo := &stubMemberRepo{
		create: func(_ context.Context, _ string, in models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
			captured = in
			return &models.ProgramMember{ID: in.ID, UserID: in.UserID, MemberType: in.MemberType, Status: in.Status}, nil
		},
	}
	svc := newMemberSvc(memberRepo, publishedProgRepo(), &stubNotifier{})
	if _, err := svc.RequestMentorship(context.Background(), selfProgramID, models.UserPrincipal{UserID: "user-1", LFID: "user-1-lfid"}); err != nil {
		t.Fatalf("RequestMentorship: %v", err)
	}
	if captured.Email != nil {
		t.Errorf("email = %q; want nil", *captured.Email)
	}
}

func TestProgramMemberService_WithdrawMine_FromRequestedOrPending(t *testing.T) {
	for _, status := range []models.ProgramMemberStatus{
		models.ProgramMemberStatusRequested,
		models.ProgramMemberStatusPending,
	} {
		t.Run(string(status), func(t *testing.T) {
			var gotFrom []models.ProgramMemberStatus
			var updatedStatus models.ProgramMemberStatus
			memberRepo := &stubMemberRepo{
				getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
					return &models.ProgramMember{ID: id, UserID: "user-1", MemberType: models.MemberTypeMentor, Status: memberStatus(status)}, nil
				},
				updateIfStatus: func(_ context.Context, id string, from []models.ProgramMemberStatus, in models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
					gotFrom, updatedStatus = from, *in.Status
					return &models.ProgramMember{ID: id, Status: in.Status}, nil
				},
			}
			n := &stubNotifier{}
			svc := newMemberSvc(memberRepo, &stubProgRepo{}, n)
			if err := svc.WithdrawMine(context.Background(), selfMemberID, "user-1"); err != nil {
				t.Fatalf("WithdrawMine: %v", err)
			}
			if n.adminMentorWithdrewCalls != 1 {
				t.Errorf("NotifyAdminMentorWithdrew called %d times; want 1", n.adminMentorWithdrewCalls)
			}
			if updatedStatus != models.ProgramMemberStatusWithdrawn {
				t.Errorf("status = %q; want withdrawn", updatedStatus)
			}
			if !slices.Equal(gotFrom, []models.ProgramMemberStatus{models.ProgramMemberStatusRequested, models.ProgramMemberStatusPending}) {
				t.Errorf("from = %v; want requested and pending only", gotFrom)
			}
		})
	}
}

func TestProgramMemberService_WithdrawMine_OtherStatusesConflict(t *testing.T) {
	for _, status := range []models.ProgramMemberStatus{
		models.ProgramMemberStatusInvited,
		models.ProgramMemberStatusApproved,
		models.ProgramMemberStatusDeclined,
		models.ProgramMemberStatusWithdrawn,
	} {
		t.Run(string(status), func(t *testing.T) {
			memberRepo := &stubMemberRepo{
				getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
					return &models.ProgramMember{ID: id, UserID: "user-1", MemberType: models.MemberTypeMentor, Status: memberStatus(status)}, nil
				},
				updateIfStatus: func(context.Context, string, []models.ProgramMemberStatus, models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
					t.Fatal("repo.UpdateIfStatus must not be called")
					return nil, nil
				},
			}
			n := &stubNotifier{}
			svc := newMemberSvc(memberRepo, &stubProgRepo{}, n)
			err := svc.WithdrawMine(context.Background(), selfMemberID, "user-1")
			if !errors.Is(err, domain.ErrInvalidStateTransition) {
				t.Errorf("expected ErrInvalidStateTransition, got %v", err)
			}
			if n.adminMentorWithdrewCalls != 0 {
				t.Errorf("NotifyAdminMentorWithdrew called %d times; want 0", n.adminMentorWithdrewCalls)
			}
		})
	}
}

// Another user's row, or the caller's own program_admin row, is a 404 so
// member IDs cannot be probed.
func TestProgramMemberService_WithdrawMine_NotCallersMentorRow(t *testing.T) {
	for name, row := range map[string]models.ProgramMember{
		"other user":    {UserID: "user-2", MemberType: models.MemberTypeMentor},
		"program admin": {UserID: "user-1", MemberType: models.MemberTypeProgramAdmin},
	} {
		t.Run(name, func(t *testing.T) {
			memberRepo := &stubMemberRepo{
				getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
					m := row
					m.ID = id
					m.Status = memberStatus(models.ProgramMemberStatusRequested)
					return &m, nil
				},
				updateIfStatus: func(context.Context, string, []models.ProgramMemberStatus, models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
					t.Fatal("repo.UpdateIfStatus must not be called")
					return nil, nil
				},
			}
			svc := newMemberSvc(memberRepo, &stubProgRepo{}, &stubNotifier{})
			err := svc.WithdrawMine(context.Background(), selfMemberID, "user-1")
			if !errors.Is(err, domain.ErrProgramMemberNotFound) {
				t.Errorf("expected ErrProgramMemberNotFound, got %v", err)
			}
		})
	}
}

func TestProgramMemberService_WithdrawMine_InvalidIDIsNotFound(t *testing.T) {
	memberRepo := &stubMemberRepo{
		getByID: func(context.Context, string) (*models.ProgramMember, error) {
			t.Fatal("repo must not be queried with a non-UUID id")
			return nil, nil
		},
	}
	svc := newMemberSvc(memberRepo, &stubProgRepo{}, &stubNotifier{})
	err := svc.WithdrawMine(context.Background(), "not-a-uuid", "user-1")
	if !errors.Is(err, domain.ErrProgramMemberNotFound) {
		t.Errorf("expected ErrProgramMemberNotFound, got %v", err)
	}
}

// ── Lifecycle and invite-email changes ───────────────────────────────────────

func TestProgramMemberService_Create_RequestedMentor_SendsNoInvite(t *testing.T) {
	memberRepo := &stubMemberRepo{
		create: func(_ context.Context, _ string, in models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
			return &models.ProgramMember{Status: in.Status}, nil
		},
	}
	n := &stubNotifier{}
	svc := newMemberSvc(memberRepo, publishedProgRepo(), n)
	_, err := svc.Create(context.Background(), "prog-1", models.ProgramMemberCreateInput{
		UserID:     "mentor-1",
		MemberType: models.MemberTypeMentor,
		Status:     memberStatus(models.ProgramMemberStatusRequested),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n.mentorInvitedCalls != 0 {
		t.Errorf("NotifyMentorInvited called %d times; want 0", n.mentorInvitedCalls)
	}
}

// Withdrawing a request is the mentor's action; an admin declines or deletes.
func TestProgramMemberService_Update_AdminCannotWithdrawRequestedOrPending(t *testing.T) {
	for _, status := range []models.ProgramMemberStatus{
		models.ProgramMemberStatusRequested,
		models.ProgramMemberStatusPending,
	} {
		t.Run(string(status), func(t *testing.T) {
			memberRepo := &stubMemberRepo{
				getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
					return &models.ProgramMember{ID: id, ProgramID: "prog-1", UserID: "u1", Status: memberStatus(status)}, nil
				},
			}
			svc := newMemberSvc(memberRepo, &stubProgRepo{}, &stubNotifier{})
			_, err := svc.Update(context.Background(), "prog-1", "member-1", models.ProgramMemberUpdateInput{Status: memberStatus(models.ProgramMemberStatusWithdrawn)}, "admin-1")
			if !errors.Is(err, domain.ErrInvalidStateTransition) {
				t.Errorf("%s→withdrawn by an admin: expected ErrInvalidStateTransition, got %v", status, err)
			}
		})
	}
}

// withdrawn → requested is reachable only through RequestMentorship.
func TestProgramMemberService_Update_WithdrawnToRequestedRefused(t *testing.T) {
	memberRepo := &stubMemberRepo{
		getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
			return &models.ProgramMember{ID: id, ProgramID: "prog-1", UserID: "u1", Status: memberStatus(models.ProgramMemberStatusWithdrawn)}, nil
		},
	}
	svc := newMemberSvc(memberRepo, &stubProgRepo{}, &stubNotifier{})
	_, err := svc.Update(context.Background(), "prog-1", "member-1", models.ProgramMemberUpdateInput{Status: memberStatus(models.ProgramMemberStatusRequested)}, "admin-1")
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Errorf("expected ErrInvalidStateTransition for withdrawn→requested, got %v", err)
	}
}

// An admin may change the row between the service's read and its write; the
// repository's conditional transition then refuses, and the 409 surfaces.
func TestProgramMemberService_SelfService_ConcurrentChangeConflicts(t *testing.T) {
	lostRace := func(context.Context, string, []models.ProgramMemberStatus, models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
		return nil, domain.ErrInvalidStateTransition
	}
	t.Run("withdraw", func(t *testing.T) {
		memberRepo := &stubMemberRepo{
			getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
				return &models.ProgramMember{ID: id, UserID: "user-1", MemberType: models.MemberTypeMentor, Status: memberStatus(models.ProgramMemberStatusRequested)}, nil
			},
			updateIfStatus: lostRace,
		}
		svc := newMemberSvc(memberRepo, &stubProgRepo{}, &stubNotifier{})
		if err := svc.WithdrawMine(context.Background(), selfMemberID, "user-1"); !errors.Is(err, domain.ErrInvalidStateTransition) {
			t.Errorf("expected ErrInvalidStateTransition, got %v", err)
		}
	})
	t.Run("reopen", func(t *testing.T) {
		memberRepo := &stubMemberRepo{
			findByUserAndType: func(_ context.Context, _, _ string, mt models.MemberType) (*models.ProgramMember, error) {
				return &models.ProgramMember{ID: "existing", MemberType: mt, Status: memberStatus(models.ProgramMemberStatusWithdrawn)}, nil
			},
			updateIfStatus: lostRace,
		}
		svc := newMemberSvc(memberRepo, publishedProgRepo(), &stubNotifier{})
		if _, err := svc.RequestMentorship(context.Background(), selfProgramID, requester); !errors.Is(err, domain.ErrInvalidStateTransition) {
			t.Errorf("expected ErrInvalidStateTransition, got %v", err)
		}
	})
}

// The admin transition is validated against the row's current status, so the
// write must carry that status; a lost race is a 409 and sends no email.
func TestProgramMemberService_Update_WritesOnlyFromValidatedStatus(t *testing.T) {
	var gotFrom []models.ProgramMemberStatus
	memberRepo := &stubMemberRepo{
		getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
			return &models.ProgramMember{ID: id, ProgramID: "prog-1", UserID: "u1", Status: memberStatus(models.ProgramMemberStatusRequested)}, nil
		},
		updateIfStatus: func(_ context.Context, id string, from []models.ProgramMemberStatus, in models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
			gotFrom = from
			return &models.ProgramMember{ID: id, Status: in.Status}, nil
		},
	}
	svc := newMemberSvc(memberRepo, &stubProgRepo{}, &stubNotifier{})
	if _, err := svc.Update(context.Background(), "prog-1", "member-1", models.ProgramMemberUpdateInput{Status: memberStatus(models.ProgramMemberStatusApproved)}, "admin-1"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !slices.Equal(gotFrom, []models.ProgramMemberStatus{models.ProgramMemberStatusRequested}) {
		t.Errorf("from = %v; want [requested]", gotFrom)
	}
}

func TestProgramMemberService_Update_LostRaceSendsNoDeclinedEmail(t *testing.T) {
	memberRepo := &stubMemberRepo{
		getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
			return &models.ProgramMember{ID: id, ProgramID: "prog-1", UserID: "u1", Status: memberStatus(models.ProgramMemberStatusRequested)}, nil
		},
		updateIfStatus: func(context.Context, string, []models.ProgramMemberStatus, models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
			return nil, domain.ErrInvalidStateTransition
		},
	}
	n := &stubNotifier{}
	svc := newMemberSvc(memberRepo, &stubProgRepo{}, n)
	_, err := svc.Update(context.Background(), "prog-1", "member-1", models.ProgramMemberUpdateInput{Status: memberStatus(models.ProgramMemberStatusDeclined)}, "admin-1")
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Errorf("expected ErrInvalidStateTransition, got %v", err)
	}
	if n.mentorDeclinedCalls != 0 {
		t.Errorf("NotifyMentorDeclined called %d times; want 0 when the write lost the race", n.mentorDeclinedCalls)
	}
}

// Accepting or declining an invite writes only while the row is still invited.
func TestProgramMemberService_InviteResponse_WritesOnlyFromInvited(t *testing.T) {
	token, err := auth.GenerateInviteToken("prog-1", "mentor-1", "test-secret")
	if err != nil {
		t.Fatalf("generate invite token: %v", err)
	}
	newRepo := func(gotFrom *[]models.ProgramMemberStatus, writeErr error) *stubMemberRepo {
		return &stubMemberRepo{
			// The invited row is found by its key, never by scanning a page of members.
			listByProgram: func(context.Context, string, models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error) {
				return nil, nil, errors.New("invite lookup must not page through members")
			},
			findByUserAndType: func(_ context.Context, programID, userID string, memberType models.MemberType) (*models.ProgramMember, error) {
				if programID != "prog-1" || userID != "mentor-1" || memberType != models.MemberTypeMentor {
					return nil, domain.ErrProgramMemberNotFound
				}
				return &models.ProgramMember{ID: "member-1", ProgramID: "prog-1", UserID: "mentor-1", Status: memberStatus(models.ProgramMemberStatusInvited)}, nil
			},
			updateIfStatus: func(_ context.Context, id string, from []models.ProgramMemberStatus, in models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
				*gotFrom = from
				if writeErr != nil {
					return nil, writeErr
				}
				return &models.ProgramMember{ID: id, Status: in.Status}, nil
			},
		}
	}
	invitedOnly := []models.ProgramMemberStatus{models.ProgramMemberStatusInvited}

	t.Run("accept", func(t *testing.T) {
		var gotFrom []models.ProgramMemberStatus
		svc := newMemberSvc(newRepo(&gotFrom, nil), &stubProgRepo{}, &stubNotifier{})
		if _, err := svc.AcceptInvite(context.Background(), token, "mentor-1"); err != nil {
			t.Fatalf("AcceptInvite: %v", err)
		}
		if !slices.Equal(gotFrom, invitedOnly) {
			t.Errorf("from = %v; want [invited]", gotFrom)
		}
	})
	t.Run("decline", func(t *testing.T) {
		var gotFrom []models.ProgramMemberStatus
		svc := newMemberSvc(newRepo(&gotFrom, nil), &stubProgRepo{}, &stubNotifier{})
		if err := svc.DeclineInvite(context.Background(), token, "mentor-1"); err != nil {
			t.Fatalf("DeclineInvite: %v", err)
		}
		if !slices.Equal(gotFrom, invitedOnly) {
			t.Errorf("from = %v; want [invited]", gotFrom)
		}
	})
	t.Run("decline lost race", func(t *testing.T) {
		var gotFrom []models.ProgramMemberStatus
		n := &stubNotifier{}
		svc := newMemberSvc(newRepo(&gotFrom, domain.ErrInvalidStateTransition), &stubProgRepo{}, n)
		if err := svc.DeclineInvite(context.Background(), token, "mentor-1"); !errors.Is(err, domain.ErrInvalidStateTransition) {
			t.Errorf("expected ErrInvalidStateTransition, got %v", err)
		}
		if n.mentorDeclinedCalls != 0 {
			t.Errorf("NotifyMentorDeclined called %d times; want 0", n.mentorDeclinedCalls)
		}
	})
}
