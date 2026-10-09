// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build e2e

package main

import (
	"net/http"
	"testing"
)

type term struct {
	ID             string `json:"id"`
	ProgramID      string `json:"program_id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	DiscoveryLabel string `json:"discovery_label"`
}

// termBody is a POST /programs/{id}/terms body whose dates are offset from today.
func termBody(name string, appStart, appEnd, start, end int) map[string]any {
	return map[string]any{
		"name": name, "application_start_date": at(appStart), "application_end_date": at(appEnd),
		"start_date_time": at(start), "end_date_time": at(end),
	}
}

func TestE2EProgramTerms(t *testing.T) {
	reset(t)
	admin := signIn(t, "admin")
	p := publishProgram(t, admin, "Terms Program")
	base := "/programs/" + p.ID + "/terms"

	var created term
	admin.mustJSON(t, http.MethodPost, base, termBody("Summer", 30, 40, 50, 90), http.StatusCreated, &created)
	if created.Status != "open" || created.ProgramID != p.ID {
		t.Fatalf("created: %+v", created)
	}

	t.Run("create validation", func(t *testing.T) {
		admin.call(t, http.MethodPost, base, termBody(" ", 30, 40, 50, 90)).expect(http.StatusBadRequest)
		admin.call(t, http.MethodPost, base, termBody("Backwards", 30, 40, 90, 50)).expect(http.StatusBadRequest)
		admin.call(t, http.MethodPost, base, termBody("Late window", 30, 60, 50, 90)).expect(http.StatusBadRequest)
		deleted := termBody("Deleted", 30, 40, 50, 90)
		deleted["status"] = "deleted"
		admin.call(t, http.MethodPost, base, deleted).expect(http.StatusBadRequest)
		anonymous().call(t, http.MethodPost, base, termBody("Anon", 30, 40, 50, 90)).expect(http.StatusUnauthorized)
	})

	t.Run("open-term cap", func(t *testing.T) {
		// Spring and Summer are open; two more reach the cap of four.
		admin.mustJSON(t, http.MethodPost, base, termBody("Autumn", 100, 110, 120, 160), http.StatusCreated, nil)
		var winter term
		admin.mustJSON(t, http.MethodPost, base, termBody("Winter", 170, 180, 190, 230), http.StatusCreated, &winter)
		res := admin.call(t, http.MethodPost, base, termBody("Fifth", 240, 250, 260, 300)).expect(http.StatusConflict)
		if msg := res.errorMessage(); msg == "" {
			t.Fatalf("no error message")
		}
		closed := termBody("Closed extra", 240, 250, 260, 300)
		closed["status"] = "closed"
		var extra term
		admin.mustJSON(t, http.MethodPost, base, closed, http.StatusCreated, &extra)
		if extra.Status != "closed" {
			t.Fatalf("closed extra: %+v", extra)
		}
		// At the cap a closed term cannot be reopened.
		admin.call(t, http.MethodPost, base+"/"+extra.ID+"/reopen", nil).expect(http.StatusConflict)
		admin.call(t, http.MethodDelete, base+"/"+winter.ID, nil).expect(http.StatusNoContent)
		admin.mustJSON(t, http.MethodPost, base+"/"+extra.ID+"/reopen", nil, http.StatusOK, &extra)
		if extra.Status != "open" {
			t.Fatalf("reopened: %+v", extra)
		}
	})

	t.Run("read", func(t *testing.T) {
		var got term
		anonymous().mustJSON(t, http.MethodGet, base+"/"+created.ID, nil, http.StatusOK, &got)
		if got.ID != created.ID || got.DiscoveryLabel == "" {
			t.Fatalf("get: %+v", got)
		}
		other := publishProgram(t, admin, "Other Terms Program")
		anonymous().call(t, http.MethodGet, "/programs/"+other.ID+"/terms/"+created.ID, nil).expect(http.StatusNotFound)
		admin.call(t, http.MethodPatch, "/programs/"+other.ID+"/terms/"+created.ID, map[string]any{"name": "x"}).expect(http.StatusNotFound)

		var list struct {
			Data []term `json:"data"`
			Meta struct {
				Total int `json:"total"`
			} `json:"meta"`
		}
		anonymous().mustJSON(t, http.MethodGet, base+"?limit=2", nil, http.StatusOK, &list)
		if len(list.Data) != 2 || list.Meta.Total < 4 {
			t.Fatalf("paged list: %+v", list)
		}
		anonymous().call(t, http.MethodGet, base+"?status=deleted", nil).expect(http.StatusBadRequest)
		anonymous().call(t, http.MethodGet, base+"?status=bogus", nil).expect(http.StatusBadRequest)
		anonymous().call(t, http.MethodGet, base+"?limit=x", nil).expect(http.StatusBadRequest)

		var mgmt struct {
			Data []struct {
				ID      string `json:"id"`
				Pending int    `json:"pending"`
			} `json:"data"`
		}
		admin.mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/term-management", nil, http.StatusOK, &mgmt)
		if len(mgmt.Data) < 4 {
			t.Fatalf("term management: %+v", mgmt)
		}
		admin.call(t, http.MethodGet, "/programs/"+p.ID+"/term-management?offset=x", nil).expect(http.StatusBadRequest)
		anonymous().call(t, http.MethodGet, "/programs/"+p.ID+"/term-management", nil).expect(http.StatusUnauthorized)
	})

	t.Run("update", func(t *testing.T) {
		var renamed term
		admin.mustJSON(t, http.MethodPatch, base+"/"+created.ID, map[string]any{"name": "Summer 2"}, http.StatusOK, &renamed)
		if renamed.Name != "Summer 2" {
			t.Fatalf("renamed: %+v", renamed)
		}
		admin.call(t, http.MethodPatch, base+"/"+created.ID, map[string]any{"status": "closed"}).expect(http.StatusBadRequest)
		admin.call(t, http.MethodPatch, base+"/"+created.ID, map[string]any{"end_date_time": at(45)}).expect(http.StatusBadRequest)
		admin.call(t, http.MethodPatch, base+"/00000000-0000-4000-8000-000000000000", map[string]any{"name": "x"}).expect(http.StatusNotFound)
	})

	t.Run("close and reopen", func(t *testing.T) {
		var closed term
		admin.mustJSON(t, http.MethodPost, base+"/"+created.ID+"/close", nil, http.StatusOK, &closed)
		if closed.Status != "closed" {
			t.Fatalf("closed: %+v", closed)
		}
		var open struct {
			Data []term `json:"data"`
		}
		anonymous().mustJSON(t, http.MethodGet, base+"?status=closed", nil, http.StatusOK, &open)
		if len(open.Data) != 1 || open.Data[0].ID != created.ID {
			t.Fatalf("closed filter: %+v", open)
		}
		var reopened term
		admin.mustJSON(t, http.MethodPost, base+"/"+created.ID+"/reopen", nil, http.StatusOK, &reopened)
		if reopened.Status != "open" {
			t.Fatalf("reopened: %+v", reopened)
		}
		admin.call(t, http.MethodPost, base+"/"+created.ID+"/reopen", nil).expect(http.StatusConflict)
		anonymous().call(t, http.MethodPost, base+"/"+created.ID+"/close", nil).expect(http.StatusUnauthorized)
		admin.call(t, http.MethodPost, base+"/00000000-0000-4000-8000-000000000000/close", nil).expect(http.StatusNotFound)
		admin.call(t, http.MethodPost, base+"/00000000-0000-4000-8000-000000000000/reopen", nil).expect(http.StatusNotFound)
	})

	t.Run("an ended term is locked", func(t *testing.T) {
		var past term
		admin.mustJSON(t, http.MethodPost, base+"/"+created.ID+"/close", nil, http.StatusOK, &past)
		dbExec(t, `UPDATE program_terms SET start_date_time = NOW() - interval '60 days', end_date_time = NOW() - interval '1 day',
			application_start_date = NOW() - interval '90 days', application_end_date = NOW() - interval '70 days' WHERE id = $1`, created.ID)
		admin.call(t, http.MethodPost, base+"/"+created.ID+"/reopen", nil).expect(http.StatusConflict)
		admin.call(t, http.MethodPatch, base+"/"+created.ID, map[string]any{"name": "Rewrite history"}).expect(http.StatusConflict)
	})

	t.Run("delete", func(t *testing.T) {
		admin.call(t, http.MethodDelete, base+"/"+created.ID, nil).expect(http.StatusNoContent)
		anonymous().call(t, http.MethodGet, base+"/"+created.ID, nil).expect(http.StatusNotFound)
		admin.call(t, http.MethodDelete, base+"/00000000-0000-4000-8000-000000000000", nil).expect(http.StatusNotFound)
		anonymous().call(t, http.MethodDelete, base+"/"+p.TermID, nil).expect(http.StatusUnauthorized)
	})
}
