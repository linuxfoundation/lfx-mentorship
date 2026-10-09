// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build e2e

package main

import (
	"net/http"
	"strings"
	"testing"
)

// directoryFixture is a published program with an active mentor and an accepted mentee, both with profiles.
func directoryFixture(t *testing.T) (mentor, mentee *actor) {
	t.Helper()
	admin := signIn(t, "admin")
	p := publishProgram(t, admin, "Directory Program")
	mentor = signIn(t, "dir-mentor")
	mentor.mustJSON(t, http.MethodPut, "/me/profiles/mentor", map[string]any{
		"first_name": "Dir", "introduction": "I mentor Go", "skill_set": map[string]any{"skills": []string{"Go"}},
	}, http.StatusCreated, nil)
	inviteMentor(t, p, mentor)
	mentee = signIn(t, "dir-mentee")
	registerMentee(t, mentee)
	app := apply(t, mentee, p)
	admin.mustJSON(t, http.MethodPatch, "/applications/"+app.ID+"/status", map[string]any{"status": "accepted", "attendance_type": "full_time"}, http.StatusOK, nil)
	return mentor, mentee
}

func TestE2EPublicDirectories(t *testing.T) {
	reset(t)
	mentor, mentee := directoryFixture(t)
	anon := anonymous()

	t.Run("mentees", func(t *testing.T) {
		var page struct {
			Data []struct {
				UserID string   `json:"user_id"`
				Status string   `json:"status"`
				Skills []string `json:"skills"`
			} `json:"data"`
		}
		anon.mustJSON(t, http.MethodGet, "/mentees", nil, http.StatusOK, &page)
		if len(page.Data) != 1 || page.Data[0].UserID != mentee.ID || page.Data[0].Status != "accepted" || len(page.Data[0].Skills) != 2 {
			t.Fatalf("mentees: %+v", page)
		}
		for _, q := range []string{"status=active", "status=all", "skill=Go", "skill=all", "search=dir"} {
			anon.mustJSON(t, http.MethodGet, "/mentees?"+q, nil, http.StatusOK, &page)
			if len(page.Data) != 1 {
				t.Fatalf("mentees?%s: %+v", q, page)
			}
		}
		anon.mustJSON(t, http.MethodGet, "/mentees?status=graduated", nil, http.StatusOK, &page)
		if len(page.Data) != 0 {
			t.Fatalf("graduated: %+v", page)
		}
		anon.call(t, http.MethodGet, "/mentees?status=bogus", nil).expect(http.StatusBadRequest)
		anon.call(t, http.MethodGet, "/mentees?limit=x", nil).expect(http.StatusBadRequest)

		var summary struct {
			MenteeCount  int `json:"mentee_count"`
			ProgramCount int `json:"program_count"`
		}
		anon.mustJSON(t, http.MethodGet, "/mentees/summary", nil, http.StatusOK, &summary)
		if summary.MenteeCount != 1 || summary.ProgramCount != 1 {
			t.Fatalf("mentee summary: %+v", summary)
		}
		var detail struct {
			UserID string `json:"user_id"`
		}
		anon.mustJSON(t, http.MethodGet, "/mentees/"+mentee.ID, nil, http.StatusOK, &detail)
		if detail.UserID != mentee.ID {
			t.Fatalf("mentee detail: %+v", detail)
		}
		anon.call(t, http.MethodGet, "/mentees/not-a-uuid", nil).expect(http.StatusBadRequest)
		anon.call(t, http.MethodGet, "/mentees/00000000-0000-4000-8000-000000000000", nil).expect(http.StatusNotFound)
	})

	t.Run("mentors", func(t *testing.T) {
		var page struct {
			Data []struct {
				UserID string `json:"user_id"`
			} `json:"data"`
		}
		anon.mustJSON(t, http.MethodGet, "/mentors", nil, http.StatusOK, &page)
		if len(page.Data) != 1 || page.Data[0].UserID != mentor.ID {
			t.Fatalf("mentors: %+v", page)
		}
		anon.mustJSON(t, http.MethodGet, "/mentors?skill=all&search=dir", nil, http.StatusOK, &page)
		if len(page.Data) != 1 {
			t.Fatalf("filtered mentors: %+v", page)
		}
		anon.call(t, http.MethodGet, "/mentors?offset=x", nil).expect(http.StatusBadRequest)

		var summary struct {
			MentorCount  int `json:"mentor_count"`
			ProgramCount int `json:"program_count"`
		}
		anon.mustJSON(t, http.MethodGet, "/mentors/summary", nil, http.StatusOK, &summary)
		if summary.MentorCount != 1 || summary.ProgramCount != 1 {
			t.Fatalf("mentor summary: %+v", summary)
		}
		var detail struct {
			UserID         string `json:"user_id"`
			Programs       []any  `json:"programs"`
			CurrentMentees []any  `json:"current_mentees"`
		}
		anon.mustJSON(t, http.MethodGet, "/mentors/"+mentor.ID, nil, http.StatusOK, &detail)
		if detail.UserID != mentor.ID || len(detail.Programs) != 1 {
			t.Fatalf("mentor detail: %+v", detail)
		}
		anon.call(t, http.MethodGet, "/mentors/not-a-uuid", nil).expect(http.StatusBadRequest)
		anon.call(t, http.MethodGet, "/mentors/00000000-0000-4000-8000-000000000000", nil).expect(http.StatusNotFound)

		mentor.call(t, http.MethodGet, "/me/mentor-programs?limit=101", nil).expect(http.StatusBadRequest)
		mentor.call(t, http.MethodGet, "/me/mentor-programs?offset=-1", nil).expect(http.StatusBadRequest)
		mentor.call(t, http.MethodGet, "/me/mentor-programs?limit=x", nil).expect(http.StatusBadRequest)
	})

	t.Run("platform summary", func(t *testing.T) {
		var summary map[string]any
		anon.mustJSON(t, http.MethodGet, "/summary", nil, http.StatusOK, &summary)
		if len(summary) == 0 {
			t.Fatalf("empty platform summary")
		}
		res := anon.call(t, http.MethodGet, "/summary", nil).expect(http.StatusOK)
		if res.Header.Get("Cache-Control") == "" {
			t.Fatalf("public summary is not cacheable: %v", res.Header)
		}
	})
}

