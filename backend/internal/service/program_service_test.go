// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

func newProgramSvc(progRepo *stubProgRepo, termRepo *stubTermRepo, appRepo *stubAppRepo) *service.ProgramService {
	return newProgramSvcWithNotifier(progRepo, termRepo, appRepo, &stubNotifier{})
}

func newProgramSvcWithNotifier(progRepo *stubProgRepo, termRepo *stubTermRepo, appRepo *stubAppRepo, n *stubNotifier) *service.ProgramService {
	return service.NewProgramService(progRepo, termRepo, appRepo, &stubMemberRepo{}, n)
}

func newProgramSvcWithMember(progRepo *stubProgRepo, termRepo *stubTermRepo, appRepo *stubAppRepo, memberRepo *stubMemberRepo) *service.ProgramService {
	return service.NewProgramService(progRepo, termRepo, appRepo, memberRepo, &stubNotifier{})
}

// ── state machine ────────────────────────────────────────────────────────────

func TestProgramService_Create_AlwaysPending(t *testing.T) {
	var capturedStatus models.ProgramStatus
	repo := &stubProgRepo{
		create: func(_ context.Context, in models.ProgramCreateInput) (*models.Program, error) {
			capturedStatus = in.Status
			return &models.Program{ID: in.ID, Status: in.Status}, nil
		},
	}
	n := &stubNotifier{}
	svc := newProgramSvcWithNotifier(repo, &stubTermRepo{}, &stubAppRepo{}, n)
	projectUID := "00000000-0000-0000-0000-000000000001"
	projectSlug, projectName := "test-project", "Test Project"
	_, err := svc.Create(context.Background(), models.ProgramCreateInput{CreatorUserID: "creator-1", Name: "Test", Slug: "test", ProjectUID: &projectUID, ProjectSlug: &projectSlug, ProjectName: &projectName})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if capturedStatus != models.ProgramStatusPending {
		t.Errorf("status = %q; want %q", capturedStatus, models.ProgramStatusPending)
	}
	// A new program is submitted for review: its admins and the review inbox are told.
	if n.programSubmittedCalls != 1 || n.programSubmitters[0] != "creator-1" {
		t.Errorf("submission notifications = %d %v; want one from creator-1", n.programSubmittedCalls, n.programSubmitters)
	}
}

func TestProgramService_Create_RepoErrorSendsNothing(t *testing.T) {
	repo := &stubProgRepo{create: func(context.Context, models.ProgramCreateInput) (*models.Program, error) {
		return nil, errors.New("db down")
	}}
	n := &stubNotifier{}
	svc := newProgramSvcWithNotifier(repo, &stubTermRepo{}, &stubAppRepo{}, n)
	projectUID := "00000000-0000-0000-0000-000000000001"
	projectSlug, projectName := "test-project", "Test Project"
	if _, err := svc.Create(context.Background(), models.ProgramCreateInput{CreatorUserID: "creator-1", Name: "Test", Slug: "test", ProjectUID: &projectUID, ProjectSlug: &projectSlug, ProjectName: &projectName}); err == nil {
		t.Fatal("Create succeeded despite the repository error")
	}
	if n.programSubmittedCalls != 0 {
		t.Errorf("submission notifications = %d; want 0", n.programSubmittedCalls)
	}
}

func TestProgramService_Create_RequiresProjectMetadata(t *testing.T) {
	projectUID := "00000000-0000-0000-0000-000000000001"
	blank, name, slug := "  ", "Test Project", "test-project"
	for label, input := range map[string]models.ProgramCreateInput{
		"missing slug": {Name: "Test", Slug: "test", ProjectUID: &projectUID, ProjectName: &name},
		"blank slug":   {Name: "Test", Slug: "test", ProjectUID: &projectUID, ProjectSlug: &blank, ProjectName: &name},
		"missing name": {Name: "Test", Slug: "test", ProjectUID: &projectUID, ProjectSlug: &slug},
	} {
		svc := newProgramSvc(&stubProgRepo{create: func(context.Context, models.ProgramCreateInput) (*models.Program, error) {
			t.Fatalf("%s: repository must not be called", label)
			return nil, nil
		}}, &stubTermRepo{}, &stubAppRepo{})
		if _, err := svc.Create(context.Background(), input); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: expected invalid input, got %v", label, err)
		}
	}
}

func TestProgramService_CreateEnrollment_RejectsMissingTerms(t *testing.T) {
	svc := newProgramSvc(&stubProgRepo{}, &stubTermRepo{}, &stubAppRepo{})
	projectUID := "00000000-0000-0000-0000-000000000001"
	_, err := svc.CreateEnrollment(context.Background(), models.ProgramEnrollmentInput{Program: models.ProgramCreateInput{ProjectUID: &projectUID, Name: "Program", Slug: "program"}, Skills: []string{"Go"}})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestProgramService_CreateEnrollment_NormalizesSkills(t *testing.T) {
	projectUID := "00000000-0000-0000-0000-000000000001"
	start := time.Now().Add(24 * time.Hour)
	termEnd := start.Add(24 * time.Hour)
	applicationStart := time.Now()
	applicationEnd := start.Add(-time.Hour)
	var captured models.ProgramEnrollmentInput
	repo := &stubProgRepo{createEnrollment: func(_ context.Context, input models.ProgramEnrollmentInput) (*models.Program, error) {
		captured = input
		return &models.Program{ID: "p1"}, nil
	}}
	n := &stubNotifier{}
	svc := newProgramSvcWithNotifier(repo, &stubTermRepo{}, &stubAppRepo{}, n)
	projectSlug, projectName, projectLogo := " program-project ", "Program Project", " "
	_, err := svc.CreateEnrollment(context.Background(), models.ProgramEnrollmentInput{
		Program: models.ProgramCreateInput{CreatorUserID: "creator-1", ProjectUID: &projectUID, ProjectSlug: &projectSlug, ProjectName: &projectName, ProjectLogoURL: &projectLogo, Name: "Program", Slug: "program"},
		Skills:  []string{" Go ", "go", "Kubernetes"},
		Terms:   []models.ProgramTermCreateInput{{Name: "Term", StartDateTime: &start, EndDateTime: &termEnd, ApplicationStartDate: &applicationStart, ApplicationEndDate: &applicationEnd}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(captured.Skills) != 2 || captured.Skills[0] != "Go" || captured.Skills[1] != "Kubernetes" {
		t.Fatalf("skills=%v", captured.Skills)
	}
	if captured.Program.ProjectSlug == nil || *captured.Program.ProjectSlug != "program-project" || captured.Program.ProjectName == nil || *captured.Program.ProjectName != "Program Project" || captured.Program.ProjectLogoURL != nil {
		t.Fatalf("project metadata=%v %v %v", captured.Program.ProjectSlug, captured.Program.ProjectName, captured.Program.ProjectLogoURL)
	}
	if n.programSubmittedCalls != 1 || n.programSubmitters[0] != "creator-1" {
		t.Fatalf("submission notifications = %d %v; want one from creator-1", n.programSubmittedCalls, n.programSubmitters)
	}
}

func TestProgramService_CreateEnrollment_RejectsNonISOPrerequisiteDueDate(t *testing.T) {
	projectUID := "00000000-0000-0000-0000-000000000001"
	start := time.Now().Add(24 * time.Hour)
	termEnd := start.Add(24 * time.Hour)
	applicationStart := time.Now()
	applicationEnd := start.Add(-time.Hour)
	svc := newProgramSvc(&stubProgRepo{createEnrollment: func(context.Context, models.ProgramEnrollmentInput) (*models.Program, error) {
		t.Fatal("repository must not be called")
		return nil, nil
	}}, &stubTermRepo{}, &stubAppRepo{})
	_, err := svc.CreateEnrollment(context.Background(), models.ProgramEnrollmentInput{
		Program:       models.ProgramCreateInput{ProjectUID: &projectUID, Name: "Program", Slug: "program"},
		Skills:        []string{"Go"},
		Terms:         []models.ProgramTermCreateInput{{Name: "Term", StartDateTime: &start, EndDateTime: &termEnd, ApplicationStartDate: &applicationStart, ApplicationEndDate: &applicationEnd}},
		Prerequisites: json.RawMessage(`[{"name":"PR","description":"Open a PR","required":true,"dueDate":"tomorrow"}]`),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid due date input, got %v", err)
	}
}

func TestProgramService_CreateEnrollment_RejectsUnsafeURL(t *testing.T) {
	projectUID := "00000000-0000-0000-0000-000000000001"
	unsafe := "javascript:alert(1)"
	svc := newProgramSvc(&stubProgRepo{}, &stubTermRepo{}, &stubAppRepo{})
	_, err := svc.CreateEnrollment(context.Background(), models.ProgramEnrollmentInput{Program: models.ProgramCreateInput{ProjectUID: &projectUID, Name: "Program", Slug: "program", WebsiteURL: &unsafe}, Skills: []string{"Go"}, Terms: []models.ProgramTermCreateInput{{Name: "Term"}}})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid URL input, got %v", err)
	}
}

// ── project metadata from Project Service ────────────────────────────────────

type stubProjectLookup struct {
	project *models.ProjectMetadata
	err     error
	gotUID  string
}

func (s *stubProjectLookup) GetProject(_ context.Context, uid string) (*models.ProjectMetadata, error) {
	s.gotUID = uid
	return s.project, s.err
}

// enrollmentWithProjectMetadata is a valid enrollment whose project slug and
// name belong to a different project than projectUID, and whose logo is unsafe,
// so a test passes only if Project Service's values replace all three.
func enrollmentWithProjectMetadata(projectUID string) models.ProgramEnrollmentInput {
	start := time.Now().Add(24 * time.Hour)
	termEnd := start.Add(24 * time.Hour)
	applicationStart := time.Now()
	applicationEnd := start.Add(-time.Hour)
	slug, name, logo := "other-project", "Other Project", "javascript:alert(1)"
	return models.ProgramEnrollmentInput{
		Program: models.ProgramCreateInput{ProjectUID: &projectUID, ProjectSlug: &slug, ProjectName: &name, ProjectLogoURL: &logo, Name: "Program", Slug: "program"},
		Skills:  []string{"Go"},
		Terms:   []models.ProgramTermCreateInput{{Name: "Term", StartDateTime: &start, EndDateTime: &termEnd, ApplicationStartDate: &applicationStart, ApplicationEndDate: &applicationEnd}},
	}
}

func TestProgramService_CreateEnrollment_StoresProjectServiceMetadata(t *testing.T) {
	var captured models.ProgramEnrollmentInput
	repo := &stubProgRepo{createEnrollment: func(_ context.Context, input models.ProgramEnrollmentInput) (*models.Program, error) {
		captured = input
		return &models.Program{ID: "p1"}, nil
	}}
	lookup := &stubProjectLookup{project: &models.ProjectMetadata{Slug: "cncf", Name: "Cloud Native Computing Foundation"}}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	svc.SetProjectLookup(lookup)

	if _, err := svc.CreateEnrollment(context.Background(), enrollmentWithProjectMetadata("7CAD5A8D-19D0-41A4-81A6-043453DAF9EE")); err != nil {
		t.Fatal(err)
	}
	if lookup.gotUID != "7cad5a8d-19d0-41a4-81a6-043453daf9ee" {
		t.Errorf("lookup uid = %q; want the canonical UUID", lookup.gotUID)
	}
	program := captured.Program
	if program.ProjectSlug == nil || *program.ProjectSlug != "cncf" || program.ProjectName == nil || *program.ProjectName != "Cloud Native Computing Foundation" || program.ProjectLogoURL != nil {
		t.Fatalf("project metadata = %v %v %v; want Project Service's, with no logo", program.ProjectSlug, program.ProjectName, program.ProjectLogoURL)
	}
}

func TestProgramService_CreateEnrollment_ProjectLookupErrors(t *testing.T) {
	for label, tc := range map[string]struct {
		err  error
		want error
	}{
		"unknown project":     {err: domain.ErrProjectNotFound, want: domain.ErrInvalidInput},
		"service unavailable": {err: domain.ErrUpstreamUnavailable, want: domain.ErrUpstreamUnavailable},
	} {
		t.Run(label, func(t *testing.T) {
			svc := newProgramSvc(&stubProgRepo{createEnrollment: func(context.Context, models.ProgramEnrollmentInput) (*models.Program, error) {
				t.Fatal("repository must not be called")
				return nil, nil
			}}, &stubTermRepo{}, &stubAppRepo{})
			svc.SetProjectLookup(&stubProjectLookup{err: tc.err})

			_, err := svc.CreateEnrollment(context.Background(), enrollmentWithProjectMetadata("00000000-0000-0000-0000-000000000001"))
			if !errors.Is(err, tc.want) || errors.Is(err, domain.ErrProjectNotFound) {
				t.Fatalf("err = %v; want %v", err, tc.want)
			}
		})
	}
}

func TestProgramService_Create_StoresProjectServiceMetadata(t *testing.T) {
	var captured models.ProgramCreateInput
	repo := &stubProgRepo{create: func(_ context.Context, in models.ProgramCreateInput) (*models.Program, error) {
		captured = in
		return &models.Program{}, nil
	}}
	logo := "https://example.org/cncf.png"
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	svc.SetProjectLookup(&stubProjectLookup{project: &models.ProjectMetadata{Slug: "cncf", Name: "CNCF", LogoURL: &logo}})

	if _, err := svc.Create(context.Background(), enrollmentWithProjectMetadata("00000000-0000-0000-0000-000000000001").Program); err != nil {
		t.Fatal(err)
	}
	if *captured.ProjectSlug != "cncf" || *captured.ProjectName != "CNCF" || captured.ProjectLogoURL == nil || *captured.ProjectLogoURL != logo {
		t.Fatalf("project metadata = %v %v %v; want Project Service's", *captured.ProjectSlug, *captured.ProjectName, captured.ProjectLogoURL)
	}
}

func TestProgramService_Update_InvalidTransition(t *testing.T) {
	repo := &stubProgRepo{
		getByID: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Status: models.ProgramStatusPending}, nil
		},
	}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	next := models.ProgramStatusArchived
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: &next})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Errorf("expected ErrInvalidStateTransition for pending→archived, got %v", err)
	}
}

