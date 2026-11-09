// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

type accessWait struct{ object, relation, username string }

type stubAccessWaiter struct {
	calls []accessWait
	err   error
}

func (s *stubAccessWaiter) WaitForAccess(_ context.Context, object, relation, username string) error {
	s.calls = append(s.calls, accessWait{object, relation, username})
	return s.err
}

func creatorContext(username string) context.Context {
	return auth.ContextWithPrincipal(context.Background(), &models.Principal{UserID: "user-1", Username: username})
}

func createdProgramRepo() *stubProgRepo {
	return &stubProgRepo{createEnrollment: func(_ context.Context, in models.ProgramEnrollmentInput) (*models.Program, error) {
		return &models.Program{ID: in.Program.ID}, nil
	}}
}

func TestProgramService_CreateEnrollment_WaitsForCreatorWriter(t *testing.T) {
	waiter := &stubAccessWaiter{}
	svc := newProgramSvc(createdProgramRepo(), &stubTermRepo{}, &stubAppRepo{})
	svc.SetAccessWaiter(waiter)

	program, err := svc.CreateEnrollment(creatorContext("creator-lfid"), validEnrollment())
	if err != nil {
		t.Fatal(err)
	}
	want := accessWait{"mentorship_program:" + program.ID, "writer", "creator-lfid"}
	if len(waiter.calls) != 1 || waiter.calls[0] != want {
		t.Fatalf("waits = %+v; want [%+v]", waiter.calls, want)
	}
}

func TestProgramService_CreateEnrollment_WaitTimeoutStillCreates(t *testing.T) {
	waiter := &stubAccessWaiter{err: context.DeadlineExceeded}
	svc := newProgramSvc(createdProgramRepo(), &stubTermRepo{}, &stubAppRepo{})
	svc.SetAccessWaiter(waiter)

	program, err := svc.CreateEnrollment(creatorContext("creator-lfid"), validEnrollment())
	if err != nil || program == nil {
		t.Fatalf("program = %v, err = %v; want the created program despite the wait failing", program, err)
	}
}

func TestProgramService_CreateEnrollment_SkipsWait(t *testing.T) {
	for label, tc := range map[string]struct {
		ctx  context.Context
		repo *stubProgRepo
	}{
		"no principal":  {ctx: context.Background(), repo: createdProgramRepo()},
		"M2M principal": {ctx: creatorContext("svc@clients"), repo: createdProgramRepo()},
		"create fails": {ctx: creatorContext("creator-lfid"), repo: &stubProgRepo{createEnrollment: func(context.Context, models.ProgramEnrollmentInput) (*models.Program, error) {
			return nil, errors.New("db down")
		}}},
	} {
		t.Run(label, func(t *testing.T) {
			waiter := &stubAccessWaiter{}
			svc := newProgramSvc(tc.repo, &stubTermRepo{}, &stubAppRepo{})
			svc.SetAccessWaiter(waiter)
			_, _ = svc.CreateEnrollment(tc.ctx, validEnrollment())
			if len(waiter.calls) != 0 {
				t.Errorf("waits = %+v; want none", waiter.calls)
			}
		})
	}
}

func TestApplicationService_Create_WaitsForApplicantAuditor(t *testing.T) {
	openTermRepo := &stubTermRepo{getByID: func(context.Context, string) (*models.ProgramTerm, error) {
		return openTerm(time.Now()), nil
	}}
	for label, repo := range map[string]*stubAppRepo{
		"new application": {create: func(_ context.Context, _ string, in models.ApplicationCreateInput) (*models.Application, error) {
			return &models.Application{ID: "app-new", Status: in.Status}, nil
		}},
		"reapply after withdrawing": {
			findByTermAndUser: func(context.Context, string, string, models.ApplicationRole) (*models.Application, error) {
				return &models.Application{ID: "app-old", Status: models.ApplicationStatusWithdrawn}, nil
			},
			reapply: func(context.Context, string, string, models.ApplicationCreateInput) (*models.Application, error) {
				return &models.Application{ID: "app-new", Status: models.ApplicationStatusPending}, nil
			},
		},
	} {
		t.Run(label, func(t *testing.T) {
			waiter := &stubAccessWaiter{}
			svc := newApplicationSvc(repo, &stubTaskRepo{}, openTermRepo, &stubProgRepo{})
			svc.SetAccessWaiter(waiter)

			if _, err := svc.Create(creatorContext("mentee-lfid"), "term-1", models.ApplicationCreateInput{UserID: "user-1", Role: models.ApplicationRoleMentee}); err != nil {
				t.Fatal(err)
			}
			want := accessWait{"mentorship_application:app-new", "auditor", "mentee-lfid"}
			if len(waiter.calls) != 1 || waiter.calls[0] != want {
				t.Fatalf("waits = %+v; want [%+v]", waiter.calls, want)
			}
		})
	}
}

func TestApplicationService_Create_RefusedApplicationSkipsWait(t *testing.T) {
	waiter := &stubAccessWaiter{}
	termRepo := &stubTermRepo{getByID: func(context.Context, string) (*models.ProgramTerm, error) {
		return &models.ProgramTerm{ID: "term-1", Status: models.ProgramTermStatusClosed}, nil
	}}
	svc := newApplicationSvc(&stubAppRepo{}, &stubTaskRepo{}, termRepo, &stubProgRepo{})
	svc.SetAccessWaiter(waiter)

	if _, err := svc.Create(creatorContext("mentee-lfid"), "term-1", models.ApplicationCreateInput{UserID: "user-1", Role: models.ApplicationRoleMentee}); err == nil {
		t.Fatal("err = nil; want the closed-term refusal")
	}
	if len(waiter.calls) != 0 {
		t.Errorf("waits = %+v; want none", waiter.calls)
	}
}
