// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build e2e

package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

type fgaMessage struct {
	ObjectType string `json:"object_type"`
	Operation  string `json:"operation"`
	Data       struct {
		UID       string          `json:"uid"`
		Public    bool            `json:"public"`
		Relations json.RawMessage `json:"relations"`
		Username  string          `json:"username"`
	} `json:"data"`
}

type indexMessage struct {
	Action  string            `json:"action"`
	Headers map[string]string `json:"headers"`
	Data    json.RawMessage   `json:"data"`
}

// awaitFGA waits for an FGA sync message on subject that satisfies match.
func awaitFGA(t *testing.T, subject string, match func(fgaMessage) bool) fgaMessage {
	t.Helper()
	var found fgaMessage
	eventually(t, 5*time.Second, "FGA message on "+subject, func() bool {
		for _, raw := range streamMessages(t, "E2E_FGA", subject) {
			var m fgaMessage
			if json.Unmarshal(raw, &m) == nil && match(m) {
				found = m
				return true
			}
		}
		return false
	})
	return found
}

// awaitIndex waits for an indexer message for objectType that satisfies match.
func awaitIndex(t *testing.T, objectType string, match func(indexMessage) bool) indexMessage {
	t.Helper()
	var found indexMessage
	eventually(t, 5*time.Second, "index message for "+objectType, func() bool {
		for _, raw := range streamMessages(t, "E2E_INDEX", "lfx.index."+objectType) {
			var m indexMessage
			if json.Unmarshal(raw, &m) == nil && match(m) {
				found = m
				return true
			}
		}
		return false
	})
	return found
}

func indexedID(m indexMessage) string {
	var doc struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(m.Data, &doc)
	return doc.ID
}

func TestE2ERelaysPublishAccessAndIndexChanges(t *testing.T) {
	reset(t)
	admin := signIn(t, "admin")
	p := publishProgram(t, admin, "Relay Program")

	t.Run("program access and creator membership", func(t *testing.T) {
		access := awaitFGA(t, "lfx.fga-sync.update_access", func(m fgaMessage) bool {
			return m.ObjectType == "mentorship_program" && m.Data.UID == p.ID && m.Data.Public
		})
		if len(access.Data.Relations) == 0 {
			t.Fatalf("program access has no relations: %+v", access)
		}
		awaitFGA(t, "lfx.fga-sync.member_put", func(m fgaMessage) bool {
			return m.ObjectType == "mentorship_program" && m.Data.UID == p.ID && m.Data.Username == "admin"
		})
	})

	t.Run("program index document carries the service token", func(t *testing.T) {
		doc := awaitIndex(t, "mentorship_program", func(m indexMessage) bool {
			return indexedID(m) == p.ID && m.Action == "updated"
		})
		if doc.Headers["authorization"] != "Bearer e2e-indexer-token" {
			t.Fatalf("index headers: %v", doc.Headers)
		}
	})

	mentor := signIn(t, "relay-mentor")
	inviteMentor(t, p, mentor)
	mentee := signIn(t, "relay-mentee")
	app := apply(t, mentee, p)

	t.Run("applications and tasks are indexed and access-controlled", func(t *testing.T) {
		awaitIndex(t, "mentorship_application", func(m indexMessage) bool { return indexedID(m) == app.ID })
		awaitFGA(t, "lfx.fga-sync.update_access", func(m fgaMessage) bool {
			return m.ObjectType == "mentorship_application" && m.Data.UID == app.ID
		})
		task := tasksOf(t, mentee, app.ID)[0]
		awaitIndex(t, "mentorship_task", func(m indexMessage) bool { return indexedID(m) == task.ID })
		awaitFGA(t, "lfx.fga-sync.update_access", func(m fgaMessage) bool {
			return m.ObjectType == "mentorship_task" && m.Data.UID == task.ID
		})
	})

	t.Run("deleting the program removes access and index entries", func(t *testing.T) {
		admin.call(t, http.MethodDelete, "/programs/"+p.ID, nil).expect(http.StatusNoContent)
		awaitFGA(t, "lfx.fga-sync.delete_access", func(m fgaMessage) bool {
			return m.ObjectType == "mentorship_program" && m.Data.UID == p.ID
		})
		awaitFGA(t, "lfx.fga-sync.delete_access", func(m fgaMessage) bool {
			return m.ObjectType == "mentorship_application" && m.Data.UID == app.ID
		})
		awaitIndex(t, "mentorship_program", func(m indexMessage) bool {
			var id string
			return m.Action == "deleted" && json.Unmarshal(m.Data, &id) == nil && id == p.ID
		})
	})
}

func TestE2ERelayPublishesApproverRoster(t *testing.T) {
	reset(t)
	manager := signIn(t, "roster-manager", "manage:mentorship:approvers")
	candidate := signIn(t, "roster-candidate")
	manager.mustJSON(t, http.MethodPost, "/admin/approver-team/members", map[string]any{"user_id": candidate.ID}, http.StatusCreated, nil)
	awaitFGA(t, "lfx.fga-sync.member_put", func(m fgaMessage) bool {
		return m.ObjectType == "mentorship_approver_team" && m.Data.Username == "roster-candidate"
	})
	manager.call(t, http.MethodDelete, "/admin/approver-team/members/"+candidate.ID, nil).expect(http.StatusNoContent)
	awaitFGA(t, "lfx.fga-sync.member_remove", func(m fgaMessage) bool {
		return m.ObjectType == "mentorship_approver_team" && m.Data.Username == "roster-candidate"
	})
}