func TestProgramService_Decide(t *testing.T) {
	for _, tc := range []struct {
		current, next models.ProgramStatus
		want          error
	}{
		{models.ProgramStatusPending, models.ProgramStatusPublished, nil},
		{models.ProgramStatusPending, models.ProgramStatusRejected, nil},
		{models.ProgramStatusHidden, models.ProgramStatusPublished, domain.ErrInvalidStateTransition},
		{models.ProgramStatusRejected, models.ProgramStatusPublished, domain.ErrInvalidStateTransition},
		{models.ProgramStatusPending, models.ProgramStatusHidden, domain.ErrInvalidInput},
	} {
		repo := &stubProgRepo{
			getByID: func(_ context.Context, _ string) (*models.Program, error) {
				prog := fullProgram()
				prog.Status = tc.current
				return prog, nil
			},
			listSkills: func(context.Context, string) ([]*models.ProgramSkill, error) {
				return []*models.ProgramSkill{{ID: "s1"}}, nil
			},
		}
		termRepo := &stubTermRepo{countOpenByProgram: func(context.Context, string) (int, error) { return 1, nil }}
		n := &stubNotifier{}
		svc := newProgramSvcWithNotifier(repo, termRepo, &stubAppRepo{}, n)
		_, err := svc.Decide(context.Background(), "prog-1", tc.next)
		if tc.want == nil && err != nil || tc.want != nil && !errors.Is(err, tc.want) {
			t.Errorf("%s→%s: got %v; want %v", tc.current, tc.next, err, tc.want)
		}
		var wantDecisions []models.ProgramStatus
		if tc.want == nil {
			wantDecisions = []models.ProgramStatus{tc.next}
		}
		if !reflect.DeepEqual(n.programDecisions, wantDecisions) {
			t.Errorf("%s→%s: decision notifications = %v; want %v", tc.current, tc.next, n.programDecisions, wantDecisions)
		}
	}
}

// Decide publishes only a complete program, but rejects any pending one.
func TestProgramService_Decide_IncompleteProgram(t *testing.T) {
	for _, tc := range []struct {
		next models.ProgramStatus
		want error
	}{
		{models.ProgramStatusPublished, domain.ErrStateLocked},
		{models.ProgramStatusRejected, nil},
	} {
		repo := &stubProgRepo{
			getByID: func(_ context.Context, _ string) (*models.Program, error) {
				prog := fullProgram()
				prog.LogoURL = nil
				return prog, nil
			},
		}
		n := &stubNotifier{}
		svc := newProgramSvcWithNotifier(repo, &stubTermRepo{}, &stubAppRepo{}, n)
		_, err := svc.Decide(context.Background(), "prog-1", tc.next)
		if tc.want == nil && err != nil || tc.want != nil && !errors.Is(err, tc.want) {
			t.Errorf("incomplete program → %s: got %v; want %v", tc.next, err, tc.want)
		}
		var wantDecisions []models.ProgramStatus
		if tc.want == nil {
			wantDecisions = []models.ProgramStatus{tc.next}
		}
		if !reflect.DeepEqual(n.programDecisions, wantDecisions) {
			t.Errorf("incomplete program → %s: decision notifications = %v; want %v", tc.next, n.programDecisions, wantDecisions)
		}
	}
}

func TestProgramService_HideUnhide(t *testing.T) {
	for _, tc := range []struct {
		name     string
		unhide   bool
		current  models.ProgramStatus
		blocking int
		want     error
		stored   models.ProgramStatus
	}{
		{name: "hide published", current: models.ProgramStatusPublished, stored: models.ProgramStatusHidden},
		{name: "hide blocked by active applications", current: models.ProgramStatusPublished, blocking: 2, want: domain.ErrStateLocked},
		{name: "hide hidden", current: models.ProgramStatusHidden, want: domain.ErrInvalidStateTransition},
		{name: "hide pending", current: models.ProgramStatusPending, want: domain.ErrInvalidStateTransition},
		{name: "hide archived", current: models.ProgramStatusArchived, want: domain.ErrInvalidStateTransition},
		{name: "unhide hidden", unhide: true, current: models.ProgramStatusHidden, stored: models.ProgramStatusPublished},
		{name: "unhide archived", unhide: true, current: models.ProgramStatusArchived, want: domain.ErrInvalidStateTransition},
		// programTransitions allows pending → published; only Decide may apply it.
		{name: "unhide pending", unhide: true, current: models.ProgramStatusPending, want: domain.ErrInvalidStateTransition},
		{name: "unhide published", unhide: true, current: models.ProgramStatusPublished, want: domain.ErrInvalidStateTransition},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stored, expected *models.ProgramStatus
			reads := 0
			repo := &stubProgRepo{
				getByID: func(_ context.Context, id string) (*models.Program, error) {
					reads++
					return &models.Program{ID: id, Status: tc.current}, nil
				},
				update: func(_ context.Context, id string, in models.ProgramUpdateInput) (*models.Program, error) {
					stored, expected = in.Status, in.ExpectedStatus
					return &models.Program{ID: id, Status: *in.Status}, nil
				},
			}
			appRepo := &stubAppRepo{countBlocking: func(context.Context, string) (int, error) { return tc.blocking, nil }}
			n := &stubNotifier{}
			svc := newProgramSvcWithNotifier(repo, &stubTermRepo{}, appRepo, n)
			change := svc.Hide
			if tc.unhide {
				change = svc.Unhide
			}
			program, err := change(context.Background(), "prog-1")
			// Unhiding republishes the program but is not an approval decision.
			if n.programDecisions != nil || n.programSubmittedCalls != 0 {
				t.Fatalf("notified %v / %d submissions; want nothing", n.programDecisions, n.programSubmittedCalls)
			}
			if reads != 1 {
				t.Fatalf("program read %d times; want once", reads)
			}
			if tc.want != nil {
				if !errors.Is(err, tc.want) {
					t.Fatalf("got %v; want %v", err, tc.want)
				}
				if stored != nil {
					t.Fatalf("status %q was stored despite the error", *stored)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if stored == nil || *stored != tc.stored || program.Status != tc.stored {
				t.Fatalf("stored %v, returned %q; want %q", stored, program.Status, tc.stored)
			}
			// The repository must apply the change only while the row is still in the status that was validated.
			if expected == nil || *expected != tc.current {
				t.Fatalf("ExpectedStatus = %v; want %q", expected, tc.current)
			}
		})
	}
}

