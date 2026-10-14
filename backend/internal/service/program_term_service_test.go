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

func TestProgramTermService_ListByProgram_RejectsNonPublicStatus(t *testing.T) {
	svc := service.NewProgramTermService(&stubTermRepo{}, &stubAppRepo{}, &stubNotifier{})
	for _, status := range []string{"deleted", "bogus"} {
		_, _, err := svc.ListByProgram(context.Background(), "p1", models.ProgramTermFilter{Status: status})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("status %q: err=%v; want ErrInvalidInput", status, err)
		}
	}
}

func TestProgramTermService_Create_ApplicationWindowMayRunIntoTerm(t *testing.T) {
	day := func(n int) *time.Time {
		d := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, n)
		return &d
	}
	cases := []struct {
		name             string
		appStart, appEnd int
		wantErr          bool
	}{
		{"closes before the term starts", 0, 10, false},
		{"closes after the term starts", 0, 40, false},
		{"closes as the term ends", 0, 90, false},
		{"closes after the term ends", 0, 91, true},
		{"closes as it opens", 10, 10, true},
	}
	svc := service.NewProgramTermService(&stubTermRepo{}, &stubAppRepo{}, &stubNotifier{})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(context.Background(), models.ProgramTermCreateInput{
				ProgramID: "p1", Name: "Fall", StartDateTime: day(30), EndDateTime: day(90),
				ApplicationStartDate: day(tc.appStart), ApplicationEndDate: day(tc.appEnd),
			})
			if got := errors.Is(err, domain.ErrInvalidInput); got != tc.wantErr {
				t.Fatalf("err = %v; want invalid input = %v", err, tc.wantErr)
			}
		})
	}
}

func TestProgramTermService_Close_NotifiesDeclinedMentees(t *testing.T) {
	termRepo := &stubTermRepo{closeWithBulkDecline: func(_ context.Context, id string) (*models.ProgramTerm, []*models.Application, error) {
		return &models.ProgramTerm{ID: id, Status: models.ProgramTermStatusClosed}, []*models.Application{
			{ID: "a1", Role: models.ApplicationRoleMentee},
			{ID: "a2", Role: models.ApplicationRoleMentor},
		}, nil
	}}
	n := &stubNotifier{}
	_, declined, err := service.NewProgramTermService(termRepo, &stubAppRepo{}, n).Close(context.Background(), "term-1")
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	if declined != 2 || n.menteeDeclinedCalls != 1 {
		t.Fatalf("declined = %d, notifications = %d; want 2 declined and 1 mentee notified", declined, n.menteeDeclinedCalls)
	}
}

func TestProgramTermService_Close_ErrorSendsNothing(t *testing.T) {
	termRepo := &stubTermRepo{closeWithBulkDecline: func(context.Context, string) (*models.ProgramTerm, []*models.Application, error) {
		return nil, nil, domain.ErrStateLocked
	}}
	n := &stubNotifier{}
	if _, _, err := service.NewProgramTermService(termRepo, &stubAppRepo{}, n).Close(context.Background(), "term-1"); !errors.Is(err, domain.ErrStateLocked) {
		t.Fatalf("Close: %v; want ErrStateLocked", err)
	}
	if n.menteeDeclinedCalls != 0 {
		t.Fatalf("notifications = %d; want 0", n.menteeDeclinedCalls)
	}
}
