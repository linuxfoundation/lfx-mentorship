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

func TestMenteeRepositoryIntegration_ProfileResolvesForApplicantsWithoutTheirStatus(t *testing.T) {
	pool := integrationPool(t)
	seed := newMentoredSeed(t, pool)
	seed.program("prog", "Prog", "published")
	seed.term("prog", "term", "open", daysFromNow(-10))
	seed.program("unpublished", "Unpublished", "pending")
	seed.term("unpublished", "unpublished-term", "open", daysFromNow(-10))

	created := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	for _, status := range []string{"accepted", "pending", "declined", "withdrawn"} {
		seed.user(status)
		seed.application(status+"-app", "term", status, "mentee", status, created)
	}
	seed.user("unpublished-applicant")
	seed.application("unpublished-app", "unpublished-term", "unpublished-applicant", "mentee", "pending", created)
	seed.user("mentor-applicant")
	seed.application("mentor-app", "term", "mentor-applicant", "mentor", "pending", created)
	seed.user("never-applied")

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

	accepted, err := repo.GetByUserID(ctx, seed.id("accepted"))
	if err != nil {
		t.Fatalf("GetByUserID(accepted): %v", err)
	}
	if accepted.Status != models.MenteeStatusAccepted || accepted.Program == nil || len(accepted.Programs) != 1 {
		t.Errorf("accepted profile = status %q, program %+v, %d programs; want accepted on Prog", accepted.Status, accepted.Program, len(accepted.Programs))
	}

	for _, label := range []string{"pending", "declined", "withdrawn"} {
		detail, err := repo.GetByUserID(ctx, seed.id(label))
		if err != nil {
			t.Fatalf("GetByUserID(%s): %v", label, err)
		}
		if detail.Status != "" || detail.Program != nil || len(detail.Programs) != 0 {
			t.Errorf("%s profile = status %q, program %+v, %d programs; want no status or programs", label, detail.Status, detail.Program, len(detail.Programs))
		}
		if detail.Name == nil || *detail.Name != label || !detail.JoinedAt.Equal(created) {
			t.Errorf("%s profile = name %v, joined_at %v; want its name and first application date %v", label, detail.Name, detail.JoinedAt, created)
		}
	}

	for _, label := range []string{"unpublished-applicant", "mentor-applicant", "never-applied"} {
		if _, err := repo.GetByUserID(ctx, seed.id(label)); !errors.Is(err, domain.ErrMenteeNotFound) {
			t.Errorf("GetByUserID(%s) err = %v; want ErrMenteeNotFound", label, err)
		}
	}
}

func TestMenteeRepositoryIntegration_ProfileShowsOnlyAcceptedAndGraduatedPrograms(t *testing.T) {
	pool := integrationPool(t)
	seed := newMentoredSeed(t, pool)
	seed.user("mentee")
	seed.program("accepted-prog", "Accepted Prog", "published")
	seed.term("accepted-prog", "accepted-term", "open", daysFromNow(-30))
	seed.program("pending-prog", "Pending Prog", "published")
	seed.term("pending-prog", "pending-term", "open", daysFromNow(-5))
	seed.program("declined-prog", "Declined Prog", "published")
	seed.term("declined-prog", "declined-term", "closed", daysFromNow(-400))

	acceptedOn := time.Now().Add(-30 * 24 * time.Hour).Truncate(time.Microsecond)
	seed.application("declined", "declined-term", "mentee", "mentee", "declined", time.Now().Add(-365*24*time.Hour))
	seed.application("accepted", "accepted-term", "mentee", "mentee", "accepted", acceptedOn)
	seed.application("pending", "pending-term", "mentee", "mentee", "pending", time.Now().Add(-24*time.Hour))

	detail, err := NewMenteeRepository(pool).GetByUserID(context.Background(), seed.id("mentee"))
	if err != nil {
		t.Fatalf("GetByUserID: %v", err)
	}
	if detail.Status != models.MenteeStatusAccepted || detail.Program == nil || detail.Program.ID != seed.id("accepted-prog") {
		t.Errorf("header = status %q, program %+v; want the accepted program", detail.Status, detail.Program)
	}
	if !detail.JoinedAt.Equal(acceptedOn) {
		t.Errorf("joined_at = %v; want the accepted application's %v, not the earlier declined one", detail.JoinedAt, acceptedOn)
	}
	if len(detail.Programs) != 1 || detail.Programs[0].ID != seed.id("accepted-prog") {
		t.Errorf("programs = %+v; want only the accepted program", detail.Programs)
	}
}