func TestProgramService_HideUnhide_NotFound(t *testing.T) {
	repo := &stubProgRepo{getByID: func(context.Context, string) (*models.Program, error) {
		return nil, domain.ErrProgramNotFound
	}}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	if _, err := svc.Hide(context.Background(), "missing"); !errors.Is(err, domain.ErrProgramNotFound) {
		t.Errorf("Hide: got %v; want ErrProgramNotFound", err)
	}
	if _, err := svc.Unhide(context.Background(), "missing"); !errors.Is(err, domain.ErrProgramNotFound) {
		t.Errorf("Unhide: got %v; want ErrProgramNotFound", err)
	}
}

func TestProgramService_Update_ArchivedTerminal(t *testing.T) {
	repo := &stubProgRepo{
		getByID: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Status: models.ProgramStatusArchived}, nil
		},
	}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	next := models.ProgramStatusPending
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: &next})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Errorf("expected ErrInvalidStateTransition from archived, got %v", err)
	}
}

// ── publication guard ──────────────────────────────────────────────────────────

func fullProgram() *models.Program {
	lfid := "lf-123"
	projectUID := "project-1"
	desc := "A description"
	repo := "https://github.com/example/repo"
	logo := "https://example.com/logo.png"
	return &models.Program{
		ID:          "prog-1",
		Status:      models.ProgramStatusPending,
		ProjectUID:  &projectUID,
		LFID:        &lfid,
		Description: &desc,
		RepoLink:    &repo,
		LogoURL:     &logo,
	}
}

func TestProgramService_Update_Publish_NoLFID(t *testing.T) {
	prog := fullProgram()
	prog.LFID = nil
	repo := &stubProgRepo{getByID: func(_ context.Context, _ string) (*models.Program, error) { return prog, nil }, listSkills: func(context.Context, string) ([]*models.ProgramSkill, error) {
		return []*models.ProgramSkill{{ID: "s1"}}, nil
	}}
	svc := newProgramSvc(repo, &stubTermRepo{countOpenByProgram: func(context.Context, string) (int, error) { return 1, nil }}, &stubAppRepo{})
	next := models.ProgramStatusPublished
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: &next})
	if err != nil {
		t.Errorf("expected publication to ignore legacy LFID, got %v", err)
	}
}

func TestProgramService_Update_Publish_NoProjectUID(t *testing.T) {
	prog := fullProgram()
	prog.ProjectUID = nil
	repo := &stubProgRepo{getByID: func(_ context.Context, _ string) (*models.Program, error) { return prog, nil }}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	next := models.ProgramStatusPublished
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: &next})
	if !errors.Is(err, domain.ErrStateLocked) {
		t.Errorf("expected ErrStateLocked for missing project_uid, got %v", err)
	}
}

func TestProgramService_Update_Publish_NoDescription(t *testing.T) {
	prog := fullProgram()
	prog.Description = nil
	repo := &stubProgRepo{getByID: func(_ context.Context, _ string) (*models.Program, error) { return prog, nil }}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	next := models.ProgramStatusPublished
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: &next})
	if !errors.Is(err, domain.ErrStateLocked) {
		t.Errorf("expected ErrStateLocked for missing description, got %v", err)
	}
}

func TestProgramService_Update_Publish_NoSkills(t *testing.T) {
	prog := fullProgram()
	repo := &stubProgRepo{
		getByID:    func(_ context.Context, _ string) (*models.Program, error) { return prog, nil },
		listSkills: func(_ context.Context, _ string) ([]*models.ProgramSkill, error) { return nil, nil },
	}
	termRepo := &stubTermRepo{countOpenByProgram: func(_ context.Context, _ string) (int, error) { return 1, nil }}
	svc := newProgramSvc(repo, termRepo, &stubAppRepo{})
	next := models.ProgramStatusPublished
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: &next})
	if !errors.Is(err, domain.ErrStateLocked) {
		t.Errorf("expected ErrStateLocked for no skills, got %v", err)
	}
}

func TestProgramService_Update_Publish_NoOpenTerms(t *testing.T) {
	prog := fullProgram()
	repo := &stubProgRepo{
		getByID: func(_ context.Context, _ string) (*models.Program, error) { return prog, nil },
		listSkills: func(_ context.Context, _ string) ([]*models.ProgramSkill, error) {
			return []*models.ProgramSkill{{ID: "s1"}}, nil
		},
	}
	termRepo := &stubTermRepo{countOpenByProgram: func(_ context.Context, _ string) (int, error) { return 0, nil }}
	svc := newProgramSvc(repo, termRepo, &stubAppRepo{})
	next := models.ProgramStatusPublished
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: &next})
	if !errors.Is(err, domain.ErrStateLocked) {
		t.Errorf("expected ErrStateLocked for no open terms, got %v", err)
	}
}

func TestProgramService_Update_Publish_AllPresent_OK(t *testing.T) {
	prog := fullProgram()
	repo := &stubProgRepo{
		getByID: func(_ context.Context, _ string) (*models.Program, error) { return prog, nil },
		listSkills: func(_ context.Context, _ string) ([]*models.ProgramSkill, error) {
			return []*models.ProgramSkill{{ID: "s1"}}, nil
		},
	}
	termRepo := &stubTermRepo{countOpenByProgram: func(_ context.Context, _ string) (int, error) { return 1, nil }}
	n := &stubNotifier{}
	svc := newProgramSvcWithNotifier(repo, termRepo, &stubAppRepo{}, n)
	next := models.ProgramStatusPublished
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: &next})
	if err != nil {
		t.Errorf("publish with all required fields should succeed, got %v", err)
	}
	// Only Decide sends the approval email; a status change through Update sends nothing.
	if n.programSubmittedCalls != 0 || n.programDecisions != nil {
		t.Errorf("notified %d submissions / %v decisions; want nothing", n.programSubmittedCalls, n.programDecisions)
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

func TestProgramService_Update_ConfirmsEditToEditor(t *testing.T) {
	name := "Renamed"
	failed := errors.New("db down")
	for label, tc := range map[string]struct {
		input   models.ProgramUpdateInput
		repoErr error
		want    []string
	}{
		"edit":                    {input: models.ProgramUpdateInput{Name: &name, EditorID: "admin-1"}, want: []string{"admin-1"}},
		"service status change":   {input: models.ProgramUpdateInput{Name: &name}},
		"edit that fails to save": {input: models.ProgramUpdateInput{Name: &name, EditorID: "admin-1"}, repoErr: failed},
	} {
		n := &stubNotifier{}
		repo := &stubProgRepo{update: func(_ context.Context, id string, _ models.ProgramUpdateInput) (*models.Program, error) {
			if tc.repoErr != nil {
				return nil, tc.repoErr
			}
			return &models.Program{ID: id}, nil
		}}
		_, err := newProgramSvcWithNotifier(repo, &stubTermRepo{}, &stubAppRepo{}, n).Update(context.Background(), "prog-1", tc.input)
		if !errors.Is(err, tc.repoErr) || !slices.Equal(n.programEditors, tc.want) {
			t.Errorf("%s: err = %v, confirmations to %v; want %v", label, err, n.programEditors, tc.want)
		}
	}
}

func TestProgramService_ListCatalog_FillsDiscoveryLabels(t *testing.T) {
	start := time.Now().Add(-24 * time.Hour)
	end := time.Now().Add(24 * time.Hour)
	repo := &stubProgRepo{
		listCatalog: func(_ context.Context, f models.ProgramFilter) ([]*models.ProgramCatalogItem, *models.PaginationMeta, error) {
			if f.Skill != "" {
				t.Errorf("skill %q should have been cleared for value %q", f.Skill, "all")
			}
			return []*models.ProgramCatalogItem{{
				Program: models.Program{ID: "p1", Name: "K8s"},
				Terms: []models.ProgramCatalogTerm{{
					ProgramTerm: models.ProgramTerm{
						Status:               "open",
						ApplicationStartDate: &start,
						ApplicationEndDate:   &end,
					},
				}},
			}}, &models.PaginationMeta{Total: 1, Limit: 20, Offset: 0}, nil
		},
	}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	items, meta, err := svc.ListCatalog(context.Background(), models.ProgramFilter{Skill: "all"})
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	if meta.Total != 1 {
		t.Errorf("meta.total = %d; want 1", meta.Total)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d; want 1", len(items))
	}
	if items[0].Terms[0].DiscoveryLabel != "Apply Now" {
		t.Errorf("discovery_label = %q; want Apply Now", items[0].Terms[0].DiscoveryLabel)
	}
	if items[0].Skills == nil || items[0].Mentors == nil {
		t.Errorf("nested slices must be non-nil empty, skills=%v mentors=%v", items[0].Skills, items[0].Mentors)
	}
}

func TestProgramService_ListCatalog_PassesSkillFilter(t *testing.T) {
	var captured models.ProgramFilter
	repo := &stubProgRepo{
		listCatalog: func(_ context.Context, f models.ProgramFilter) ([]*models.ProgramCatalogItem, *models.PaginationMeta, error) {
			captured = f
			return []*models.ProgramCatalogItem{}, &models.PaginationMeta{}, nil
		},
	}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	if _, _, err := svc.ListCatalog(context.Background(), models.ProgramFilter{
		Status:          string(models.ProgramStatusPending),
		Skill:           "  Go  ",
		DiscoveryStatus: "acceptance",
		SortBy:          "name_desc",
	}); err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	if captured.Skill != "Go" {
		t.Errorf("skill = %q; want Go", captured.Skill)
	}
	if captured.Status != string(models.ProgramStatusPublished) {
		t.Errorf("status = %q; want published", captured.Status)
	}
	if captured.DiscoveryStatus != "acceptance" {
		t.Errorf("discovery status = %q; want acceptance", captured.DiscoveryStatus)
	}
	if captured.SortBy != "name_desc" {
		t.Errorf("sortBy = %q; want name_desc", captured.SortBy)
	}
}

func TestProgramService_GetCatalog_NotFound(t *testing.T) {
	repo := &stubProgRepo{
		getCatalog: func(_ context.Context, _ string) (*models.ProgramCatalogItem, error) {
			return nil, domain.ErrProgramNotFound
		},
	}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	_, err := svc.GetCatalog(context.Background(), "missing")
	if !errors.Is(err, domain.ErrProgramNotFound) {
		t.Errorf("expected ErrProgramNotFound, got %v", err)
	}
}

func TestProgramService_GetCatalog_PendingReturnsOK(t *testing.T) {
	repo := &stubProgRepo{
		getCatalog: func(_ context.Context, id string) (*models.ProgramCatalogItem, error) {
			return &models.ProgramCatalogItem{Program: models.Program{ID: id, Status: models.ProgramStatusPending}}, nil
		},
	}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	item, err := svc.GetCatalog(context.Background(), "p1")
	if err != nil {
		t.Fatalf("expected nil error for pending, got %v", err)
	}
	if item.ID != "p1" {
		t.Errorf("id = %q; want p1", item.ID)
	}
}

func TestProgramService_GetCatalog_Unpublished(t *testing.T) {
	for _, status := range []models.ProgramStatus{
		models.ProgramStatusRejected,
		models.ProgramStatusHidden,
		models.ProgramStatusArchived,
	} {
		t.Run(string(status), func(t *testing.T) {
			repo := &stubProgRepo{
				getCatalog: func(_ context.Context, id string) (*models.ProgramCatalogItem, error) {
					return &models.ProgramCatalogItem{Program: models.Program{ID: id, Status: status}}, nil
				},
			}
			svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
			_, err := svc.GetCatalog(context.Background(), "p1")
			if !errors.Is(err, domain.ErrProgramNotFound) {
				t.Errorf("status %s: expected ErrProgramNotFound, got %v", status, err)
			}
		})
	}
}

func TestProgramService_ListCatalogMentees(t *testing.T) {
	var captured string
	repo := &stubProgRepo{
		listMentees: func(_ context.Context, id string) ([]*models.ProgramCatalogMentee, error) {
			captured = id
			return []*models.ProgramCatalogMentee{{UserID: "u1", Status: models.ApplicationStatusAccepted, TermID: "t1", TermName: "Spring 2026"}}, nil
		},
	}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	mentees, err := svc.ListCatalogMentees(context.Background(), "prog-1")
	if err != nil {
		t.Fatalf("ListCatalogMentees: %v", err)
	}
	if captured != "prog-1" || len(mentees) != 1 || mentees[0].UserID != "u1" {
		t.Errorf("mentees = %+v captured = %q", mentees, captured)
	}
}

func TestProgramService_Update_InvalidProgramTermStatus_Rejected(t *testing.T) {
	svc := newProgramSvc(&stubProgRepo{}, &stubTermRepo{}, &stubAppRepo{})
	bad := models.ProgramTermStatus("ajar") // not a member of the enum
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{ProgramTermStatus: &bad})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for unknown program_term_status, got %v", err)
	}
}

