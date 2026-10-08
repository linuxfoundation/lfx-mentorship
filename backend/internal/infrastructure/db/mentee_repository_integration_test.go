// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

func TestMenteeRepositoryIntegration_ProfileShowsApplicantsDirectoryDoesNot(t *testing.T) {
	pool := integrationPool(t)
	seed := newMentoredSeed(t, pool)
	seed.program("prog", "Prog", "published")
	seed.term("prog", "term", "open", daysFromNow(-10))
	seed.program("unpublished", "Unpublished", "pending")
	seed.term("unpublished", "unpublished-term", "open", daysFromNow(-10))

	created := time.Now().Add(-time.Hour)
	for _, status := range []string{"accepted", "pending", "declined", "withdrawn", "hold"} {
		seed.user(status)
		seed.application(status+"-app", "term", status, "mentee", status, created)
	}
	seed.user("unpublished-applicant")
	seed.application("unpublished-app", "unpublished-term", "unpublished-applicant", "mentee", "pending", created)
	seed.user("mentor-applicant")
	seed.application("mentor-app", "term", "mentor-applicant", "mentor", "pending", created)

	repo := NewMenteeRepository(pool)
	ctx := context.Background()

	page, err := repo.List(ctx, models.MenteeFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].UserID != seed.id("accepted") || page.Meta.Total != 1 {
		t.Fatalf("directory = %d mentees (total %d); want only the accepted mentee", len(page.Data), page.Meta.Total)
	}
	summary, err := repo.Summary(ctx)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.MenteeCount != 1 || summary.ProgramCount != 1 {
		t.Errorf("summary = %+v; want 1 mentee in 1 program", summary)
	}

	for _, status := range []models.MenteeStatus{
		models.MenteeStatusAccepted, models.MenteeStatusPending, models.MenteeStatusDeclined, models.MenteeStatusWithdrawn,
	} {
		detail, err := repo.GetByUserID(ctx, seed.id(string(status)))
		if err != nil {
			t.Fatalf("GetByUserID(%s): %v", status, err)
		}
		if detail.Status != status || detail.Program == nil || detail.Program.ID != seed.id("prog") {
			t.Errorf("%s profile = status %q, program %+v; want status %q on Prog", status, detail.Status, detail.Program, status)
		}
		if len(detail.Programs) != 1 || detail.Programs[0].Status != status || len(detail.Programs[0].Terms) != 1 {
			t.Errorf("%s profile programs = %+v; want Prog with one %s term", status, detail.Programs, status)
		}
	}

	for _, label := range []string{"hold", "unpublished-applicant", "mentor-applicant"} {
		if _, err := repo.GetByUserID(ctx, seed.id(label)); !errors.Is(err, domain.ErrMenteeNotFound) {
			t.Errorf("GetByUserID(%s) err = %v; want ErrMenteeNotFound", label, err)
		}
	}
}

func TestMenteeRepositoryIntegration_ProfilePrefersAcceptedAndShowsReapplication(t *testing.T) {
	pool := integrationPool(t)
	seed := newMentoredSeed(t, pool)
	seed.user("mentee")
	seed.program("accepted-prog", "Accepted Prog", "published")
	seed.term("accepted-prog", "accepted-term", "open", daysFromNow(-30))
	seed.program("pending-prog", "Pending Prog", "published")
	seed.term("pending-prog", "pending-term", "open", daysFromNow(-5))
	seed.program("declined-prog", "Declined Prog", "published")
	seed.term("declined-prog", "declined-term", "closed", daysFromNow(-400))

	declinedOn := time.Now().Add(-365 * 24 * time.Hour)
	acceptedOn := time.Now().Add(-30 * 24 * time.Hour)
	seed.application("declined", "declined-term", "mentee", "mentee", "declined", declinedOn)
	seed.application("accepted", "accepted-term", "mentee", "mentee", "accepted", acceptedOn)
	seed.application("withdrawn", "pending-term", "mentee", "mentee", "withdrawn", time.Now().Add(-48*time.Hour))
	seed.application("reapplied", "pending-term", "mentee", "mentee", "pending", time.Now().Add(-24*time.Hour))

	detail, err := NewMenteeRepository(pool).GetByUserID(context.Background(), seed.id("mentee"))
	if err != nil {
		t.Fatalf("GetByUserID: %v", err)
	}
	if detail.Status != models.MenteeStatusAccepted || detail.Program == nil || detail.Program.ID != seed.id("accepted-prog") {
		t.Errorf("header = status %q, program %+v; want the accepted program featured", detail.Status, detail.Program)
	}
	if !detail.JoinedAt.Equal(acceptedOn.Truncate(time.Microsecond)) {
		t.Errorf("joined_at = %v; want the accepted application's %v, not the earlier declined one", detail.JoinedAt, acceptedOn)
	}

	want := map[string]models.MenteeStatus{
		seed.id("accepted-prog"): models.MenteeStatusAccepted,
		seed.id("pending-prog"):  models.MenteeStatusPending,
		seed.id("declined-prog"): models.MenteeStatusDeclined,
	}
	if len(detail.Programs) != len(want) {
		t.Fatalf("programs = %+v; want %d", detail.Programs, len(want))
	}
	for _, program := range detail.Programs {
		if program.Status != want[program.ID] || len(program.Terms) != 1 {
			t.Errorf("program %s = status %q with %d terms; want %q with one term", program.Name, program.Status, len(program.Terms), want[program.ID])
		}
	}
}
