// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build e2e

package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type programPage struct {
	Data []struct {
		ID     string   `json:"id"`
		Slug   string   `json:"slug"`
		Name   string   `json:"name"`
		Status string   `json:"status"`
		Skills []string `json:"skills"`
		Terms  []struct {
			ID             string `json:"id"`
			DiscoveryLabel string `json:"discovery_label"`
		} `json:"terms"`
		AdminStatus string `json:"admin_status"`
	} `json:"data"`
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
}

func (p programPage) ids() []string {
	out := make([]string, 0, len(p.Data))
	for _, d := range p.Data {
		out = append(out, d.ID)
	}
	return out
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestE2EProgramLifecycle(t *testing.T) {
	reset(t)
	admin := signIn(t, "admin")
	uid := newProject(t, "lifecycle")

	var created struct {
		ID          string `json:"id"`
		Slug        string `json:"slug"`
		Status      string `json:"status"`
		ProjectUID  string `json:"project_uid"`
		ProjectSlug string `json:"project_slug"`
		ProjectName string `json:"project_name"`
		ProjectLogo string `json:"project_logo_url"`
	}
	body := enrollmentBody("Lifecycle Program", uid)
	// Project Service is the source of the project metadata, not the caller.
	body["projectSlug"], body["projectName"] = "spoofed", "Spoofed"
	admin.mustJSON(t, http.MethodPost, "/programs", body, http.StatusCreated, &created)
	if created.Status != "pending" || created.Slug != "lifecycle-program" || created.ProjectUID != uid ||
		created.ProjectSlug != "lifecycle" || created.ProjectName != "Lifecycle" || !strings.HasSuffix(created.ProjectLogo, "/lifecycle.png") {
		t.Fatalf("created program: %+v", created)
	}
	id := created.ID

	t.Run("creator becomes an active program admin", func(t *testing.T) {
		var mine programPage
		admin.mustJSON(t, http.MethodGet, "/me/programs", nil, http.StatusOK, &mine)
		if len(mine.Data) != 1 || mine.Data[0].ID != id || mine.Data[0].AdminStatus != "pending_review" {
			t.Fatalf("my programs: %+v", mine)
		}
	})

	t.Run("a pending program is readable but not listed", func(t *testing.T) {
		anonymous().call(t, http.MethodGet, "/programs/"+id, nil).expect(http.StatusOK)
		anonymous().call(t, http.MethodGet, "/programs/resolve/lifecycle-program", nil).expect(http.StatusNotFound)
		var list programPage
		anonymous().mustJSON(t, http.MethodGet, "/programs", nil, http.StatusOK, &list)
		if contains(list.ids(), id) {
			t.Fatalf("pending program is listed publicly")
		}
	})

	t.Run("submission requires a logo", func(t *testing.T) {
		res := admin.call(t, http.MethodPost, "/programs/"+id+"/submit", nil).expect(http.StatusConflict)
		if !strings.Contains(res.errorMessage(), "logo is required") {
			t.Fatalf("got %q", res.errorMessage())
		}
	})

	t.Run("a pending program cannot be decided", func(t *testing.T) {
		approver(t).call(t, http.MethodPost, "/programs/"+id+"/decision", map[string]any{"status": "published"}).expect(http.StatusConflict)
	})

	var logo struct {
		PublicURL   string `json:"public_url"`
		ContentType string `json:"content_type"`
	}
	uploadLogo(t, admin, id).expect(http.StatusCreated).decode(&logo)
	if !strings.HasPrefix(logo.PublicURL, e2eCDNPrefix+"/") || logo.ContentType != "image/png" {
		t.Fatalf("logo upload: %+v", logo)
	}

	var submitted program
	admin.mustJSON(t, http.MethodPost, "/programs/"+id+"/submit", nil, http.StatusOK, &submitted)
	if submitted.Status != "submitted" {
		t.Fatalf("after submit: %q", submitted.Status)
	}

	t.Run("a submitted program is hidden from anonymous callers", func(t *testing.T) {
		anonymous().call(t, http.MethodGet, "/programs/"+id, nil).expect(http.StatusNotFound)
		anonymous().call(t, http.MethodGet, "/programs/"+id+"/catalog", nil).expect(http.StatusNotFound)
		admin.call(t, http.MethodGet, "/programs/"+id, nil).expect(http.StatusOK)
	})

	t.Run("decision validates its input", func(t *testing.T) {
		approver(t).call(t, http.MethodPost, "/programs/"+id+"/decision", map[string]any{}).expect(http.StatusBadRequest)
		approver(t).call(t, http.MethodPost, "/programs/"+id+"/decision", map[string]any{"status": "archived"}).expect(http.StatusBadRequest)
		anonymous().call(t, http.MethodPost, "/programs/"+id+"/decision", map[string]any{"status": "published"}).expect(http.StatusUnauthorized)
	})

	var published program
	approver(t).mustJSON(t, http.MethodPost, "/programs/"+id+"/decision", map[string]any{"status": "published"}, http.StatusOK, &published)
	if published.Status != "published" {
		t.Fatalf("after decision: %q", published.Status)
	}
	approver(t).call(t, http.MethodPost, "/programs/"+id+"/decision", map[string]any{"status": "rejected"}).expect(http.StatusConflict)

	t.Run("a published program is listed and resolvable", func(t *testing.T) {
		var resolved struct {
			ID string `json:"id"`
		}
		anonymous().mustJSON(t, http.MethodGet, "/programs/resolve/lifecycle-program", nil, http.StatusOK, &resolved)
		if resolved.ID != id {
			t.Fatalf("resolve: %q", resolved.ID)
		}
		anonymous().mustJSON(t, http.MethodGet, "/programs/resolve/"+id, nil, http.StatusOK, &resolved)
		anonymous().call(t, http.MethodGet, "/programs/"+id, nil).expect(http.StatusOK)
		var list programPage
		anonymous().mustJSON(t, http.MethodGet, "/programs?search=Lifecycle", nil, http.StatusOK, &list)
		if !contains(list.ids(), id) || list.Meta.Total != 1 {
			t.Fatalf("public list: %+v", list)
		}
	})

	t.Run("hide and unhide", func(t *testing.T) {
		var hidden program
		admin.mustJSON(t, http.MethodPost, "/programs/"+id+"/hide", nil, http.StatusOK, &hidden)
		if hidden.Status != "hidden" {
			t.Fatalf("after hide: %q", hidden.Status)
		}
		anonymous().call(t, http.MethodGet, "/programs/"+id, nil).expect(http.StatusNotFound)
		anonymous().call(t, http.MethodGet, "/programs/"+id+"/terms", nil).expect(http.StatusNotFound)
		admin.call(t, http.MethodGet, "/programs/"+id, nil).expect(http.StatusOK)
		admin.call(t, http.MethodPost, "/programs/"+id+"/hide", nil).expect(http.StatusConflict)

		var mine programPage
		admin.mustJSON(t, http.MethodGet, "/me/programs?status=hidden", nil, http.StatusOK, &mine)
		if len(mine.Data) != 1 {
			t.Fatalf("hidden filter: %+v", mine)
		}

		var shown program
		admin.mustJSON(t, http.MethodPost, "/programs/"+id+"/unhide", nil, http.StatusOK, &shown)
		if shown.Status != "published" {
			t.Fatalf("after unhide: %q", shown.Status)
		}
		admin.call(t, http.MethodPost, "/programs/"+id+"/unhide", nil).expect(http.StatusConflict)
		anonymous().call(t, http.MethodPost, "/programs/"+id+"/hide", nil).expect(http.StatusUnauthorized)
	})

	t.Run("delete removes the program", func(t *testing.T) {
		admin.call(t, http.MethodDelete, "/programs/"+id, nil).expect(http.StatusNoContent)
		admin.call(t, http.MethodGet, "/programs/"+id, nil).expect(http.StatusNotFound)
		admin.call(t, http.MethodDelete, "/programs/"+id, nil).expect(http.StatusNotFound)
		if dbCount(t, "SELECT count(*) FROM object_deletions WHERE bucket = 'logos'") == 0 && len(s3Keys(t, e2eLogosBucket)) != 0 {
			t.Fatalf("logo neither queued for deletion nor deleted")
		}
	})
}

func TestE2EProgramRejectionAndResubmission(t *testing.T) {
	reset(t)
	admin := signIn(t, "admin")
	p := createProgram(t, admin, "Rejected Program")
	uploadLogo(t, admin, p.ID).expect(http.StatusCreated)
	admin.mustJSON(t, http.MethodPost, "/programs/"+p.ID+"/submit", nil, http.StatusOK, nil)

	var rejected program
	approver(t).mustJSON(t, http.MethodPost, "/programs/"+p.ID+"/decision", map[string]any{"status": "rejected"}, http.StatusOK, &rejected)
	if rejected.Status != "rejected" {
		t.Fatalf("after reject: %q", rejected.Status)
	}
	var mine programPage
	admin.mustJSON(t, http.MethodGet, "/me/programs?status=rejected", nil, http.StatusOK, &mine)
	if len(mine.Data) != 1 || mine.Data[0].AdminStatus != "rejected" {
		t.Fatalf("rejected filter: %+v", mine)
	}

	// A rejected program goes straight back to submitted.
	var resubmitted program
	admin.mustJSON(t, http.MethodPost, "/programs/"+p.ID+"/submit", nil, http.StatusOK, &resubmitted)
	if resubmitted.Status != "submitted" {
		t.Fatalf("after resubmit: %q", resubmitted.Status)
	}
	admin.call(t, http.MethodPost, "/programs/"+p.ID+"/hide", nil).expect(http.StatusConflict)
}

func TestE2EProgramCreateValidation(t *testing.T) {
	reset(t)
	admin := signIn(t, "admin")
	uid := newProject(t, "validation")
	admin.mustJSON(t, http.MethodPost, "/programs", enrollmentBody("Taken Name", uid), http.StatusCreated, nil)

	cases := []struct {
		name   string
		mutate func(map[string]any)
		status int
	}{
		{"terms not accepted", func(b map[string]any) { b["termsAccepted"] = false }, http.StatusBadRequest},
		{"no skills", func(b map[string]any) { b["skills"] = []string{" ", ""} }, http.StatusBadRequest},
		{"no terms", func(b map[string]any) { b["terms"] = []any{} }, http.StatusBadRequest},
		{"too many terms", func(b map[string]any) {
			b["terms"] = []any{openTermBody("a"), openTermBody("b"), openTermBody("c"), openTermBody("d"), openTermBody("e")}
		}, http.StatusBadRequest},
		{"term without a name", func(b map[string]any) { b["terms"] = []any{openTermBody(" ")} }, http.StatusBadRequest},
		{"unparseable date", func(b map[string]any) {
			term := openTermBody("x")
			term["startDate"] = "next week"
			b["terms"] = []any{term}
		}, http.StatusBadRequest},
		{"end before start", func(b map[string]any) {
			term := openTermBody("x")
			term["endDate"] = day(19)
			b["terms"] = []any{term}
		}, http.StatusBadRequest},
		{"application window after the term starts", func(b map[string]any) {
			term := openTermBody("x")
			term["applicationEndDate"] = day(30)
			b["terms"] = []any{term}
		}, http.StatusBadRequest},
		{"required prerequisite without description", func(b map[string]any) {
			b["prerequisites"] = []any{map[string]any{"name": "x", "required": true}}
		}, http.StatusBadRequest},
		{"prerequisite with a bad due date", func(b map[string]any) {
			b["prerequisites"] = []any{map[string]any{"name": "x", "description": "y", "required": true, "dueDate": "soon"}}
		}, http.StatusBadRequest},
		{"prerequisites that are not a list", func(b map[string]any) { b["prerequisites"] = "nope" }, http.StatusBadRequest},
		{"name in use", func(b map[string]any) { b["name"] = "Taken Name" }, http.StatusConflict},
		{"repository URL is not http", func(b map[string]any) { b["repositoryUrl"] = "ftp://example.org/repo" }, http.StatusBadRequest},
		{"project ID is not a UUID", func(b map[string]any) { b["projectId"] = "not-a-uuid" }, http.StatusBadRequest},
		{"project unknown to Project Service", func(b map[string]any) { b["projectId"] = "00000000-0000-4000-8000-000000000000" }, http.StatusBadRequest},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := enrollmentBody("Validation "+string(rune('A'+i)), uid)
			tc.mutate(body)
			admin.call(t, http.MethodPost, "/programs", body).expect(tc.status)
		})
	}

	t.Run("anonymous caller cannot create", func(t *testing.T) {
		anonymous().call(t, http.MethodPost, "/programs", enrollmentBody("Anon", uid)).expect(http.StatusUnauthorized)
	})
	t.Run("malformed JSON", func(t *testing.T) {
		admin.send(t, http.MethodPost, "/programs", strings.NewReader("{"), "application/json", nil).expect(http.StatusBadRequest)
	})
	t.Run("name availability", func(t *testing.T) {
		var avail struct {
			Available bool `json:"available"`
		}
		admin.mustJSON(t, http.MethodGet, "/programs/name-availability?name="+url.QueryEscape("taken name"), nil, http.StatusOK, &avail)
		if avail.Available {
			t.Fatalf("taken name reported available")
		}
		admin.mustJSON(t, http.MethodGet, "/programs/name-availability?name=Fresh", nil, http.StatusOK, &avail)
		if !avail.Available {
			t.Fatalf("fresh name reported taken")
		}
		admin.call(t, http.MethodGet, "/programs/name-availability?name=", nil).expect(http.StatusBadRequest)
		anonymous().call(t, http.MethodGet, "/programs/name-availability?name=Fresh", nil).expect(http.StatusUnauthorized)
	})
}