func TestProgramService_Update_NormalizesSkills(t *testing.T) {
	var captured models.ProgramUpdateInput
	repo := &stubProgRepo{update: func(_ context.Context, _ string, input models.ProgramUpdateInput) (*models.Program, error) {
		captured = input
		return &models.Program{ID: "prog-1"}, nil
	}}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	if _, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Skills: []string{" Go ", "go", "", "Kubernetes"}}); err != nil {
		t.Fatal(err)
	}
	if len(captured.Skills) != 2 || captured.Skills[0] != "Go" || captured.Skills[1] != "Kubernetes" {
		t.Fatalf("skills=%v", captured.Skills)
	}
}

func TestProgramService_Update_RejectsEmptySkills(t *testing.T) {
	repo := &stubProgRepo{update: func(context.Context, string, models.ProgramUpdateInput) (*models.Program, error) {
		t.Fatal("repository must not be called for an empty skill set")
		return nil, nil
	}}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Skills: []string{" ", ""}})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestProgramService_Update_OmittedSkillsLeftUnchanged(t *testing.T) {
	var captured models.ProgramUpdateInput
	repo := &stubProgRepo{update: func(_ context.Context, _ string, input models.ProgramUpdateInput) (*models.Program, error) {
		captured = input
		return &models.Program{ID: "prog-1"}, nil
	}}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	name := "Renamed"
	if _, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if captured.Skills != nil {
		t.Fatalf("skills=%v; want nil", captured.Skills)
	}
}

func TestProgramService_Update_NormalizesTaskTemplates(t *testing.T) {
	var captured models.ProgramUpdateInput
	repo := &stubProgRepo{
		getByID: func(context.Context, string) (*models.Program, error) {
			return &models.Program{ID: "prog-1", TaskTemplates: json.RawMessage(`[{"name":"Resume","custom":false},{"name":"Essay","custom":true}]`)}, nil
		},
		update: func(_ context.Context, _ string, input models.ProgramUpdateInput) (*models.Program, error) {
			captured = input
			return &models.Program{ID: "prog-1"}, nil
		},
	}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	templates := json.RawMessage(`[
		{"name":" resume ","description":" Upload it ","submitFile":"required","dueDate":"2026-05-01"},
		{"name":"Essay","description":"","submitFile":"","dueDate":""},
		{"name":"New task","description":"Do it","submitFile":null,"dueDate":null,"custom":true}
	]`)
	if _, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{TaskTemplates: templates}); err != nil {
		t.Fatal(err)
	}
	type template struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
		SubmitFile  *string `json:"submitFile"`
		DueDate     *string `json:"dueDate"`
		Custom      bool    `json:"custom"`
	}
	var got []template
	if err := json.Unmarshal(captured.TaskTemplates, &got); err != nil {
		t.Fatal(err)
	}
	description, required, dueDate, doIt := "Upload it", "required", "2026-05-01", "Do it"
	want := []template{
		{Name: "resume", Description: &description, SubmitFile: &required, DueDate: &dueDate, Custom: false},
		{Name: "Essay", Custom: true},
		{Name: "New task", Description: &doIt, Custom: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("task templates = %s; want blank values cleared and custom kept by name", captured.TaskTemplates)
	}
}

func TestProgramService_Update_KeepsLegacyStringCustom(t *testing.T) {
	var captured models.ProgramUpdateInput
	repo := &stubProgRepo{
		getByID: func(context.Context, string) (*models.Program, error) {
			// Imported before the migration normalized templates, so custom is a string.
			return &models.Program{ID: "prog-1", TaskTemplates: json.RawMessage(`[{"name":"Essay","custom":"true"},{"name":"Resume","custom":"false"}]`)}, nil
		},
		update: func(_ context.Context, _ string, input models.ProgramUpdateInput) (*models.Program, error) {
			captured = input
			return &models.Program{ID: "prog-1"}, nil
		},
	}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	if _, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{TaskTemplates: json.RawMessage(`[{"name":"Essay"},{"name":"Resume"}]`)}); err != nil {
		t.Fatal(err)
	}
	want := `[{"name":"Essay","description":null,"submitFile":null,"dueDate":null,"custom":true},{"name":"Resume","description":null,"submitFile":null,"dueDate":null,"custom":false}]`
	if string(captured.TaskTemplates) != want {
		t.Fatalf("task templates = %s; want %s", captured.TaskTemplates, want)
	}
}

func TestProgramService_Update_EmptyTaskTemplatesClear(t *testing.T) {
	var captured models.ProgramUpdateInput
	repo := &stubProgRepo{
		getByID: func(context.Context, string) (*models.Program, error) {
			return &models.Program{ID: "prog-1", TaskTemplates: json.RawMessage(`[{"name":"Resume"}]`)}, nil
		},
		update: func(_ context.Context, _ string, input models.ProgramUpdateInput) (*models.Program, error) {
			captured = input
			return &models.Program{ID: "prog-1"}, nil
		},
	}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	if _, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{TaskTemplates: json.RawMessage(`[]`)}); err != nil {
		t.Fatal(err)
	}
	if string(captured.TaskTemplates) != "[]" {
		t.Fatalf("task templates = %s; want []", captured.TaskTemplates)
	}
}

func TestProgramService_Update_RejectsInvalidTaskTemplates(t *testing.T) {
	cases := map[string]string{
		"not an array":   `{"name":"Resume"}`,
		"null":           `null`,
		"blank name":     `[{"name":" "}]`,
		"unknown submit": `[{"name":"Resume","submitFile":"yes"}]`,
		"bad due date":   `[{"name":"Resume","dueDate":"05/01/2026"}]`,
	}
	for name, templates := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &stubProgRepo{
				getByID: func(context.Context, string) (*models.Program, error) { return &models.Program{ID: "prog-1"}, nil },
				update: func(context.Context, string, models.ProgramUpdateInput) (*models.Program, error) {
					t.Fatal("repository must not be called for invalid task templates")
					return nil, nil
				},
			}
			svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
			_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{TaskTemplates: json.RawMessage(templates)})
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("expected ErrInvalidInput, got %v", err)
			}
		})
	}
}

// openTermEntry returns an open-term entry with a valid application window and term dates.
func openTermEntry(id, name string) models.ProgramOpenTermInput {
	appStart := time.Now().Add(24 * time.Hour)
	appEnd, start, end := appStart.Add(24*time.Hour), appStart.Add(48*time.Hour), appStart.Add(30*24*time.Hour)
	return models.ProgramOpenTermInput{ID: id, Name: name, ApplicationStartDate: &appStart, ApplicationEndDate: &appEnd, StartDateTime: &start, EndDateTime: &end}
}

func TestProgramService_Update_PassesOpenTerms(t *testing.T) {
	var captured models.ProgramUpdateInput
	repo := &stubProgRepo{update: func(_ context.Context, _ string, input models.ProgramUpdateInput) (*models.Program, error) {
		captured = input
		return &models.Program{ID: "prog-1"}, nil
	}}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	input := models.ProgramUpdateInput{Terms: []models.ProgramOpenTermInput{
		openTermEntry("7CAD5A8D-19D0-41A4-81A6-043453DAF9EE", " Fall 2026 "),
		openTermEntry("", "Spring 2027"),
	}}
	if _, err := svc.Update(context.Background(), "prog-1", input); err != nil {
		t.Fatal(err)
	}
	if len(captured.Terms) != 2 || captured.Terms[0].ID != "7cad5a8d-19d0-41a4-81a6-043453daf9ee" || captured.Terms[0].Name != "Fall 2026" || captured.Terms[1].ID != "" {
		t.Fatalf("terms=%+v", captured.Terms)
	}
}

