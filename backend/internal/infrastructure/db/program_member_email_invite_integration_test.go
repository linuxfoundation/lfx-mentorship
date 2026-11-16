// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// createEmailInvite inserts an invited mentor row with no user for address.
func createEmailInvite(t *testing.T, repo *ProgramMemberRepository, s *mentoredSeed, programID, label, address string) *models.ProgramMember {
	t.Helper()
	invited := models.ProgramMemberStatusInvited
	m, err := repo.Create(context.Background(), programID, models.ProgramMemberCreateInput{
		ID:         s.id(label),
		MemberType: models.MemberTypeMentor,
		Status:     &invited,
		Email:      &address,
	})
	if err != nil {
		t.Fatalf("create email invitation %s: %v", label, err)
	}
	return m
}

func pgErrorCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func TestEmailInvite_CreateFindAndList(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	s := newMentoredSeed(t, pool)
	repo := NewProgramMemberRepository(pool)
	program := s.program("p", "Kernel", "published")

	m := createEmailInvite(t, repo, s, program, "invite", "New.Mentor@example.org")
	if m.UserID != "" || m.Status == nil || *m.Status != models.ProgramMemberStatusInvited {
		t.Fatalf("created %+v; want an invited row with no user", m)
	}

	found, err := repo.FindEmailInvite(ctx, program, "new.mentor@EXAMPLE.org")
	if err != nil || found.ID != m.ID {
		t.Fatalf("FindEmailInvite = %v, %v; want the invitation, matched without case", found, err)
	}
	if _, err := repo.FindEmailInvite(ctx, program, "other@example.org"); !errors.Is(err, domain.ErrProgramMemberNotFound) {
		t.Errorf("unknown address err = %v; want ErrProgramMemberNotFound", err)
	}

	invited := models.ProgramMemberStatusInvited
	duplicate := "new.mentor@example.org"
	_, err = repo.Create(ctx, program, models.ProgramMemberCreateInput{ID: s.id("duplicate"), MemberType: models.MemberTypeMentor, Status: &invited, Email: &duplicate})
	if pgErrorCode(err) != "23505" {
		t.Errorf("second invitation for the same address err = %v; want a unique violation", err)
	}

	rows, meta, err := repo.ListMentorManagement(ctx, program, models.ProgramMemberFilter{})
	if err != nil || meta.Total != 1 || len(rows) != 1 {
		t.Fatalf("ListMentorManagement = %v, %v, %v; want the one email invitation", rows, meta, err)
	}
	if row := rows[0]; !row.PendingAccount || row.UserID != "" || row.Email == nil || *row.Email != "New.Mentor@example.org" {
		t.Errorf("row = %+v; want a pending-account row showing its address", row)
	}
}

func TestEmailInvite_CannotBeApprovedWithoutAUser(t *testing.T) {
	pool := integrationPool(t)
	s := newMentoredSeed(t, pool)
	repo := NewProgramMemberRepository(pool)
	program := s.program("p", "Kernel", "published")
	m := createEmailInvite(t, repo, s, program, "invite", "new.mentor@example.org")

	approved := models.ProgramMemberStatusApproved
	_, err := repo.UpdateIfStatus(context.Background(), m.ID, []models.ProgramMemberStatus{models.ProgramMemberStatusInvited}, models.ProgramMemberUpdateInput{Status: &approved})
	if pgErrorCode(err) != "23514" {
		t.Errorf("approving an email invitation err = %v; want a check violation", err)
	}
	declined := models.ProgramMemberStatusDeclined
	if _, err := repo.UpdateIfStatus(context.Background(), m.ID, []models.ProgramMemberStatus{models.ProgramMemberStatusInvited}, models.ProgramMemberUpdateInput{Status: &declined}); err != nil {
		t.Errorf("revoking an email invitation: %v", err)
	}
}

func TestEmailInvite_ClaimSetsTheUser(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	s := newMentoredSeed(t, pool)
	repo := NewProgramMemberRepository(pool)
	program := s.program("p", "Kernel", "published")
	user := s.user("new-mentor")
	invite := createEmailInvite(t, repo, s, program, "invite", "new.mentor@example.org")

	m, err := repo.ClaimEmailInvite(ctx, invite.ID, user)
	if err != nil {
		t.Fatalf("ClaimEmailInvite: %v", err)
	}
	if m.ID != invite.ID || m.UserID != user || m.Status == nil || *m.Status != models.ProgramMemberStatusApproved {
		t.Fatalf("claimed %+v; want the invitation approved for the user", m)
	}
	var markers int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM fga_outbox WHERE object_uid = $1 AND relation = 'mentor' AND username = 'new-mentor'`, program).Scan(&markers); err != nil || markers != 1 {
		t.Errorf("mentor FGA markers = %d, %v; want 1", markers, err)
	}

	if _, err := repo.ClaimEmailInvite(ctx, invite.ID, user); !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Errorf("second claim err = %v; want ErrInvalidStateTransition", err)
	}
}

func TestEmailInvite_ClaimReusesTheCallersRow(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	s := newMentoredSeed(t, pool)
	repo := NewProgramMemberRepository(pool)
	program := s.program("p", "Kernel", "published")
	user := s.user("new-mentor")
	s.member("p", "new-mentor", "mentor", "declined")
	invite := createEmailInvite(t, repo, s, program, "invite", "new.mentor@example.org")

	m, err := repo.ClaimEmailInvite(ctx, invite.ID, user)
	if err != nil {
		t.Fatalf("ClaimEmailInvite: %v", err)
	}
	if m.ID != s.id("p/new-mentor/mentor") || m.Status == nil || *m.Status != models.ProgramMemberStatusApproved {
		t.Errorf("claimed %+v; want the caller's own row approved", m)
	}
	if _, err := repo.GetByID(ctx, invite.ID); !errors.Is(err, domain.ErrProgramMemberNotFound) {
		t.Errorf("email invitation err = %v; want it deleted", err)
	}
}

func TestEmailInvite_ClaimByAnExistingMentorConflicts(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	s := newMentoredSeed(t, pool)
	repo := NewProgramMemberRepository(pool)
	program := s.program("p", "Kernel", "published")
	user := s.user("new-mentor")
	s.member("p", "new-mentor", "mentor", "approved")
	invite := createEmailInvite(t, repo, s, program, "invite", "new.mentor@example.org")

	if _, err := repo.ClaimEmailInvite(ctx, invite.ID, user); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v; want ErrConflict", err)
	}
	still, err := repo.GetByID(ctx, invite.ID)
	if err != nil || still.UserID != "" || still.Status == nil || *still.Status != models.ProgramMemberStatusInvited {
		t.Errorf("invitation = %+v, %v; want it left open", still, err)
	}
}