func TestE2EProgramUpdate(t *testing.T) {
	reset(t)
	admin := signIn(t, "admin")
	p := createProgram(t, admin, "Editable Program")
	path := "/programs/" + p.ID

	t.Run("fields", func(t *testing.T) {
		var updated struct {
			Description string `json:"description"`
			IsPaid      bool   `json:"is_paid"`
			Industry    string `json:"industry"`
		}
		admin.mustJSON(t, http.MethodPatch, path, map[string]any{"description": "New description", "is_paid": true, "industry": "Cloud"}, http.StatusOK, &updated)
		if updated.Description != "New description" || !updated.IsPaid || updated.Industry != "Cloud" {
			t.Fatalf("updated: %+v", updated)
		}
	})

	t.Run("skills are replaced as a set", func(t *testing.T) {
		admin.mustJSON(t, http.MethodPatch, path, map[string]any{"skills": []string{"Rust", "rust", " Go "}}, http.StatusOK, nil)
		var skills struct {
			Data []struct {
				Skill string `json:"skill"`
			} `json:"data"`
		}
		anonymous().mustJSON(t, http.MethodGet, path+"/skills", nil, http.StatusOK, &skills)
		if len(skills.Data) != 2 {
			t.Fatalf("skills: %+v", skills)
		}
		admin.call(t, http.MethodPatch, path, map[string]any{"skills": []string{" "}}).expect(http.StatusBadRequest)
	})

	t.Run("open terms are replaced as a set", func(t *testing.T) {
		terms := []map[string]any{
			{"id": p.TermID, "name": "Spring renamed", "start_date_time": at(20), "end_date_time": at(120), "application_start_date": at(-1), "application_end_date": at(10)},
			{"name": "Autumn", "start_date_time": at(200), "end_date_time": at(300), "application_start_date": at(150), "application_end_date": at(180)},
		}
		admin.mustJSON(t, http.MethodPatch, path, map[string]any{"terms": terms}, http.StatusOK, nil)
		var list struct {
			Data []struct {
				Name string `json:"name"`
			} `json:"data"`
		}
		anonymous().mustJSON(t, http.MethodGet, path+"/terms?status=open", nil, http.StatusOK, &list)
		if len(list.Data) != 2 {
			t.Fatalf("open terms: %+v", list)
		}
		bad := []map[string]any{{"id": "not-a-uuid", "name": "x", "start_date_time": at(20), "end_date_time": at(120), "application_start_date": at(-1), "application_end_date": at(10)}}
		admin.call(t, http.MethodPatch, path, map[string]any{"terms": bad}).expect(http.StatusBadRequest)
		admin.call(t, http.MethodPatch, path, map[string]any{"terms": []any{}}).expect(http.StatusBadRequest)
		dup := []map[string]any{terms[0], terms[0]}
		admin.call(t, http.MethodPatch, path, map[string]any{"terms": dup}).expect(http.StatusBadRequest)
	})

	t.Run("moving to another project takes Project Service metadata", func(t *testing.T) {
		other := newProject(t, "otherproj")
		var moved struct {
			ProjectUID  string `json:"project_uid"`
			ProjectSlug string `json:"project_slug"`
		}
		admin.mustJSON(t, http.MethodPatch, path, map[string]any{"project_uid": other, "project_slug": "ignored"}, http.StatusOK, &moved)
		if moved.ProjectUID != other || moved.ProjectSlug != "otherproj" {
			t.Fatalf("moved: %+v", moved)
		}
		admin.call(t, http.MethodPatch, path, map[string]any{"project_slug": "no-uid"}).expect(http.StatusBadRequest)
		admin.call(t, http.MethodPatch, path, map[string]any{"project_uid": "nope"}).expect(http.StatusBadRequest)
	})

	t.Run("reserved and lifecycle fields are refused", func(t *testing.T) {
		admin.call(t, http.MethodPatch, path, map[string]any{"status": "published"}).expect(http.StatusBadRequest)
		admin.call(t, http.MethodPatch, path, map[string]any{"logo_url": "https://evil.example.org/x.png"}).expect(http.StatusBadRequest)
		admin.call(t, http.MethodPatch, path, map[string]any{"program_term_status": "bogus"}).expect(http.StatusBadRequest)
	})

	t.Run("unknown program", func(t *testing.T) {
		admin.call(t, http.MethodPatch, "/programs/00000000-0000-4000-8000-000000000000", map[string]any{"description": "x"}).expect(http.StatusNotFound)
		anonymous().call(t, http.MethodPatch, path, map[string]any{"description": "x"}).expect(http.StatusUnauthorized)
	})

	t.Run("skills sub-resource", func(t *testing.T) {
		var skill struct {
			ID    string `json:"id"`
			Skill string `json:"skill"`
		}
		admin.mustJSON(t, http.MethodPost, path+"/skills", map[string]any{"skill": "Python"}, http.StatusCreated, &skill)
		admin.call(t, http.MethodPost, path+"/skills", map[string]any{"skill": " "}).expect(http.StatusBadRequest)
		admin.call(t, http.MethodDelete, path+"/skills/"+skill.ID, nil).expect(http.StatusNoContent)
		anonymous().call(t, http.MethodDelete, path+"/skills/"+skill.ID, nil).expect(http.StatusUnauthorized)
	})
}