func TestProgramService_Update_RejectsInvalidOpenTerms(t *testing.T) {
	const id = "7cad5a8d-19d0-41a4-81a6-043453daf9ee"
	badDates := openTermEntry("", "Bad")
	badDates.EndDateTime = badDates.StartDateTime
	missingDates := models.ProgramOpenTermInput{Name: "No dates"}

	tests := []struct {
		name  string
		terms []models.ProgramOpenTermInput
	}{
		{"empty set", []models.ProgramOpenTermInput{}},
		{"over the open-term cap", []models.ProgramOpenTermInput{openTermEntry("", "A"), openTermEntry("", "B"), openTermEntry("", "C"), openTermEntry("", "D"), openTermEntry("", "E")}},
		{"blank name", []models.ProgramOpenTermInput{openTermEntry("", "  ")}},
		{"invalid dates", []models.ProgramOpenTermInput{badDates}},
		{"missing dates", []models.ProgramOpenTermInput{missingDates}},
		{"malformed id", []models.ProgramOpenTermInput{openTermEntry("term-1", "Fall")}},
		{"duplicate id", []models.ProgramOpenTermInput{openTermEntry(id, "Fall"), openTermEntry(id, "Fall again")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubProgRepo{update: func(context.Context, string, models.ProgramUpdateInput) (*models.Program, error) {
				t.Fatal("repository must not be called for an invalid open-term set")
				return nil, nil
			}}
			svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
			_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Terms: tt.terms})
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("expected ErrInvalidInput, got %v", err)
			}
		})
	}
}

func TestProgramService_Update_OmittedTermsLeftUnchanged(t *testing.T) {
	var captured models.ProgramUpdateInput
	repo := &stubProgRepo{update: func(_ context.Context, _ string, input models.ProgramUpdateInput) (*models.Program, error) {
		captured = input
		return &models.Program{ID: "prog-1"}, nil
	}}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	name := "Renamed"
	if _, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if captured.Terms != nil {
		t.Fatalf("terms=%v; want nil", captured.Terms)
	}
}

func TestProgramService_Update_ChangesProject(t *testing.T) {
	var captured models.ProgramUpdateInput
	repo := &stubProgRepo{update: func(_ context.Context, _ string, input models.ProgramUpdateInput) (*models.Program, error) {
		captured = input
		return &models.Program{ID: "prog-1"}, nil
	}}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	uid, slug, name, logo := " 7CAD5A8D-19D0-41A4-81A6-043453DAF9EE ", " new-project ", " New Project ", " "
	_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{ProjectUID: &uid, ProjectSlug: &slug, ProjectName: &name, ProjectLogoURL: &logo})
	if err != nil {
		t.Fatal(err)
	}
	if captured.ProjectUID == nil || *captured.ProjectUID != "7cad5a8d-19d0-41a4-81a6-043453daf9ee" {
		t.Fatalf("project_uid = %v; want canonical UUID", captured.ProjectUID)
	}
	if captured.ProjectSlug == nil || *captured.ProjectSlug != "new-project" || captured.ProjectName == nil || *captured.ProjectName != "New Project" {
		t.Fatalf("project slug/name = %v %v", captured.ProjectSlug, captured.ProjectName)
	}
	if captured.ProjectLogoURL != nil {
		t.Fatalf("project_logo_url = %q; want nil for a blank logo", *captured.ProjectLogoURL)
	}
}

func TestProgramService_Update_RejectsInvalidProject(t *testing.T) {
	uid, slug, name, badLogo, notUUID := "7cad5a8d-19d0-41a4-81a6-043453daf9ee", "new-project", "New Project", "ftp://example.com/logo.svg", "not-a-uuid"
	cases := map[string]models.ProgramUpdateInput{
		"missing uid":  {ProjectSlug: &slug, ProjectName: &name},
		"invalid uid":  {ProjectUID: &notUUID, ProjectSlug: &slug, ProjectName: &name},
		"missing slug": {ProjectUID: &uid, ProjectName: &name},
		"missing name": {ProjectUID: &uid, ProjectSlug: &slug},
		"bad logo":     {ProjectUID: &uid, ProjectSlug: &slug, ProjectName: &name, ProjectLogoURL: &badLogo},
	}
	for label, input := range cases {
		t.Run(label, func(t *testing.T) {
			repo := &stubProgRepo{update: func(context.Context, string, models.ProgramUpdateInput) (*models.Program, error) {
				t.Fatal("repository must not be called for an invalid project")
				return nil, nil
			}}
			svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
			if _, err := svc.Update(context.Background(), "prog-1", input); !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("expected ErrInvalidInput, got %v", err)
			}
		})
	}
}

func TestProgramService_Update_ChangesProjectFromProjectService(t *testing.T) {
	var captured models.ProgramUpdateInput
	repo := &stubProgRepo{update: func(_ context.Context, _ string, input models.ProgramUpdateInput) (*models.Program, error) {
		captured = input
		return &models.Program{ID: "prog-1"}, nil
	}}
	lookup := &stubProjectLookup{project: &models.ProjectMetadata{Slug: "cncf", Name: "CNCF"}}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	svc.SetProjectLookup(lookup)
	uid, slug, name, logo := "7CAD5A8D-19D0-41A4-81A6-043453DAF9EE", "other-project", "Other Project", "javascript:alert(1)"

	if _, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{ProjectUID: &uid, ProjectSlug: &slug, ProjectName: &name, ProjectLogoURL: &logo}); err != nil {
		t.Fatal(err)
	}
	if lookup.gotUID != "7cad5a8d-19d0-41a4-81a6-043453daf9ee" {
		t.Errorf("lookup uid = %q; want the canonical UUID", lookup.gotUID)
	}
	if captured.ProjectSlug == nil || *captured.ProjectSlug != "cncf" || captured.ProjectName == nil || *captured.ProjectName != "CNCF" || captured.ProjectLogoURL != nil {
		t.Fatalf("project metadata = %v %v %v; want Project Service's, with no logo", captured.ProjectSlug, captured.ProjectName, captured.ProjectLogoURL)
	}
}

func TestProgramService_Update_ProjectLookupErrors(t *testing.T) {
	for label, tc := range map[string]struct {
		err  error
		want error
	}{
		"unknown project":     {err: domain.ErrProjectNotFound, want: domain.ErrInvalidInput},
		"service unavailable": {err: domain.ErrUpstreamUnavailable, want: domain.ErrUpstreamUnavailable},
	} {
		t.Run(label, func(t *testing.T) {
			svc := newProgramSvc(&stubProgRepo{update: func(context.Context, string, models.ProgramUpdateInput) (*models.Program, error) {
				t.Fatal("repository must not be called")
				return nil, nil
			}}, &stubTermRepo{}, &stubAppRepo{})
			svc.SetProjectLookup(&stubProjectLookup{err: tc.err})
			uid := "00000000-0000-0000-0000-000000000001"

			_, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{ProjectUID: &uid})
			if !errors.Is(err, tc.want) || errors.Is(err, domain.ErrProjectNotFound) {
				t.Fatalf("err = %v; want %v", err, tc.want)
			}
		})
	}
}

type fakeCrowdfundingClient struct {
	getCategorizedTransactions func(context.Context, string, string, bool, int, int) (*models.ProgramCategorizedTransactions, error)
}

func (f *fakeCrowdfundingClient) GetCategorizedTransactions(ctx context.Context, initiativeID, categoryType string, subscriptionOnly bool, limit, offset int) (*models.ProgramCategorizedTransactions, error) {
	if f.getCategorizedTransactions != nil {
		return f.getCategorizedTransactions(ctx, initiativeID, categoryType, subscriptionOnly, limit, offset)
	}
	return &models.ProgramCategorizedTransactions{}, nil
}

func TestProgramService_GetProgramSponsors_ClientNotConfigured(t *testing.T) {
	svc := newProgramSvc(&stubProgRepo{}, &stubTermRepo{}, &stubAppRepo{})
	_, err := svc.GetProgramSponsors(context.Background(), "prog-1", "mentorship", false, false)
	if !errors.Is(err, domain.ErrUpstreamUnavailable) {
		t.Errorf("expected ErrUpstreamUnavailable, got %v", err)
	}
}

func TestProgramService_GetProgramSponsors_AggregatesOrgsAndIndividuals(t *testing.T) {
	svc := newProgramSvc(&stubProgRepo{}, &stubTermRepo{}, &stubAppRepo{})

	var calls int
	svc.SetCrowdfundingClient(&fakeCrowdfundingClient{
		getCategorizedTransactions: func(_ context.Context, programID, categoryType string, subscriptionOnly bool, limit, offset int) (*models.ProgramCategorizedTransactions, error) {
			calls++
			if programID != "prog-1" {
				t.Errorf("programID = %q; want prog-1", programID)
			}
			if categoryType != string(models.MentorshipCategory) {
				t.Errorf("categoryType = %q; want mentorship", categoryType)
			}
			if !subscriptionOnly {
				t.Error("subscriptionOnly = false; want true")
			}
			if limit != 100 || offset != 0 {
				t.Errorf("limit/offset = %d/%d; want 100/0", limit, offset)
			}
			return &models.ProgramCategorizedTransactions{
				OrganizationTransactions: []models.ProgramTransaction{
					{ID: "o1", DonorType: "organization", DonorName: "GMC", AmountCents: 150000, DonorLogoURL: "logo-1"},
					{ID: "o2", DonorType: "organization", DonorName: "GMC", AmountCents: 100000},
					{ID: "o3", DonorType: "organization", DonorName: "Anonymous", AmountCents: 250000},
				},
				IndividualTransactions: []models.ProgramTransaction{
					{ID: "i1", DonorType: "individual", DonorName: "Alice", AmountCents: 30000},
					{ID: "i2", DonorType: "individual", DonorName: "Bob", AmountCents: 40000},
				},
				TotalCount: 5,
				Limit:      100,
				Offset:     0,
			}, nil
		},
	})

	out, err := svc.GetProgramSponsors(context.Background(), "prog-1", "", true, false)
	if err != nil {
		t.Fatalf("GetProgramSponsors: %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d; want 1", calls)
	}
	if len(out) != 3 {
		t.Fatalf("len(out) = %d; want 3", len(out))
	}
	if out[0].ID != "anonymous" || out[0].AmountCents != 250000 {
		t.Errorf("out[0] = %+v; want Anonymous 250000", out[0])
	}
	if out[1].ID != "gmc" || out[1].AmountCents != 250000 {
		t.Errorf("out[1] = %+v; want GMC 250000", out[1])
	}
	if out[2].ID != "individual-donors" || out[2].AmountCents != 70000 {
		t.Errorf("out[2] = %+v; want Individual donors 70000", out[2])
	}
}

