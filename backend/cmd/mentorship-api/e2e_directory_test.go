// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build e2e

package main

import (
	"context"
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
		anon.mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/transactions?limit=0", nil, http.StatusOK, &txns)
		anon.call(t, http.MethodGet, "/programs/"+p.ID+"/transactions?limit=x", nil).expect(http.StatusBadRequest)
	})

	type sponsorsBody struct {
		Data []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			AmountCents int64  `json:"amount_cents"`
			LogoURL     string `json:"logo_url"`
		} `json:"data"`
		CrowdfundingInitiative bool `json:"crowdfunding_initiative"`
	}

	t.Run("sponsors aggregate organizations and collapse individuals", func(t *testing.T) {
		for _, q := range []string{"", "?aggregate", "?aggregate=true", "?aggregate=no&categoryType=mentorship"} {
			var sponsors sponsorsBody
			anon.mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/sponsors"+q, nil, http.StatusOK, &sponsors)
			if len(sponsors.Data) != 2 || sponsors.Data[0].ID != "acme" || sponsors.Data[0].AmountCents != 7500 || sponsors.Data[0].LogoURL == "" ||
				sponsors.Data[1].ID != "individual-donors" || sponsors.Data[1].AmountCents != 1700 || !sponsors.CrowdfundingInitiative {
				t.Fatalf("sponsors%s: %+v", q, sponsors)
			}
		}
		other := publishProgram(t, admin, "No Sponsors Program")
		e2e.fakes.setTransactions(other.ID, []map[string]any{})
		var sponsors sponsorsBody
		anon.mustJSON(t, http.MethodGet, "/programs/"+other.ID+"/sponsors", nil, http.StatusOK, &sponsors)
		if sponsors.Data == nil || len(sponsors.Data) != 0 || !sponsors.CrowdfundingInitiative {
			t.Fatalf("empty sponsors: %+v", sponsors)
		}
	})

	t.Run("a program without a crowdfunding initiative has no sponsors", func(t *testing.T) {
		uninitiated := publishProgram(t, admin, "No Initiative Program")
		var sponsors sponsorsBody
		anon.mustJSON(t, http.MethodGet, "/programs/"+uninitiated.ID+"/sponsors?aggregate=true", nil, http.StatusOK, &sponsors)
		if sponsors.Data == nil || len(sponsors.Data) != 0 || sponsors.CrowdfundingInitiative {
			t.Fatalf("sponsors without initiative: %+v", sponsors)
		}
		anon.call(t, http.MethodGet, "/programs/"+uninitiated.ID+"/transactions", nil).expect(http.StatusNotFound)
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

// A migrated user keeps their legacy photo in users.avatar_url, while a picture
// saved on a mentorship page lands in the role profile's logo_url, which must win;
// with no profile logo (NULL or empty) the legacy photo must still show.
func TestE2EProfilePicture(t *testing.T) {
	const mentorAvatar, menteeAvatar = "https://legacy.example.org/mentor.png", "https://legacy.example.org/mentee.png"
	const mentorLogo, menteeLogo = "https://cdn.example.org/mentor.png", "https://cdn.example.org/mentee.png"
	logo := func(s string) *string { return &s }
	for _, tc := range []struct {
		name                   string
		mentorLogo, menteeLogo *string // nil stores NULL
		mentorWant, mentorNot  string
		menteeWant, menteeNot  string
	}{
		{"profile logo wins", logo(mentorLogo), logo(menteeLogo), mentorLogo, mentorAvatar, menteeLogo, menteeAvatar},
		{"legacy avatar when the profile has none", logo(""), nil, mentorAvatar, mentorLogo, menteeAvatar, menteeLogo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reset(t)
			mentor, mentee := directoryFixture(t)
			for _, p := range []struct {
				id, profileType, avatar string
				logo                    *string
			}{{mentor.ID, "mentor", mentorAvatar, tc.mentorLogo}, {mentee.ID, "mentee", menteeAvatar, tc.menteeLogo}} {
				if _, err := e2e.pool.Exec(context.Background(), `UPDATE users SET avatar_url = $2 WHERE id = $1`, p.id, p.avatar); err != nil {
					t.Fatal(err)
				}
				if _, err := e2e.pool.Exec(context.Background(), `UPDATE user_profiles SET logo_url = $3 WHERE user_id = $1 AND profile_type = $2`, p.id, p.profileType, p.logo); err != nil {
					t.Fatal(err)
				}
			}
			var mine struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			mentor.mustJSON(t, http.MethodGet, "/me/mentor-programs", nil, http.StatusOK, &mine)
			if len(mine.Data) != 1 {
				t.Fatalf("mentor programs: %+v", mine)
			}
			programID := mine.Data[0].ID
			admin := signIn(t, "admin")

			check := func(path string, body []byte, showsMentor, showsMentee bool) {
				t.Helper()
				for _, c := range []struct {
					shown     bool
					want, not string
				}{{showsMentor, tc.mentorWant, tc.mentorNot}, {showsMentee, tc.menteeWant, tc.menteeNot}} {
					if c.shown && !strings.Contains(string(body), c.want) {
						t.Errorf("%s does not show %s: %s", path, c.want, body)
					}
					if strings.Contains(string(body), c.not) {
						t.Errorf("%s shows %s: %s", path, c.not, body)
					}
				}
			}
			for _, r := range []struct {
				caller                   *actor
				path                     string
				showsMentor, showsMentee bool
			}{
				{anonymous(), "/mentors", true, false},
				{anonymous(), "/mentors/" + mentor.ID, true, true},
				{anonymous(), "/mentees", false, true},
				{anonymous(), "/mentees/" + mentee.ID, true, true},
				{anonymous(), "/programs/catalog", true, false},
				{anonymous(), "/programs/" + programID + "/mentees", false, true},
				{admin, "/programs/" + programID + "/applications", false, true},
				{admin, "/programs/" + programID + "/member-management", true, false},
			} {
				check(r.path, r.caller.call(t, http.MethodGet, r.path, nil).expect(http.StatusOK).Body, r.showsMentor, r.showsMentee)
			}

			// The platform summary only previews graduated mentees.
			if _, err := e2e.pool.Exec(context.Background(), `UPDATE applications SET status = 'graduated' WHERE user_id = $1`, mentee.ID); err != nil {
				t.Fatal(err)
			}
			check("/summary", anonymous().call(t, http.MethodGet, "/summary", nil).expect(http.StatusOK).Body, false, true)
		})
	}
}