func TestE2EFunding(t *testing.T) {
	reset(t)
	admin := signIn(t, "admin")
	p := publishProgram(t, admin, "Funded Program")
	anon := anonymous()
	dbExec(t, `INSERT INTO program_funding_stats (id, program_id, amount_raised, amount_spent) VALUES (gen_random_uuid(), $1, 1500, 400)`, p.ID)

	t.Run("stats", func(t *testing.T) {
		var stats struct {
			AmountRaised float64 `json:"amount_raised"`
			AmountSpent  float64 `json:"amount_spent"`
		}
		anon.mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/funding-stats", nil, http.StatusOK, &stats)
		if stats.AmountRaised != 1500 || stats.AmountSpent != 400 {
			t.Fatalf("program stats: %+v", stats)
		}
		anon.mustJSON(t, http.MethodGet, "/funding-stats/total", nil, http.StatusOK, &stats)
		if stats.AmountRaised != 1500 {
			t.Fatalf("total stats: %+v", stats)
		}
		unfunded := publishProgram(t, admin, "Unfunded Program")
		anon.call(t, http.MethodGet, "/programs/"+unfunded.ID+"/funding-stats", nil).expect(http.StatusNotFound)
	})

	e2e.fakes.setTransactions(p.ID, []map[string]any{
		{"id": "t1", "type": "donation", "amount_cents": 5000, "date": "2026-01-01T00:00:00Z", "donor_type": "organization", "donor_name": "Acme", "donor_logo_url": "https://acme.example.org/logo.png"},
		{"id": "t2", "type": "donation", "amount_cents": 2500, "date": "2026-01-02T00:00:00Z", "donor_type": "Organization", "donor_name": "acme"},
		{"id": "t3", "type": "donation", "amount_cents": 1000, "date": "2026-01-03T00:00:00Z", "donor_type": "individual", "donor_name": "Ann"},
		{"id": "t4", "type": "donation", "amount_cents": 700, "date": "2026-01-04T00:00:00Z"},
		{"id": "t5", "type": "refund", "amount_cents": -300, "date": "2026-01-05T00:00:00Z", "donor_type": "organization", "donor_name": "Beta"},
	})

	t.Run("categorized transactions", func(t *testing.T) {
		var txns struct {
			Individual   []any `json:"individual_transactions"`
			Organization []any `json:"organization_transactions"`
		}
		anon.mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/transactions?limit=500&offset=-1&subscriptionOnly=true&categoryType=mentorship", nil, http.StatusOK, &txns)
		if len(txns.Individual) != 2 || len(txns.Organization) != 3 {
			t.Fatalf("transactions: %+v", txns)
		}
		anon.mustJSON(t, http.MethodGet, "/programs/"+p.Slug+"/transactions?limit=0", nil, http.StatusOK, &txns)
		anon.call(t, http.MethodGet, "/programs/"+p.ID+"/transactions?limit=x", nil).expect(http.StatusBadRequest)
	})

	t.Run("sponsors aggregate organizations and collapse individuals", func(t *testing.T) {
		var sponsors struct {
			Data []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				AmountCents int64  `json:"amount_cents"`
				LogoURL     string `json:"logo_url"`
			} `json:"data"`
		}
		for _, q := range []string{"", "?aggregate", "?aggregate=true", "?aggregate=no&categoryType=mentorship"} {
			anon.mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/sponsors"+q, nil, http.StatusOK, &sponsors)
			if len(sponsors.Data) != 2 || sponsors.Data[0].ID != "acme" || sponsors.Data[0].AmountCents != 7500 || sponsors.Data[0].LogoURL == "" ||
				sponsors.Data[1].ID != "individual-donors" || sponsors.Data[1].AmountCents != 1700 {
				t.Fatalf("sponsors%s: %+v", q, sponsors)
			}
		}
		other := publishProgram(t, admin, "No Sponsors Program")
		anon.mustJSON(t, http.MethodGet, "/programs/"+other.ID+"/sponsors", nil, http.StatusOK, &sponsors)
		if sponsors.Data == nil || len(sponsors.Data) != 0 {
			t.Fatalf("empty sponsors: %+v", sponsors)
		}
	})

	t.Run("crowdfunding outage is a 503", func(t *testing.T) {
		e2e.fakes.setCrowdfundingDown(true)
		anon.call(t, http.MethodGet, "/programs/"+p.ID+"/transactions", nil).expect(http.StatusServiceUnavailable)
		anon.call(t, http.MethodGet, "/programs/"+p.ID+"/sponsors", nil).expect(http.StatusServiceUnavailable)
	})

	t.Run("hidden programs keep their funding private", func(t *testing.T) {
		admin.mustJSON(t, http.MethodPost, "/programs/"+p.ID+"/hide", nil, http.StatusOK, nil)
		for _, path := range []string{"/funding-stats", "/transactions", "/sponsors", "/skills", "/header", "/members"} {
			anon.call(t, http.MethodGet, "/programs/"+p.ID+path, nil).expect(http.StatusNotFound)
		}
	})
}