func TestProgramService_GetProgramSponsors_AggregatePagesAllTransactions(t *testing.T) {
	svc := newProgramSvc(&stubProgRepo{}, &stubTermRepo{}, &stubAppRepo{})

	// GetProgramSponsors fetches the remaining pages concurrently, so the fake
	// is called from several goroutines at once and must guard its own state.
	var mu sync.Mutex
	callArgs := make([][2]int, 0, 2)
	svc.SetCrowdfundingClient(&fakeCrowdfundingClient{
		getCategorizedTransactions: func(_ context.Context, _ string, _ string, _ bool, limit, offset int) (*models.ProgramCategorizedTransactions, error) {
			mu.Lock()
			callArgs = append(callArgs, [2]int{limit, offset})
			mu.Unlock()
			switch offset {
			case 0:
				return &models.ProgramCategorizedTransactions{
					OrganizationTransactions: []models.ProgramTransaction{
						{ID: "o1", DonorType: "organization", DonorName: "Google", AmountCents: 1000},
						{ID: "o2", DonorType: "organization", DonorName: "Google", AmountCents: 2000},
					},
					IndividualTransactions: []models.ProgramTransaction{
						{ID: "i1", DonorType: "individual", DonorName: "Alice", AmountCents: 300},
					},
					TotalCount: 2501,
					Limit:      limit,
					Offset:     offset,
				}, nil
			case 2000:
				return &models.ProgramCategorizedTransactions{
					OrganizationTransactions: []models.ProgramTransaction{
						{ID: "o3", DonorType: "organization", DonorName: "Google", AmountCents: 1500},
					},
					IndividualTransactions: []models.ProgramTransaction{
						{ID: "i2", DonorType: "individual", DonorName: "Bob", AmountCents: 700},
					},
					TotalCount: 2501,
					Limit:      limit,
					Offset:     offset,
				}, nil
			default:
				return &models.ProgramCategorizedTransactions{
					TotalCount: 2501,
					Limit:      limit,
					Offset:     offset,
				}, nil
			}
		},
	})

	out, err := svc.GetProgramSponsors(context.Background(), "prog-1", "mentorship", false, true)
	if err != nil {
		t.Fatalf("GetProgramSponsors: %v", err)
	}
	if len(callArgs) != 2 {
		t.Fatalf("calls = %d; want 2", len(callArgs))
	}
	if callArgs[0] != [2]int{2000, 0} || callArgs[1] != [2]int{2000, 2000} {
		t.Fatalf("call args = %+v; want [[2000 0] [2000 2000]]", callArgs)
	}
	if len(out) != 2 {
		t.Fatalf("len(out) = %d; want 2", len(out))
	}
	if out[0].ID != "google" || out[0].AmountCents != 4500 {
		t.Errorf("out[0] = %+v; want Google 4500", out[0])
	}
	if out[1].ID != "individual-donors" || out[1].AmountCents != 1000 {
		t.Errorf("out[1] = %+v; want Individual donors 1000", out[1])
	}
}

func TestProgramService_GetProgramSponsors_AggregateDoesNotStopOnEmptyPage(t *testing.T) {
	svc := newProgramSvc(&stubProgRepo{}, &stubTermRepo{}, &stubAppRepo{})

	// GetProgramSponsors fetches the remaining pages concurrently, so the fake
	// is called from several goroutines at once and must guard its own state.
	var mu sync.Mutex
	callArgs := make([][2]int, 0, 2)
	svc.SetCrowdfundingClient(&fakeCrowdfundingClient{
		getCategorizedTransactions: func(_ context.Context, _ string, _ string, _ bool, limit, offset int) (*models.ProgramCategorizedTransactions, error) {
			mu.Lock()
			callArgs = append(callArgs, [2]int{limit, offset})
			mu.Unlock()
			switch offset {
			case 0:
				return &models.ProgramCategorizedTransactions{
					OrganizationTransactions: []models.ProgramTransaction{},
					IndividualTransactions:   []models.ProgramTransaction{},
					TotalCount:               2501,
					Limit:                    limit,
					Offset:                   offset,
				}, nil
			case 2000:
				return &models.ProgramCategorizedTransactions{
					OrganizationTransactions: []models.ProgramTransaction{
						{ID: "o1", DonorType: "organization", DonorName: "Google", AmountCents: 1200},
					},
					IndividualTransactions: []models.ProgramTransaction{
						{ID: "i1", DonorType: "individual", DonorName: "Alice", AmountCents: 800},
					},
					TotalCount: 2501,
					Limit:      limit,
					Offset:     offset,
				}, nil
			default:
				return &models.ProgramCategorizedTransactions{
					TotalCount: 2501,
					Limit:      limit,
					Offset:     offset,
				}, nil
			}
		},
	})

	out, err := svc.GetProgramSponsors(context.Background(), "prog-1", "mentorship", false, true)
	if err != nil {
		t.Fatalf("GetProgramSponsors: %v", err)
	}
	if len(callArgs) != 2 {
		t.Fatalf("calls = %d; want 2", len(callArgs))
	}
	if callArgs[0] != [2]int{2000, 0} || callArgs[1] != [2]int{2000, 2000} {
		t.Fatalf("call args = %+v; want [[2000 0] [2000 2000]]", callArgs)
	}
	if len(out) != 2 {
		t.Fatalf("len(out) = %d; want 2", len(out))
	}
	if out[0].ID != "google" || out[0].AmountCents != 1200 {
		t.Errorf("out[0] = %+v; want Google 1200", out[0])
	}
	if out[1].ID != "individual-donors" || out[1].AmountCents != 800 {
		t.Errorf("out[1] = %+v; want Individual donors 800", out[1])
	}
}

func TestProgramService_GetProgramSponsors_AggregateUsesReturnedLimitForOffsets(t *testing.T) {
	svc := newProgramSvc(&stubProgRepo{}, &stubTermRepo{}, &stubAppRepo{})

	// GetProgramSponsors fetches the remaining pages concurrently, so the fake
	// is called from several goroutines at once and must guard its own state.
	var mu sync.Mutex
	callArgs := make([][2]int, 0, 3)
	svc.SetCrowdfundingClient(&fakeCrowdfundingClient{
		getCategorizedTransactions: func(_ context.Context, _ string, _ string, _ bool, limit, offset int) (*models.ProgramCategorizedTransactions, error) {
			mu.Lock()
			callArgs = append(callArgs, [2]int{limit, offset})
			mu.Unlock()
			switch offset {
			case 0:
				return &models.ProgramCategorizedTransactions{
					OrganizationTransactions: []models.ProgramTransaction{
						{ID: "o1", DonorType: "organization", DonorName: "Google", AmountCents: 1000},
					},
					TotalCount: 250,
					Limit:      100,
					Offset:     0,
				}, nil
			case 100:
				return &models.ProgramCategorizedTransactions{
					IndividualTransactions: []models.ProgramTransaction{
						{ID: "i1", DonorType: "individual", DonorName: "Alice", AmountCents: 200},
					},
					TotalCount: 250,
					Limit:      100,
					Offset:     100,
				}, nil
			case 200:
				return &models.ProgramCategorizedTransactions{
					OrganizationTransactions: []models.ProgramTransaction{
						{ID: "o2", DonorType: "organization", DonorName: "Google", AmountCents: 300},
					},
					TotalCount: 250,
					Limit:      100,
					Offset:     200,
				}, nil
			default:
				return &models.ProgramCategorizedTransactions{TotalCount: 250, Limit: 100, Offset: offset}, nil
			}
		},
	})

	out, err := svc.GetProgramSponsors(context.Background(), "prog-1", "mentorship", false, true)
	if err != nil {
		t.Fatalf("GetProgramSponsors: %v", err)
	}
	if len(callArgs) != 3 {
		t.Fatalf("calls = %d; want 3", len(callArgs))
	}
	seen := map[[2]int]bool{}
	for _, args := range callArgs {
		seen[args] = true
	}
	for _, want := range [][2]int{{2000, 0}, {2000, 100}, {2000, 200}} {
		if !seen[want] {
			t.Fatalf("missing call args %v in %+v", want, callArgs)
		}
	}
	if len(out) != 2 {
		t.Fatalf("len(out) = %d; want 2", len(out))
	}
	if out[0].ID != "google" || out[0].AmountCents != 1300 {
		t.Errorf("out[0] = %+v; want Google 1300", out[0])
	}
	if out[1].ID != "individual-donors" || out[1].AmountCents != 200 {
		t.Errorf("out[1] = %+v; want Individual donors 200", out[1])
	}
}

func TestProgramService_GetCategorizedTransactions_ClientNotConfigured(t *testing.T) {
	svc := newProgramSvc(&stubProgRepo{}, &stubTermRepo{}, &stubAppRepo{})
	_, err := svc.GetCategorizedTransactions(context.Background(), "prog-1", "mentorship", false, 10, 0)
	if !errors.Is(err, domain.ErrUpstreamUnavailable) {
		t.Errorf("expected ErrUpstreamUnavailable, got %v", err)
	}
}

func TestProgramService_GetCategorizedTransactions_DefaultCategoryAndArgs(t *testing.T) {
	svc := newProgramSvc(&stubProgRepo{}, &stubTermRepo{}, &stubAppRepo{})

	var gotProgramID, gotCategory string
	var gotSubOnly bool
	var gotLimit, gotOffset int

	svc.SetCrowdfundingClient(&fakeCrowdfundingClient{
		getCategorizedTransactions: func(_ context.Context, programID, categoryType string, subscriptionOnly bool, limit, offset int) (*models.ProgramCategorizedTransactions, error) {
			gotProgramID = programID
			gotCategory = categoryType
			gotSubOnly = subscriptionOnly
			gotLimit = limit
			gotOffset = offset
			return &models.ProgramCategorizedTransactions{
				IndividualTransactions: []models.ProgramTransaction{{ID: "ind-1"}},
				TotalCount:             1,
				Limit:                  limit,
				Offset:                 offset,
			}, nil
		},
	})

	out, err := svc.GetCategorizedTransactions(context.Background(), "prog-1", "", true, 25, 3)
	if err != nil {
		t.Fatalf("GetCategorizedTransactions: %v", err)
	}
	if gotProgramID != "prog-1" {
		t.Errorf("programID = %q; want prog-1", gotProgramID)
	}
	if gotCategory != string(models.MentorshipCategory) {
		t.Errorf("category = %q; want %q", gotCategory, string(models.MentorshipCategory))
	}
	if !gotSubOnly || gotLimit != 25 || gotOffset != 3 {
		t.Errorf("args = subOnly=%v limit=%d offset=%d; want true/25/3", gotSubOnly, gotLimit, gotOffset)
	}
	if out.TotalCount != 1 {
		t.Errorf("TotalCount = %d; want 1", out.TotalCount)
	}
}

