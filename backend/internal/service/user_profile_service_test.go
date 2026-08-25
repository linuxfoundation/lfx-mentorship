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

// stubUserProfileRepo implements domain.UserProfileRepository.
type stubUserProfileRepo struct {
	getByID                  func(context.Context, string) (*models.UserProfile, error)
	getBySlug                func(context.Context, string) (*models.UserProfile, error)
	list                     func(context.Context, models.UserProfileFilter) ([]*models.UserProfile, *models.PaginationMeta, error)
	create                   func(context.Context, models.UserProfileCreateInput) (*models.UserProfile, error)
	update                   func(context.Context, string, models.UserProfileUpdateInput) (*models.UserProfile, error)
	delete                   func(context.Context, string) error
	countActiveMenteeProfiles func(context.Context, string) (int, error)
}

func (m *stubUserProfileRepo) GetByID(ctx context.Context, id string) (*models.UserProfile, error) {
	if m.getByID != nil {
		return m.getByID(ctx, id)
	}
	return &models.UserProfile{ID: id}, nil
}
func (m *stubUserProfileRepo) GetBySlug(ctx context.Context, slug string) (*models.UserProfile, error) {
	if m.getBySlug != nil {
		return m.getBySlug(ctx, slug)
	}
	return &models.UserProfile{}, nil
}
func (m *stubUserProfileRepo) List(ctx context.Context, f models.UserProfileFilter) ([]*models.UserProfile, *models.PaginationMeta, error) {
	if m.list != nil {
		return m.list(ctx, f)
	}
	return nil, &models.PaginationMeta{}, nil
}
func (m *stubUserProfileRepo) Create(ctx context.Context, in models.UserProfileCreateInput) (*models.UserProfile, error) {
	if m.create != nil {
		return m.create(ctx, in)
	}
	return &models.UserProfile{UserID: in.UserID, ProfileType: in.ProfileType}, nil
}
func (m *stubUserProfileRepo) Update(ctx context.Context, id string, in models.UserProfileUpdateInput) (*models.UserProfile, error) {
	if m.update != nil {
		return m.update(ctx, id, in)
	}
	return &models.UserProfile{ID: id}, nil
}
func (m *stubUserProfileRepo) Delete(ctx context.Context, id string) error {
	if m.delete != nil {
		return m.delete(ctx, id)
	}
	return nil
}
func (m *stubUserProfileRepo) CountActiveMenteeProfiles(ctx context.Context, userID string) (int, error) {
	if m.countActiveMenteeProfiles != nil {
		return m.countActiveMenteeProfiles(ctx, userID)
	}
	return 0, nil
}

func newUserProfileSvc(repo *stubUserProfileRepo) *service.UserProfileService {
	return service.NewUserProfileService(repo)
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestUserProfileService_Create_Mentee_AgeEligibleRequired(t *testing.T) {
	svc := newUserProfileSvc(&stubUserProfileRepo{})
	_, err := svc.Create(context.Background(), models.UserProfileCreateInput{
		UserID:       "u1",
		ProfileType:  "mentee",
		AgeEligible:  false,
		WorkEligible: true,
	})
	if !errors.Is(err, domain.ErrIneligible) {
		t.Errorf("expected ErrIneligible when age_eligible=false, got %v", err)
	}
}

func TestUserProfileService_Create_Mentee_WorkEligibleRequired(t *testing.T) {
	svc := newUserProfileSvc(&stubUserProfileRepo{})
	_, err := svc.Create(context.Background(), models.UserProfileCreateInput{
		UserID:       "u1",
		ProfileType:  "mentee",
		AgeEligible:  true,
		WorkEligible: false,
	})
	if !errors.Is(err, domain.ErrIneligible) {
		t.Errorf("expected ErrIneligible when work_eligible=false, got %v", err)
	}
}

func TestUserProfileService_Create_Mentee_AlreadyHasActiveProfile(t *testing.T) {
	repo := &stubUserProfileRepo{
		countActiveMenteeProfiles: func(_ context.Context, _ string) (int, error) {
			return 1, nil
		},
	}
	svc := newUserProfileSvc(repo)
	_, err := svc.Create(context.Background(), models.UserProfileCreateInput{
		UserID:       "u1",
		ProfileType:  "mentee",
		AgeEligible:  true,
		WorkEligible: true,
	})
	if !errors.Is(err, domain.ErrIneligible) {
		t.Errorf("expected ErrIneligible when user already has active mentee profile, got %v", err)
	}
}

func TestUserProfileService_Create_Mentee_Eligible_OK(t *testing.T) {
	svc := newUserProfileSvc(&stubUserProfileRepo{})
	_, err := svc.Create(context.Background(), models.UserProfileCreateInput{
		UserID:       "u1",
		ProfileType:  "mentee",
		AgeEligible:  true,
		WorkEligible: true,
	})
	if err != nil {
		t.Errorf("eligible mentee profile creation should succeed, got %v", err)
	}
}

func TestUserProfileService_Create_Mentor_NoEligibilityCheck(t *testing.T) {
	// Mentor profiles must NOT require age/work eligibility fields.
	svc := newUserProfileSvc(&stubUserProfileRepo{})
	_, err := svc.Create(context.Background(), models.UserProfileCreateInput{
		UserID:       "u2",
		ProfileType:  "mentor",
		AgeEligible:  false,
		WorkEligible: false,
	})
	if err != nil {
		t.Errorf("mentor profile should not require eligibility flags, got %v", err)
	}
}

func TestUserProfileService_Create_GeneratesIDWhenEmpty(t *testing.T) {
	var capturedID string
	repo := &stubUserProfileRepo{
		create: func(_ context.Context, in models.UserProfileCreateInput) (*models.UserProfile, error) {
			capturedID = in.ID
			return &models.UserProfile{ID: in.ID}, nil
		},
	}
	svc := newUserProfileSvc(repo)
	_, err := svc.Create(context.Background(), models.UserProfileCreateInput{
		UserID:      "u1",
		ProfileType: "mentor",
		// ID intentionally left empty
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if capturedID == "" {
		t.Error("expected a generated UUID when ID is empty")
	}
}

func TestUserProfileService_Create_MissingUserID(t *testing.T) {
	svc := newUserProfileSvc(&stubUserProfileRepo{})
	_, err := svc.Create(context.Background(), models.UserProfileCreateInput{ProfileType: "mentor"})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for missing user_id, got %v", err)
	}
}

func TestUserProfileService_Create_InvalidProfileType(t *testing.T) {
	svc := newUserProfileSvc(&stubUserProfileRepo{})
	_, err := svc.Create(context.Background(), models.UserProfileCreateInput{
		UserID:      "u1",
		ProfileType: "admin",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for invalid profile_type, got %v", err)
	}
}