func TestE2EApproverRoster(t *testing.T) {
	reset(t)
	manager := signIn(t, "roster-manager", "manage:mentorship:approvers")
	candidate := signIn(t, "approver-candidate")
	path := "/admin/approver-team/members"

	candidate.call(t, http.MethodGet, path, nil).expect(http.StatusForbidden)
	candidate.call(t, http.MethodPost, path, map[string]any{"user_id": manager.ID}).expect(http.StatusForbidden)
	anonymous().call(t, http.MethodGet, path, nil).expect(http.StatusUnauthorized)
	manager.call(t, http.MethodPost, path, map[string]any{"user_id": manager.ID}).expect(http.StatusForbidden)
	manager.send(t, http.MethodPost, path, strings.NewReader("{"), "application/json", nil).expect(http.StatusBadRequest)

	var added struct {
		UserID string `json:"user_id"`
		LFID   string `json:"lfid"`
	}
	manager.mustJSON(t, http.MethodPost, path, map[string]any{"user_id": candidate.ID}, http.StatusCreated, &added)
	if added.UserID != candidate.ID || added.LFID != "approver-candidate" {
		t.Fatalf("added: %+v", added)
	}
	var list struct {
		Data []struct {
			UserID string `json:"user_id"`
		} `json:"data"`
	}
	manager.mustJSON(t, http.MethodGet, path, nil, http.StatusOK, &list)
	if len(list.Data) != 1 || list.Data[0].UserID != candidate.ID {
		t.Fatalf("roster: %+v", list)
	}
	manager.call(t, http.MethodPost, path, map[string]any{"user_id": "00000000-0000-4000-8000-000000000000"}).expect(http.StatusUnprocessableEntity)

	manager.call(t, http.MethodDelete, path+"/"+manager.ID, nil).expect(http.StatusForbidden)
	candidate.call(t, http.MethodDelete, path+"/"+candidate.ID, nil).expect(http.StatusForbidden)
	manager.call(t, http.MethodDelete, path+"/"+candidate.ID, nil).expect(http.StatusNoContent)
	manager.mustJSON(t, http.MethodGet, path, nil, http.StatusOK, &list)
	if len(list.Data) != 0 {
		t.Fatalf("roster after removal: %+v", list)
	}
}

func TestE2EMachinePrincipals(t *testing.T) {
	reset(t)
	admin := signIn(t, "admin")
	p := publishProgram(t, admin, "M2M Program")
	bot := &actor{token: mintToken(t, validClaims("svc-bot@clients", "", ""))}

	// M2M callers skip local-user resolution, so routes keyed on "me" have no user to act for.
	bot.call(t, http.MethodGet, "/me/programs", nil).expect(http.StatusUnauthorized)
	bot.call(t, http.MethodGet, "/me/mentor-programs", nil).expect(http.StatusUnauthorized)
	bot.call(t, http.MethodGet, "/programs/"+p.ID+"/management-summary", nil).expect(http.StatusOK)
	bot.call(t, http.MethodGet, "/programs/"+p.ID, nil).expect(http.StatusOK)
}