func TestProgramService_DeleteSkill_ActiveAdmin_ScopesDeleteByProgramAndSkillID(t *testing.T) {
	active := models.ProgramMemberStatusApproved
	called := false
	var gotProgramID, gotSkillID string

	svc := newProgramSvcWithMember(&stubProgRepo{
		deleteSkill: func(_ context.Context, programID, skillID string) error {
			called = true
			gotProgramID = programID
			gotSkillID = skillID
			return nil
		},
	}, &stubTermRepo{}, &stubAppRepo{}, &stubMemberRepo{
		findByProgramUser: func(_ context.Context, programID, userID string) (*models.ProgramMember, error) {
			if programID != "prog-1" || userID != "admin-1" {
				t.Fatalf("unexpected membership lookup args: %s %s", programID, userID)
			}
			return &models.ProgramMember{MemberType: models.MemberTypeProgramAdmin, Status: &active}, nil
		},
	})

	if err := svc.DeleteSkill(context.Background(), "prog-1", "skill-1", "admin-1"); err != nil {
		t.Fatalf("DeleteSkill: %v", err)
	}
	if !called {
		t.Fatal("expected DeleteSkill repository call")
	}
	if gotProgramID != "prog-1" || gotSkillID != "skill-1" {
		t.Fatalf("delete args = %s/%s; want prog-1/skill-1", gotProgramID, gotSkillID)
	}
}

func TestProgramService_DeleteSkill_MembershipLookupFailurePropagates(t *testing.T) {
	opErr := errors.New("db unavailable")
	svc := newProgramSvcWithMember(&stubProgRepo{}, &stubTermRepo{}, &stubAppRepo{}, &stubMemberRepo{
		findByProgramUser: func(_ context.Context, _, _ string) (*models.ProgramMember, error) {
			return nil, opErr
		},
	})

	err := svc.DeleteSkill(context.Background(), "prog-1", "skill-1", "admin-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, opErr) {
		t.Fatalf("expected wrapped lookup error, got %v", err)
	}
}

func TestProgramService_DeleteSkill_NonMemberForbidden(t *testing.T) {
	svc := newProgramSvcWithMember(&stubProgRepo{}, &stubTermRepo{}, &stubAppRepo{}, &stubMemberRepo{
		findByProgramUser: func(_ context.Context, _, _ string) (*models.ProgramMember, error) {
			return nil, domain.ErrProgramMemberNotFound
		},
	})

	err := svc.DeleteSkill(context.Background(), "prog-1", "skill-1", "admin-1")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestProgramService_DeleteSkill_WrongRoleForbidden(t *testing.T) {
	active := models.ProgramMemberStatusApproved
	svc := newProgramSvcWithMember(&stubProgRepo{}, &stubTermRepo{}, &stubAppRepo{}, &stubMemberRepo{
		findByProgramUser: func(_ context.Context, _, _ string) (*models.ProgramMember, error) {
			return &models.ProgramMember{MemberType: models.MemberTypeMentor, Status: &active}, nil
		},
	})

	err := svc.DeleteSkill(context.Background(), "prog-1", "skill-1", "mentor-1")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestProgramService_DeleteSkill_InactiveAdminForbidden(t *testing.T) {
	inactive := models.ProgramMemberStatusInvited
	svc := newProgramSvcWithMember(&stubProgRepo{}, &stubTermRepo{}, &stubAppRepo{}, &stubMemberRepo{
		findByProgramUser: func(_ context.Context, _, _ string) (*models.ProgramMember, error) {
			return &models.ProgramMember{MemberType: models.MemberTypeProgramAdmin, Status: &inactive}, nil
		},
	})

	err := svc.DeleteSkill(context.Background(), "prog-1", "skill-1", "admin-1")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestProgramService_ListMine_ScopesToCaller(t *testing.T) {
	var gotUser string
	var gotFilter models.AdministeredProgramFilter
	repo := &stubProgRepo{
		listAdministered: func(_ context.Context, userID string, f models.AdministeredProgramFilter) ([]*models.AdministeredProgram, *models.PaginationMeta, error) {
			gotUser, gotFilter = userID, f
			return []*models.AdministeredProgram{{ID: "p1"}}, &models.PaginationMeta{Total: 1}, nil
		},
	}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	got, meta, err := svc.ListMine(context.Background(), "user-1", models.AdministeredProgramFilter{
		Limit:  5,
		Search: "  grid  ",
		Status: models.AdministeredProgramStatusPendingReview,
	})
	if err != nil {
		t.Fatalf("ListMine: %v", err)
	}
	if gotUser != "user-1" {
		t.Errorf("user = %q; want user-1", gotUser)
	}
	want := models.AdministeredProgramFilter{Limit: 5, Search: "grid", Status: models.AdministeredProgramStatusPendingReview}
	if gotFilter != want {
		t.Errorf("filter = %+v; want %+v", gotFilter, want)
	}
	if len(got) != 1 || meta.Total != 1 {
		t.Errorf("got %d programs, total %d; want 1, 1", len(got), meta.Total)
	}
}

func TestProgramService_ListMine_RejectsBadInput(t *testing.T) {
	repo := &stubProgRepo{
		listAdministered: func(context.Context, string, models.AdministeredProgramFilter) ([]*models.AdministeredProgram, *models.PaginationMeta, error) {
			t.Fatal("repo must not be called for invalid input")
			return nil, nil, nil
		},
	}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})

	if _, _, err := svc.ListMine(context.Background(), "", models.AdministeredProgramFilter{}); !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("empty user: got %v; want ErrUnauthorized", err)
	}
	// The BFF's display value uses a hyphen; the API value is pending_review.
	for _, status := range []models.AdministeredProgramStatus{"pending-review", "published", "pending"} {
		if _, _, err := svc.ListMine(context.Background(), "user-1", models.AdministeredProgramFilter{Status: status}); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("status %q: got %v; want ErrInvalidInput", status, err)
		}
	}
}

func TestProgramService_ListMine_WrapsRepoError(t *testing.T) {
	repoErr := errors.New("db down")
	repo := &stubProgRepo{
		listAdministered: func(context.Context, string, models.AdministeredProgramFilter) ([]*models.AdministeredProgram, *models.PaginationMeta, error) {
			return nil, nil, repoErr
		},
	}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})
	if _, _, err := svc.ListMine(context.Background(), "user-1", models.AdministeredProgramFilter{}); !errors.Is(err, repoErr) {
		t.Errorf("got %v; want wrapped repo error", err)
	}
}

// validEnrollment passes every CreateEnrollment check when no Project Service lookup is configured.
func validEnrollment() models.ProgramEnrollmentInput {
	projectUID := "00000000-0000-0000-0000-000000000001"
	start := time.Now().Add(24 * time.Hour)
	termEnd := start.Add(24 * time.Hour)
	applicationStart := time.Now()
	applicationEnd := start.Add(-time.Hour)
	slug, name := "program-project", "Program Project"
	return models.ProgramEnrollmentInput{
		Program: models.ProgramCreateInput{CreatorUserID: "creator-1", ProjectUID: &projectUID, ProjectSlug: &slug, ProjectName: &name, Name: "Program", Slug: "program"},
		Skills:  []string{"Go"},
		Terms:   []models.ProgramTermCreateInput{{Name: "Term", StartDateTime: &start, EndDateTime: &termEnd, ApplicationStartDate: &applicationStart, ApplicationEndDate: &applicationEnd}},
	}
}

func TestProgramService_CreateEnrollment_RejectsInvalidInput(t *testing.T) {
	blank, logo, notUUID := " ", "https://example.org/logo.png", "not-a-uuid"
	for label, mutate := range map[string]func(*models.ProgramEnrollmentInput){
		"logo set outside its file route": func(in *models.ProgramEnrollmentInput) { in.Program.LogoURL = &logo },
		"too many terms": func(in *models.ProgramEnrollmentInput) {
			in.Terms = []models.ProgramTermCreateInput{in.Terms[0], in.Terms[0], in.Terms[0], in.Terms[0], in.Terms[0]}
		},
		"only blank skills": func(in *models.ProgramEnrollmentInput) { in.Skills = []string{" ", ""} },
		"blank term name":   func(in *models.ProgramEnrollmentInput) { in.Terms[0].Name = blank },
		"term not open":     func(in *models.ProgramEnrollmentInput) { in.Terms[0].Status = models.ProgramTermStatusClosed },
		"term ends before start": func(in *models.ProgramEnrollmentInput) {
			in.Terms[0].EndDateTime, in.Terms[0].StartDateTime = in.Terms[0].StartDateTime, in.Terms[0].EndDateTime
		},
		"prerequisites not JSON": func(in *models.ProgramEnrollmentInput) { in.Prerequisites = json.RawMessage(`{`) },
		"required prerequisite without description": func(in *models.ProgramEnrollmentInput) {
			in.Prerequisites = json.RawMessage(`[{"name":"PR","required":true}]`)
		},
		"blank program name":   func(in *models.ProgramEnrollmentInput) { in.Program.Name = blank },
		"blank slug":           func(in *models.ProgramEnrollmentInput) { in.Program.Slug = blank },
		"missing project_uid":  func(in *models.ProgramEnrollmentInput) { in.Program.ProjectUID = nil },
		"blank project_uid":    func(in *models.ProgramEnrollmentInput) { in.Program.ProjectUID = &blank },
		"project_uid not UUID": func(in *models.ProgramEnrollmentInput) { in.Program.ProjectUID = &notUUID },
		"missing project name": func(in *models.ProgramEnrollmentInput) { in.Program.ProjectName = nil },
	} {
		t.Run(label, func(t *testing.T) {
			svc := newProgramSvc(&stubProgRepo{createEnrollment: func(context.Context, models.ProgramEnrollmentInput) (*models.Program, error) {
				t.Fatal("repository must not be called")
				return nil, nil
			}}, &stubTermRepo{}, &stubAppRepo{})
			in := validEnrollment()
			mutate(&in)
			if _, err := svc.CreateEnrollment(context.Background(), in); !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("err = %v; want ErrInvalidInput", err)
			}
		})
	}
}

