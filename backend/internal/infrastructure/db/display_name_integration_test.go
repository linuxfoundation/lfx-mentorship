// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// TestDisplayNameIntegration_FallsBackToProfile covers every list that names a mentee or mentor:
// an account signed in without name or email claims is shown by what its profile holds.
func TestDisplayNameIntegration_FallsBackToProfile(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	const (
		menteeID    = "00000000-0000-0000-0000-0000000000a1"
		mentorID    = "00000000-0000-0000-0000-0000000000a2"
		menteeName  = "Ada Lovelace"
		mentorName  = "Grace Hopper"
		mentorEmail = "grace@example.org"
	)
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO users (id, lfid) VALUES ($1, 'bare-mentee'), ($2, 'bare-mentor')`, []any{menteeID, mentorID}},
		{`INSERT INTO user_profiles (id, user_id, profile_type, first_name, last_name) VALUES (gen_random_uuid(), $1, 'mentee', 'Ada', 'Lovelace')`, []any{menteeID}},
		{`INSERT INTO user_profiles (id, user_id, profile_type, first_name, last_name, email) VALUES (gen_random_uuid(), $1, 'mentor', 'Grace', 'Hopper', $2)`, []any{mentorID, mentorEmail}},
		{`INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ('00000000-0000-0000-0000-0000000000a3', $1, $2, 'mentee', 'accepted')`, []any{fixture.OpenTerm, menteeID}},
		{`INSERT INTO program_members (id, program_id, user_id, member_type, status) VALUES ('00000000-0000-0000-0000-0000000000a4', $1, $2, 'mentor', 'approved')`, []any{fixture.ProgramID, mentorID}},
	} {
		if _, err := pool.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Fatalf("seed %q: %v", stmt.sql, err)
		}
	}
	programs := NewProgramRepository(pool)

	mentees, err := programs.ListCatalogMentees(ctx, fixture.ProgramID)
	if err != nil || len(mentees) != 1 || deref(mentees[0].Name) != menteeName {
		t.Fatalf("ListCatalogMentees = %+v, err = %v; want one mentee named %q", mentees, err, menteeName)
	}

	catalog, err := programs.GetCatalog(ctx, fixture.ProgramID)
	if err != nil || len(catalog.Mentors) != 1 || deref(catalog.Mentors[0].Name) != mentorName {
		t.Fatalf("GetCatalog mentors = %+v, err = %v; want one mentor named %q", catalogMentors(catalog), err, mentorName)
	}

	mentee, err := NewMenteeRepository(pool).GetByUserID(ctx, menteeID)
	if err != nil || deref(mentee.Name) != menteeName || len(mentee.Programs) != 1 ||
		len(mentee.Programs[0].Mentors) != 1 || deref(mentee.Programs[0].Mentors[0].Name) != mentorName {
		t.Fatalf("mentee GetByUserID = %+v, err = %v; want %q with program mentor %q", mentee, err, menteeName, mentorName)
	}

	mentor, err := NewMentorRepository(pool).GetByUserID(ctx, mentorID)
	if err != nil || len(mentor.Programs) != 1 || len(mentor.Programs[0].Mentors) != 1 || deref(mentor.Programs[0].Mentors[0].Name) != mentorName {
		t.Fatalf("mentor GetByUserID = %+v, err = %v; want program mentor %q", mentor, err, mentorName)
	}

	rows, meta, err := NewProgramMemberRepository(pool).ListMentorManagement(ctx, fixture.ProgramID, models.ProgramMemberFilter{Search: "hopper"})
	if err != nil || meta.Total != 1 || len(rows) != 1 {
		t.Fatalf("ListMentorManagement(search) = %d rows, meta %+v, err = %v; want one", len(rows), meta, err)
	}
	if row := rows[0]; deref(row.Name) != mentorName || deref(row.Email) != mentorEmail || !row.ProfileCreated {
		t.Fatalf("ListMentorManagement row = name %q email %q profile %v; want %q %q true", deref(row.Name), deref(row.Email), row.ProfileCreated, mentorName, mentorEmail)
	}
}

func catalogMentors(item *models.ProgramCatalogItem) []models.ProgramCatalogMentor {
	if item == nil {
		return nil
	}
	return item.Mentors
}
