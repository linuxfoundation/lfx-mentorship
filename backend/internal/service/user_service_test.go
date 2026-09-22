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

type stubUserRepository struct {
	delete func(context.Context, string) error
	update func(context.Context, string, models.UserUpdateInput) (*models.User, error)
}

func (s *stubUserRepository) GetByID(context.Context, string) (*models.User, error) {
	return &models.User{}, nil
}

func (s *stubUserRepository) GetByLFID(context.Context, string) (*models.User, error) {
	return nil, domain.ErrUserNotFound
}

func (s *stubUserRepository) List(context.Context, models.UserFilter) ([]*models.User, *models.PaginationMeta, error) {
	return nil, &models.PaginationMeta{}, nil
}

func (s *stubUserRepository) Create(context.Context, models.UserCreateInput) (*models.User, error) {
	return &models.User{}, nil
}

func (s *stubUserRepository) UpsertByLFID(context.Context, models.UserCreateInput) (*models.User, error) {
	return &models.User{}, nil
}

func (s *stubUserRepository) Update(ctx context.Context, id string, input models.UserUpdateInput) (*models.User, error) {
	if s.update != nil {
		return s.update(ctx, id, input)
	}
	return &models.User{}, nil
}

func (s *stubUserRepository) Delete(ctx context.Context, id string) error {
	return s.delete(ctx, id)
}

func TestUserService_Delete_AllowsOwner(t *testing.T) {
	called := false
	svc := service.NewUserService(&stubUserRepository{
		delete: func(_ context.Context, id string) error {
			called = id == "user-1"
			return nil
		},
	})

	if err := svc.Delete(context.Background(), "user-1", "user-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !called {
		t.Fatal("expected repository delete for owner")
	}
}

func TestUserService_DeleteRejectsOtherActor(t *testing.T) {
	svc := service.NewUserService(&stubUserRepository{
		delete: func(context.Context, string) error {
			t.Fatal("repository delete should not be called")
			return nil
		},
	})

	if err := svc.Delete(context.Background(), "user-1", "user-2"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("err = %v; want ErrForbidden", err)
	}
}

func TestUserService_DeleteRejectsMissingActor(t *testing.T) {
	svc := service.NewUserService(&stubUserRepository{
		delete: func(context.Context, string) error {
			t.Fatal("repository delete should not be called")
			return nil
		},
	})

	if err := svc.Delete(context.Background(), "user-1", ""); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("err = %v; want ErrForbidden", err)
	}
}

func TestUserService_UpdateRejectsLFIDMutation(t *testing.T) {
	called := false
	svc := service.NewUserService(&stubUserRepository{
		update: func(context.Context, string, models.UserUpdateInput) (*models.User, error) {
			called = true
			return &models.User{}, nil
		},
	})
	lfid := "attempted-change"
	_, err := svc.Update(context.Background(), "user-1", models.UserUpdateInput{LFID: &lfid})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("err = %v; want ErrForbidden", err)
	}
	if called {
		t.Fatal("repository update should not be called when LFID mutation is requested")
	}
}
