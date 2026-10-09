// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build e2e

package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

type profile struct {
	ID           string          `json:"id"`
	UserID       string          `json:"user_id"`
	ProfileType  string          `json:"profile_type"`
	FirstName    string          `json:"first_name"`
	Introduction string          `json:"introduction"`
	SkillSet     json.RawMessage `json:"skill_set"`
	ProfileLinks json.RawMessage `json:"profile_links"`
}

// menteeProfileBody is a complete mentee registration.
func menteeProfileBody(first string) map[string]any {
	return map[string]any{
		"age_eligible": true, "work_eligible": true, "terms_and_conditions": true,
		"first_name": first, "last_name": "Mentee", "introduction": "Hi, I am " + first,
		"address":       map[string]any{"country": "KE"},
		"skill_set":     map[string]any{"skills": []string{"Go", "Rust"}},
		"profile_links": map[string]any{"github": "https://github.com/" + first},
	}
}

// registerMentee creates the actor's mentee profile.
func registerMentee(t *testing.T, a *actor) profile {
	t.Helper()
	var p profile
	a.mustJSON(http.MethodPut, "/me/profiles/mentee", menteeProfileBody(a.LFID), http.StatusCreated, &p)
	return p
}

func TestE2EUserProfiles(t *testing.T) {
	reset(t)
	ada := signIn(t, "ada")

	t.Run("mentee eligibility gates", func(t *testing.T) {
		body := menteeProfileBody("ada")
		body["age_eligible"] = false
		ada.call(http.MethodPut, "/me/profiles/mentee", body).expect(http.StatusUnprocessableEntity)
		body = menteeProfileBody("ada")
		body["work_eligible"] = false
		ada.call(http.MethodPut, "/me/profiles/mentee", body).expect(http.StatusUnprocessableEntity)
	})

	t.Run("body validation", func(t *testing.T) {
		ada.call(http.MethodPut, "/me/profiles/owner", menteeProfileBody("ada")).expect(http.StatusBadRequest)
		body := menteeProfileBody("ada")
		body["skill_set"] = map[string]any{"skills": []any{"Go", ""}}
		ada.call(http.MethodPut, "/me/profiles/mentee", body).expect(http.StatusBadRequest)
		body["skill_set"] = []string{"Go"}
		ada.call(http.MethodPut, "/me/profiles/mentee", body).expect(http.StatusBadRequest)
		body["skill_set"] = map[string]any{"improvementSkills": "Go"}
		ada.call(http.MethodPut, "/me/profiles/mentee", body).expect(http.StatusBadRequest)
		body = menteeProfileBody("ada")
		body["profile_links"] = []string{"x"}
		ada.call(http.MethodPut, "/me/profiles/mentee", body).expect(http.StatusBadRequest)
		ada.call(http.MethodPost, "/me/profiles", map[string]any{"first_name": "x"}).expect(http.StatusBadRequest)
		ada.call(http.MethodPost, "/me/profiles", map[string]any{"profile_type": "owner"}).expect(http.StatusBadRequest)
		anonymous(t).call(http.MethodPut, "/me/profiles/mentee", menteeProfileBody("ada")).expect(http.StatusUnauthorized)
	})

	created := registerMentee(t, ada)
	if created.UserID != ada.ID || created.ProfileType != "mentee" || created.FirstName != "ada" {
		t.Fatalf("created: %+v", created)
	}

	t.Run("PUT replaces the profile of that type", func(t *testing.T) {
		body := menteeProfileBody("ada")
		body["introduction"] = "Updated intro"
		// A legacy resume link is dropped rather than rejected.
		body["profile_links"] = map[string]any{"github": "https://github.com/ada", "resumeLink": "https://legacy.example.org/cv.pdf"}
		var replaced profile
		ada.mustJSON(http.MethodPut, "/me/profiles/mentee", body, http.StatusOK, &replaced)
		if replaced.ID != created.ID || replaced.Introduction != "Updated intro" {
			t.Fatalf("replaced: %+v", replaced)
		}
		var links map[string]any
		if err := json.Unmarshal(replaced.ProfileLinks, &links); err != nil || links["resumeLink"] != nil || links["github"] == nil {
			t.Fatalf("profile links: %s", replaced.ProfileLinks)
		}
	})

	t.Run("POST refuses a second mentee profile", func(t *testing.T) {
		body := menteeProfileBody("ada")
		body["profile_type"] = "mentee"
		ada.call(http.MethodPost, "/me/profiles", body).expect(http.StatusUnprocessableEntity)
	})

	var mentor profile
	ada.mustJSON(http.MethodPost, "/me/profiles", map[string]any{
		"profile_type": "mentor", "first_name": "Ada", "user_id": "someone-else",
		"skill_set": map[string]any{"skills": []string{"Go"}},
	}, http.StatusCreated, &mentor)
	if mentor.UserID != ada.ID {
		t.Fatalf("profile owner taken from body: %+v", mentor)
	}

	t.Run("read", func(t *testing.T) {
		var list struct {
			Data []profile `json:"data"`
			Meta struct {
				Total int `json:"total"`
			} `json:"meta"`
		}
		ada.mustJSON(http.MethodGet, "/me/profiles", nil, http.StatusOK, &list)
		if list.Meta.Total != 2 {
			t.Fatalf("list: %+v", list)
		}
		ada.mustJSON(http.MethodGet, "/me/profiles?profile_type=mentor", nil, http.StatusOK, &list)
		if len(list.Data) != 1 || list.Data[0].ID != mentor.ID {
			t.Fatalf("filtered list: %+v", list)
		}
		ada.call(http.MethodGet, "/me/profiles?limit=x", nil).expect(http.StatusBadRequest)
		var got profile
		ada.mustJSON(http.MethodGet, "/me/profiles/mentee", nil, http.StatusOK, &got)
		if got.ID != created.ID {
			t.Fatalf("get by type: %+v", got)
		}
		signIn(t, "grace").call(http.MethodGet, "/me/profiles/mentee", nil).expect(http.StatusNotFound)
	})

	t.Run("update by type and by ID", func(t *testing.T) {
		var updated profile
		ada.mustJSON(http.MethodPatch, "/me/profiles/mentor", map[string]any{"introduction": "Mentor intro"}, http.StatusOK, &updated)
		if updated.Introduction != "Mentor intro" {
			t.Fatalf("patch by type: %+v", updated)
		}
		ada.mustJSON(http.MethodPatch, "/me/profiles/by-id/"+mentor.ID, map[string]any{"first_name": "Augusta"}, http.StatusOK, &updated)
		if updated.FirstName != "Augusta" {
			t.Fatalf("patch by id: %+v", updated)
		}
		ada.call(http.MethodPatch, "/me/profiles/mentor", map[string]any{"skill_set": map[string]any{"skills": []any{1}}}).expect(http.StatusBadRequest)
		ada.call(http.MethodPatch, "/me/profiles/by-id/"+mentor.ID, map[string]any{"profile_links": "x"}).expect(http.StatusBadRequest)
		ada.call(http.MethodPatch, "/me/profiles/by-id/"+mentor.ID, "{").expect(http.StatusBadRequest)
	})

	t.Run("another user cannot touch the profile by ID", func(t *testing.T) {
		grace := signIn(t, "grace")
		grace.call(http.MethodPatch, "/me/profiles/by-id/"+mentor.ID, map[string]any{"first_name": "x"}).expect(http.StatusForbidden)
		grace.call(http.MethodDelete, "/me/profiles/by-id/"+mentor.ID, nil).expect(http.StatusForbidden)
		grace.call(http.MethodPatch, "/me/profiles/by-id/00000000-0000-4000-8000-000000000000", map[string]any{"first_name": "x"}).expect(http.StatusNotFound)
		grace.call(http.MethodDelete, "/me/profiles/by-id/00000000-0000-4000-8000-000000000000", nil).expect(http.StatusNotFound)
		grace.call(http.MethodPatch, "/me/profiles/mentor", map[string]any{"first_name": "x"}).expect(http.StatusNotFound)
	})

	t.Run("delete by ID and by type", func(t *testing.T) {
		ada.call(http.MethodDelete, "/me/profiles/by-id/"+mentor.ID, nil).expect(http.StatusNoContent)
		ada.call(http.MethodDelete, "/me/profiles/mentee", nil).expect(http.StatusNoContent)
		ada.call(http.MethodDelete, "/me/profiles/mentee", nil).expect(http.StatusNotFound)
		var list struct {
			Data []profile `json:"data"`
		}
		ada.mustJSON(http.MethodGet, "/me/profiles", nil, http.StatusOK, &list)
		if len(list.Data) != 0 {
			t.Fatalf("profiles remain: %+v", list)
		}
	})
}