func TestProgramService_CreateEnrollment_NameAvailability(t *testing.T) {
	repoErr := errors.New("db down")
	for label, tc := range map[string]struct {
		available bool
		err       error
		want      error
	}{
		"name taken":   {available: false, want: domain.ErrConflict},
		"lookup fails": {err: repoErr, want: repoErr},
	} {
		t.Run(label, func(t *testing.T) {
			var gotName string
			svc := newProgramSvc(&stubProgRepo{
				nameAvailable: func(_ context.Context, name, _ string) (bool, error) {
					gotName = name
					return tc.available, tc.err
				},
				createEnrollment: func(context.Context, models.ProgramEnrollmentInput) (*models.Program, error) {
					t.Fatal("repository must not be called")
					return nil, nil
				},
			}, &stubTermRepo{}, &stubAppRepo{})
			if _, err := svc.CreateEnrollment(context.Background(), validEnrollment()); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v; want %v", err, tc.want)
			}
			if gotName != "Program" {
				t.Errorf("checked name %q; want %q", gotName, "Program")
			}
		})
	}
}

func TestProgramService_CreateEnrollment_KeepsOnlyRequiredPrerequisites(t *testing.T) {
	var captured models.ProgramEnrollmentInput
	svc := newProgramSvc(&stubProgRepo{createEnrollment: func(_ context.Context, in models.ProgramEnrollmentInput) (*models.Program, error) {
		captured = in
		return &models.Program{ID: in.Program.ID}, nil
	}}, &stubTermRepo{}, &stubAppRepo{})
	in := validEnrollment()
	in.Prerequisites = json.RawMessage(`[
		{"name":"Optional","required":false},
		{"name":"Upload CV","description":"Attach a CV","required":true,"requireFile":true,"dueDate":"2030-01-02"},
		{"name":"Intro","description":"Say hello","required":true}
	]`)
	if _, err := svc.CreateEnrollment(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	var templates []struct {
		Name       string  `json:"name"`
		SubmitFile *string `json:"submitFile"`
		DueDate    *string `json:"dueDate"`
	}
	if err := json.Unmarshal(captured.Prerequisites, &templates); err != nil {
		t.Fatalf("decode stored templates: %v", err)
	}
	if len(templates) != 2 || templates[0].Name != "Upload CV" || templates[1].Name != "Intro" {
		t.Fatalf("templates = %+v; want only the two required prerequisites, in order", templates)
	}
	if templates[0].SubmitFile == nil || *templates[0].SubmitFile != "required" || templates[0].DueDate == nil || *templates[0].DueDate != "2030-01-02" {
		t.Errorf("Upload CV template = %+v; want a required file and its due date", templates[0])
	}
	if templates[1].SubmitFile != nil {
		t.Errorf("Intro submitFile = %q; want none", *templates[1].SubmitFile)
	}
	if captured.Program.Status != models.ProgramStatusPending || captured.Program.ID == "" {
		t.Errorf("stored status %q id %q; want a pending program with a generated ID", captured.Program.Status, captured.Program.ID)
	}
}

func TestProgramService_CreateEnrollment_RepoErrorSendsNothing(t *testing.T) {
	repoErr := errors.New("db down")
	n := &stubNotifier{}
	svc := newProgramSvcWithNotifier(&stubProgRepo{createEnrollment: func(context.Context, models.ProgramEnrollmentInput) (*models.Program, error) {
		return nil, repoErr
	}}, &stubTermRepo{}, &stubAppRepo{}, n)
	if _, err := svc.CreateEnrollment(context.Background(), validEnrollment()); !errors.Is(err, repoErr) {
		t.Fatalf("err = %v; want the repository error", err)
	}
	if n.programSubmittedCalls != 0 {
		t.Errorf("submission notifications = %d; want 0", n.programSubmittedCalls)
	}
}

func TestProgramService_Update_RejectsUnknownStatus(t *testing.T) {
	svc := newProgramSvc(&stubProgRepo{getByID: func(context.Context, string) (*models.Program, error) {
		t.Fatal("repository must not be read for an invalid status")
		return nil, nil
	}}, &stubTermRepo{}, &stubAppRepo{})
	bogus := models.ProgramStatus("bogus")
	if _, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: &bogus}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v; want ErrInvalidInput", err)
	}
}

func TestProgramService_Update_Publish_RequiresRepoAndLogo(t *testing.T) {
	blank := "  "
	for label, tc := range map[string]struct {
		mutate func(*models.Program)
		want   string
	}{
		"no repository URL":    {func(p *models.Program) { p.RepoLink = nil }, "repository URL is required"},
		"blank repository URL": {func(p *models.Program) { p.RepoLink = &blank }, "repository URL is required"},
		"no logo":              {func(p *models.Program) { p.LogoURL = nil }, "logo is required"},
		"blank logo":           {func(p *models.Program) { p.LogoURL = &blank }, "logo is required"},
	} {
		t.Run(label, func(t *testing.T) {
			prog := fullProgram()
			tc.mutate(prog)
			// Every other publication prerequisite holds, so only the guard under test can refuse.
			repo := &stubProgRepo{
				getByID: func(context.Context, string) (*models.Program, error) { return prog, nil },
				listSkills: func(context.Context, string) ([]*models.ProgramSkill, error) {
					return []*models.ProgramSkill{{ID: "s1"}}, nil
				},
				update: func(context.Context, string, models.ProgramUpdateInput) (*models.Program, error) {
					t.Fatal("an incomplete program must not be published")
					return nil, nil
				},
			}
			termRepo := &stubTermRepo{countOpenByProgram: func(context.Context, string) (int, error) { return 1, nil }}
			next := models.ProgramStatusPublished
			_, err := newProgramSvc(repo, termRepo, &stubAppRepo{}).Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: &next})
			if !errors.Is(err, domain.ErrStateLocked) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v; want ErrStateLocked mentioning %q", err, tc.want)
			}
		})
	}
}

func TestProgramService_Update_PropagatesRepositoryErrors(t *testing.T) {
	repoErr := errors.New("db down")
	publish, hide := models.ProgramStatusPublished, models.ProgramStatusHidden
	skills := func(context.Context, string) ([]*models.ProgramSkill, error) {
		return []*models.ProgramSkill{{ID: "s1"}}, nil
	}
	pending := func(context.Context, string) (*models.Program, error) { return fullProgram(), nil }
	published := func(_ context.Context, id string) (*models.Program, error) {
		return &models.Program{ID: id, Status: models.ProgramStatusPublished}, nil
	}
	failingLoad := func(context.Context, string) (*models.Program, error) { return nil, repoErr }
	for label, tc := range map[string]struct {
		status    *models.ProgramStatus
		templates json.RawMessage
		progRepo  *stubProgRepo
		termRepo  *stubTermRepo
		appRepo   *stubAppRepo
	}{
		"load program":                    {status: &publish, progRepo: &stubProgRepo{getByID: failingLoad}},
		"load program for task templates": {templates: json.RawMessage(`[{"name":"Intro"}]`), progRepo: &stubProgRepo{getByID: failingLoad}},
		"list skills": {status: &publish, progRepo: &stubProgRepo{getByID: pending, listSkills: func(context.Context, string) ([]*models.ProgramSkill, error) {
			return nil, repoErr
		}}},
		"count open terms": {
			status:   &publish,
			progRepo: &stubProgRepo{getByID: pending, listSkills: skills},
			termRepo: &stubTermRepo{countOpenByProgram: func(context.Context, string) (int, error) { return 0, repoErr }},
		},
		"count blocking applications": {
			status:   &hide,
			progRepo: &stubProgRepo{getByID: published},
			appRepo:  &stubAppRepo{countBlocking: func(context.Context, string) (int, error) { return 0, repoErr }},
		},
		"write update": {progRepo: &stubProgRepo{update: func(context.Context, string, models.ProgramUpdateInput) (*models.Program, error) {
			return nil, repoErr
		}}},
	} {
		t.Run(label, func(t *testing.T) {
			if tc.termRepo == nil {
				tc.termRepo = &stubTermRepo{}
			}
			if tc.appRepo == nil {
				tc.appRepo = &stubAppRepo{}
			}
			svc := newProgramSvc(tc.progRepo, tc.termRepo, tc.appRepo)
			if _, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Status: tc.status, TaskTemplates: tc.templates}); !errors.Is(err, repoErr) {
				t.Fatalf("err = %v; want the repository error", err)
			}
		})
	}
}

func TestProgramService_DescriptionLengthCapped(t *testing.T) {
	// Multi-byte characters: the cap counts characters, not bytes.
	atCap, overCap := strings.Repeat("é", 18_000), strings.Repeat("<", 18_001)
	writes := 0
	repo := &stubProgRepo{update: func(_ context.Context, id string, _ models.ProgramUpdateInput) (*models.Program, error) {
		writes++
		return &models.Program{ID: id}, nil
	}}
	svc := newProgramSvc(repo, &stubTermRepo{}, &stubAppRepo{})

	if _, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Description: &atCap}); err != nil {
		t.Fatalf("update at the cap: %v", err)
	}
	if _, err := svc.Update(context.Background(), "prog-1", models.ProgramUpdateInput{Description: &overCap}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("update over the cap: expected ErrInvalidInput, got %v", err)
	}
	if writes != 1 {
		t.Errorf("repository writes = %d; want only the update at the cap", writes)
	}

	projectUID := "00000000-0000-0000-0000-000000000001"
	enrollment := models.ProgramEnrollmentInput{Program: models.ProgramCreateInput{ProjectUID: &projectUID, Name: "Program", Slug: "program", Description: &overCap}, Skills: []string{"Go"}}
	if _, err := svc.CreateEnrollment(context.Background(), enrollment); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("enrollment over the cap: expected ErrInvalidInput, got %v", err)
	}
	if _, err := svc.Create(context.Background(), enrollment.Program); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("create over the cap: expected ErrInvalidInput, got %v", err)
	}
}
