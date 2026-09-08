// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

func newProgramSvc(progRepo *stubProgRepo, termRepo *stubTermRepo, appRepo *stubAppRepo) *service.ProgramService {
	return service.NewProgramService(progRepo, termRepo, appRepo)
}

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

func TestProgramService_GetCatalog_Unpublished(t *testing.T) {
	for _, status := range []models.ProgramStatus{
		models.ProgramStatusDraft,
		models.ProgramStatusRejected,
		models.ProgramStatusHidden,
		models.ProgramStatusArchived,
		models.ProgramStatusSubmitted,
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
			return []*models.ProgramCatalogMentee{{UserID: "u1", Status: "active", TermID: "t1", TermName: "Spring 2026"}}, nil
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

	callArgs := make([][2]int, 0, 2)
	svc.SetCrowdfundingClient(&fakeCrowdfundingClient{
		getCategorizedTransactions: func(_ context.Context, _ string, _ string, _ bool, limit, offset int) (*models.ProgramCategorizedTransactions, error) {
			callArgs = append(callArgs, [2]int{limit, offset})
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

	callArgs := make([][2]int, 0, 2)
	svc.SetCrowdfundingClient(&fakeCrowdfundingClient{
		getCategorizedTransactions: func(_ context.Context, _ string, _ string, _ bool, limit, offset int) (*models.ProgramCategorizedTransactions, error) {
			callArgs = append(callArgs, [2]int{limit, offset})
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