func TestE2EProgramReadModels(t *testing.T) {
	reset(t)
	admin := signIn(t, "admin")
	p := publishProgram(t, admin, "Catalog Program")
	pending := createProgram(t, admin, "Pending Program")

	t.Run("catalog lists published programs with skills and labelled terms", func(t *testing.T) {
		var page programPage
		anonymous().mustJSON(t, http.MethodGet, "/programs/catalog", nil, http.StatusOK, &page)
		if len(page.Data) != 1 || page.Data[0].ID != p.ID || len(page.Data[0].Skills) != 2 || len(page.Data[0].Terms) != 1 {
			t.Fatalf("catalog: %+v", page)
		}
		if page.Data[0].Terms[0].DiscoveryLabel == "" {
			t.Fatalf("term has no discovery label")
		}
		for _, q := range []string{"skill=Go", "skill=all", "status=acceptance", "sort_by=name_desc", "sortBy=updated_newest", "search=Catalog", "status=bogus&sort_by=bogus"} {
			var filtered programPage
			anonymous().mustJSON(t, http.MethodGet, "/programs/catalog?"+q, nil, http.StatusOK, &filtered)
			if len(filtered.Data) != 1 {
				t.Fatalf("catalog?%s: %+v", q, filtered)
			}
		}
		var none programPage
		anonymous().mustJSON(t, http.MethodGet, "/programs/catalog?skill=COBOL", nil, http.StatusOK, &none)
		if len(none.Data) != 0 {
			t.Fatalf("skill filter matched: %+v", none)
		}
		anonymous().call(t, http.MethodGet, "/programs/catalog?limit=x", nil).expect(http.StatusBadRequest)
		anonymous().call(t, http.MethodGet, "/programs?offset=x", nil).expect(http.StatusBadRequest)
	})

	t.Run("catalog item, header and mentees", func(t *testing.T) {
		var item struct {
			ID      string `json:"id"`
			Mentors []any  `json:"mentors"`
		}
		anonymous().mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/catalog", nil, http.StatusOK, &item)
		if item.ID != p.ID || item.Mentors == nil {
			t.Fatalf("catalog item: %+v", item)
		}
		anonymous().call(t, http.MethodGet, "/programs/"+pending.ID+"/catalog", nil).expect(http.StatusOK)
		anonymous().call(t, http.MethodGet, "/programs/00000000-0000-4000-8000-000000000000/catalog", nil).expect(http.StatusNotFound)

		var header struct {
			Program struct {
				ID string `json:"id"`
			} `json:"program"`
			ActiveTerm *struct {
				ID string `json:"id"`
			} `json:"active_term"`
		}
		anonymous().mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/header", nil, http.StatusOK, &header)
		if header.Program.ID != p.ID {
			t.Fatalf("header: %+v", header)
		}
		var mentees struct {
			Data []any `json:"data"`
		}
		anonymous().mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/mentees", nil, http.StatusOK, &mentees)
		anonymous().call(t, http.MethodGet, "/programs/00000000-0000-4000-8000-000000000000/mentees", nil).expect(http.StatusNotFound)
	})

	t.Run("management summary and enrollment template", func(t *testing.T) {
		var summary struct {
			HasOpenTerm bool `json:"has_open_term"`
			Mentors     int  `json:"mentors"`
			Terms       int  `json:"terms"`
		}
		admin.mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/management-summary", nil, http.StatusOK, &summary)
		if !summary.HasOpenTerm || summary.Terms != 1 {
			t.Fatalf("summary: %+v", summary)
		}
		admin.mustJSON(t, http.MethodGet, "/programs/"+pending.ID+"/management-summary", nil, http.StatusOK, &summary)
		admin.call(t, http.MethodGet, "/programs/00000000-0000-4000-8000-000000000000/management-summary", nil).expect(http.StatusNotFound)
		anonymous().call(t, http.MethodGet, "/programs/"+p.ID+"/management-summary", nil).expect(http.StatusUnauthorized)

		var template struct {
			Program struct {
				ID string `json:"id"`
			} `json:"program"`
			Skills        []string `json:"skills"`
			Prerequisites []struct {
				Name       string  `json:"name"`
				SubmitFile *string `json:"submitFile"`
			} `json:"prerequisites"`
		}
		admin.mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/enroll-template", nil, http.StatusOK, &template)
		// Only the required prerequisite becomes a task template.
		if template.Program.ID != p.ID || len(template.Skills) != 2 || len(template.Prerequisites) != 1 || template.Prerequisites[0].SubmitFile == nil {
			t.Fatalf("template: %+v", template)
		}
	})

	t.Run("my programs filters", func(t *testing.T) {
		for status, want := range map[string]int{"": 2, "open": 1, "pending_review": 1, "completed": 0} {
			var mine programPage
			admin.mustJSON(t, http.MethodGet, "/me/programs?status="+status, nil, http.StatusOK, &mine)
			if len(mine.Data) != want {
				t.Fatalf("status=%q: got %d programs; want %d", status, len(mine.Data), want)
			}
		}
		admin.call(t, http.MethodGet, "/me/programs?status=bogus", nil).expect(http.StatusBadRequest)
		other := signIn(t, "other")
		var mine programPage
		other.mustJSON(t, http.MethodGet, "/me/programs", nil, http.StatusOK, &mine)
		if len(mine.Data) != 0 {
			t.Fatalf("non-admin sees programs: %+v", mine)
		}
	})
}
