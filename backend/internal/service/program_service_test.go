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

func newProgramSvc(progRepo *stubProgRepo, termRepo *stubTermRepo, appRepo *stubAppRepo) *service.ProgramService {
	return service.NewProgramService(progRepo, termRepo, appRepo)
}

func strPtr(s string) *string { return &s }

// ── state machine ────────────────────────────────────────────────────────────

func TestProgramService_Create_AlwaysDraft(t *testing.T) {
	var capturedStatus models.ProgramStatus
	repo := &stubProgRepo{
		create: func(_ context.Context, in models.ProgramCreateInput) (*models.Program, error) {
			capturedStatus = in.Status
			return &models.Program{Status: in.Status}, nil
		},
	}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	_, err := svc.Create(context.Background(), models.ProgramCreateInput{Name: "Test", Slug: "test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if capturedStatus != models.ProgramStatusDraft {
		t.Errorf("status = %q; want %q", capturedStatus, models.ProgramStatusDraft)
	}
}

func TestProgramService_Update_InvalidTransition(t *testing.T) {
	repo := &stubProgRepo{
		getByID: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Status: models.ProgramStatusDraft}, nil
		},
	}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	next := models.ProgramStatusArchived
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: &next})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Errorf("expected ErrInvalidStateTransition for draft→archived, got %v", err)
	}
}

func TestProgramService_Update_ArchivedTerminal(t *testing.T) {
	repo := &stubProgRepo{
		getByID: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Status: models.ProgramStatusArchived}, nil
		},
	}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	next := models.ProgramStatusDraft
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: &next})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Errorf("expected ErrInvalidStateTransition from archived, got %v", err)
	}
}

// ── submission guard ──────────────────────────────────────────────────────────

func fullProgram() *models.Program {
	lfid := "lf-123"
	desc := "A description"
	repo := "https://github.com/example/repo"
	logo := "https://example.com/logo.png"
	return &models.Program{
		ID:          "prog-1",
		Status:      models.ProgramStatusDraft,
		LFID:        &lfid,
		Description: &desc,
		RepoLink:    &repo,
		LogoURL:     &logo,
	}
}

func TestProgramService_Update_Submit_NoLFID(t *testing.T) {
	prog := fullProgram()
	prog.LFID = nil
	repo := &stubProgRepo{getByID: func(_ context.Context, _ string) (*models.Program, error) { return prog, nil }}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	next := models.ProgramStatusSubmitted
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: &next})
	if !errors.Is(err, domain.ErrStateLocked) {
		t.Errorf("expected ErrStateLocked for missing LFID, got %v", err)
	}
}

func TestProgramService_Update_Submit_NoDescription(t *testing.T) {
	prog := fullProgram()
	prog.Description = nil
	repo := &stubProgRepo{getByID: func(_ context.Context, _ string) (*models.Program, error) { return prog, nil }}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	next := models.ProgramStatusSubmitted
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: &next})
	if !errors.Is(err, domain.ErrStateLocked) {
		t.Errorf("expected ErrStateLocked for missing description, got %v", err)
	}
}

func TestProgramService_Update_Submit_NoSkills(t *testing.T) {
	prog := fullProgram()
	repo := &stubProgRepo{
		getByID:    func(_ context.Context, _ string) (*models.Program, error) { return prog, nil },
		listSkills: func(_ context.Context, _ string) ([]*models.ProgramSkill, error) { return nil, nil },
	}
	termRepo := &stubTermRepo{countOpenByProgram: func(_ context.Context, _ string) (int, error) { return 1, nil }}
	svc := newProgramSvc(repo, termRepo, &stubAppRepo{})
	next := models.ProgramStatusSubmitted
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: &next})
	if !errors.Is(err, domain.ErrStateLocked) {
		t.Errorf("expected ErrStateLocked for no skills, got %v", err)
	}
}

func TestProgramService_Update_Submit_NoOpenTerms(t *testing.T) {
	prog := fullProgram()
	repo := &stubProgRepo{
		getByID: func(_ context.Context, _ string) (*models.Program, error) { return prog, nil },
		listSkills: func(_ context.Context, _ string) ([]*models.ProgramSkill, error) {
			return []*models.ProgramSkill{{ID: "s1"}}, nil
		},
	}
	termRepo := &stubTermRepo{countOpenByProgram: func(_ context.Context, _ string) (int, error) { return 0, nil }}
	svc := newProgramSvc(repo, termRepo, &stubAppRepo{})
	next := models.ProgramStatusSubmitted
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: &next})
	if !errors.Is(err, domain.ErrStateLocked) {
		t.Errorf("expected ErrStateLocked for no open terms, got %v", err)
	}
}

func TestProgramService_Update_Submit_AllPresent_OK(t *testing.T) {
	prog := fullProgram()
	repo := &stubProgRepo{
		getByID: func(_ context.Context, _ string) (*models.Program, error) { return prog, nil },
		listSkills: func(_ context.Context, _ string) ([]*models.ProgramSkill, error) {
			return []*models.ProgramSkill{{ID: "s1"}}, nil
		},
	}
	termRepo := &stubTermRepo{countOpenByProgram: func(_ context.Context, _ string) (int, error) { return 1, nil }}
	svc := newProgramSvc(repo, termRepo, &stubAppRepo{})
	next := models.ProgramStatusSubmitted
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: &next})
	if err != nil {
		t.Errorf("submit with all required fields should succeed, got %v", err)
	}
}

// ── hide guard ───────────────────────────────────────────────────────────────

func TestProgramService_Update_Hide_ActiveApplications_Blocked(t *testing.T) {
	repo := &stubProgRepo{
		getByID: func(_ context.Context, _ string) (*models.Program, error) {
			return &models.Program{ID: "prog-1", Status: models.ProgramStatusPublished}, nil
		},
	}
	appRepo := &stubAppRepo{
		countBlocking: func(_ context.Context, _ string) (int, error) { return 3, nil },
	}
	svc := newProgramSvc(repo, &stubTermRepo{}, appRepo)
	next := models.ProgramStatusHidden
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: &next})
	if !errors.Is(err, domain.ErrStateLocked) {
		t.Errorf("expected ErrStateLocked when active apps exist, got %v", err)
	}
}

func TestProgramService_Update_Hide_NoApplications_OK(t *testing.T) {
	repo := &stubProgRepo{
		getByID: func(_ context.Context, _ string) (*models.Program, error) {
			return &models.Program{ID: "prog-1", Status: models.ProgramStatusPublished}, nil
		},
	}
	appRepo := &stubAppRepo{
		countBlocking: func(_ context.Context, _ string) (int, error) { return 0, nil },
	}
	svc := newProgramSvc(repo, &stubTermRepo{}, appRepo)
	next := models.ProgramStatusHidden
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: &next})
	if err != nil {
		t.Errorf("hide with no blocking apps should succeed, got %v", err)
	}
}
