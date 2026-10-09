// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build e2e

package main

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	emailapi "github.com/linuxfoundation/lfx-v2-email-service/pkg/api"
)

type member struct {
	ID         string  `json:"id"`
	ProgramID  string  `json:"program_id"`
	UserID     string  `json:"user_id"`
	MemberType string  `json:"member_type"`
	Status     string  `json:"status"`
	Email      *string `json:"email"`
}

var inviteLink = regexp.MustCompile(regexp.QuoteMeta(e2eSelfServeURL) + `/mentorship/mentor/invites\?token=([^\s"<&]+)`)

// awaitEmail waits for the nth (1-based) email to address whose subject contains subject.
func awaitEmail(t *testing.T, address, subject string, n int) emailapi.SendEmailRequest {
	t.Helper()
	var match []emailapi.SendEmailRequest
	eventually(t, 5*time.Second, "email to "+address+" about "+subject, func() bool {
		match = match[:0]
		for _, e := range e2e.fakes.emailsTo(address) {
			if strings.Contains(e.Subject, subject) {
				match = append(match, e)
			}
		}
		return len(match) >= n
	})
	return match[n-1]
}

// inviteToken returns the token from the nth invite email sent to address.
func inviteToken(t *testing.T, address string, n int) string {
	t.Helper()
	msg := awaitEmail(t, address, "invited to mentor", n)
	m := inviteLink.FindStringSubmatch(msg.Text)
	if m == nil {
		t.Fatalf("no invite link in email: %s", msg.Text)
	}
	token, err := url.QueryUnescape(m[1])
	if err != nil {
		t.Fatalf("unescape token: %v", err)
	}
	if !strings.Contains(msg.HTML, url.QueryEscape(token)) {
		t.Fatalf("HTML body lacks the invite link")
	}
	return token
}

// inviteMentor invites mentor to p and has them accept, returning the active membership.
func inviteMentor(t *testing.T, p *program, mentor *actor) member {
	t.Helper()
	before := len(e2e.fakes.emailsTo(mentor.Email))
	var m member
	p.Admin.mustJSON(t, http.MethodPost, "/programs/"+p.ID+"/members", map[string]any{"lfid": mentor.LFID, "member_type": "mentor"}, http.StatusCreated, &m)
	token := inviteToken(t, mentor.Email, before+1)
	mentor.mustJSON(t, http.MethodPost, "/mentor-invites/"+url.PathEscape(token)+"/accept", nil, http.StatusOK, &m)
	return m
}

func TestE2EMentorInvites(t *testing.T) {
	reset(t)
	admin := signIn(t, "admin")
	p := publishProgram(t, admin, "Invite Program")
	members := "/programs/" + p.ID + "/members"

	t.Run("invite an existing user and accept", func(t *testing.T) {
		mentor := signIn(t, "mentor-one")
		var invited member
		admin.mustJSON(t, http.MethodPost, members, map[string]any{"lfid": "mentor-one", "member_type": "mentor"}, http.StatusCreated, &invited)
		if invited.Status != "invited" || invited.UserID != mentor.ID {
			t.Fatalf("invited: %+v", invited)
		}
		admin.call(t, http.MethodPost, members, map[string]any{"user_id": mentor.ID, "member_type": "mentor"}).expect(http.StatusConflict)

		token := inviteToken(t, mentor.Email, 1)
		escaped := url.PathEscape(token)
		someoneElse := signIn(t, "someone-else")
		someoneElse.call(t, http.MethodPost, "/mentor-invites/"+escaped+"/accept", nil).expect(http.StatusForbidden)
		someoneElse.call(t, http.MethodPost, "/mentor-invites/"+escaped+"/decline", nil).expect(http.StatusForbidden)

		var accepted member
		mentor.mustJSON(t, http.MethodPost, "/mentor-invites/"+escaped+"/accept", nil, http.StatusOK, &accepted)
		if accepted.Status != "active" {
			t.Fatalf("accepted: %+v", accepted)
		}
		awaitEmail(t, admin.Email, "accepted your mentor invitation", 1)
		mentor.call(t, http.MethodPost, "/mentor-invites/"+escaped+"/accept", nil).expect(http.StatusBadRequest)

		var roster struct {
			Data []member `json:"data"`
		}
		anonymous().mustJSON(t, http.MethodGet, members, nil, http.StatusOK, &roster)
		if len(roster.Data) != 1 || roster.Data[0].UserID != mentor.ID || roster.Data[0].Email != nil {
			t.Fatalf("public roster: %+v", roster)
		}

		var mine struct {
			Data []struct {
				ProgramID   string `json:"program_id"`
				ProgramName string `json:"program_name"`
				MemberType  string `json:"member_type"`
				Status      string `json:"status"`
			} `json:"data"`
		}
		mentor.mustJSON(t, http.MethodGet, "/me/program-memberships?member_type=mentor", nil, http.StatusOK, &mine)
		if len(mine.Data) != 1 || mine.Data[0].ProgramName != "Invite Program" || mine.Data[0].Status != "active" {
			t.Fatalf("my memberships: %+v", mine)
		}
		mentor.call(t, http.MethodGet, "/me/program-memberships?member_type=owner", nil).expect(http.StatusBadRequest)
		admin.mustJSON(t, http.MethodGet, "/me/program-memberships?member_type=program_admin", nil, http.StatusOK, &mine)
		if len(mine.Data) != 1 || mine.Data[0].MemberType != "program_admin" {
			t.Fatalf("admin memberships: %+v", mine)
		}

		var mentored struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		mentor.mustJSON(t, http.MethodGet, "/me/mentor-programs", nil, http.StatusOK, &mentored)
		if len(mentored.Data) != 1 || mentored.Data[0].ID != p.ID {
			t.Fatalf("mentored programs: %+v", mentored)
		}
	})

	t.Run("invite someone who never signed in, by LFID", func(t *testing.T) {
		e2e.fakes.addAccount(fakeAccount{Username: "newcomer", Email: "newcomer@lf.example.org", Name: "New Comer"})
		var invited member
		admin.mustJSON(t, http.MethodPost, members, map[string]any{"lfid": "newcomer", "member_type": "mentor"}, http.StatusCreated, &invited)
		token := inviteToken(t, "newcomer@lf.example.org", 1)

		// Signing in later binds to the user the invite created.
		newcomer := signIn(t, "newcomer")
		if newcomer.ID != invited.UserID {
			t.Fatalf("sign-in created a second user: %s vs %s", newcomer.ID, invited.UserID)
		}
		newcomer.call(t, http.MethodPost, "/mentor-invites/"+url.PathEscape(token)+"/decline", nil).expect(http.StatusNoContent)
		awaitEmail(t, admin.Email, "declined your mentor invitation", 1)
		newcomer.call(t, http.MethodPost, "/mentor-invites/"+url.PathEscape(token)+"/decline", nil).expect(http.StatusBadRequest)
	})

	t.Run("invite by email", func(t *testing.T) {
		e2e.fakes.addAccount(fakeAccount{Username: "by-email", Email: "by-email@lf.example.org", Name: "By Email"})
		var invited member
		admin.mustJSON(t, http.MethodPost, members, map[string]any{"email": "by-email@lf.example.org", "member_type": "mentor"}, http.StatusCreated, &invited)
		if invited.Email == nil || *invited.Email != "by-email@lf.example.org" {
			t.Fatalf("invited by email: %+v", invited)
		}
		inviteToken(t, "by-email@lf.example.org", 1)

		t.Run("resend issues a new invite", func(t *testing.T) {
			admin.call(t, http.MethodPost, members+"/"+invited.ID+"/resend-invite", nil).expect(http.StatusNoContent)
			inviteToken(t, "by-email@lf.example.org", 2)
			admin.call(t, http.MethodPost, members+"/00000000-0000-4000-8000-000000000000/resend-invite", nil).expect(http.StatusNotFound)
			anonymous().call(t, http.MethodPost, members+"/"+invited.ID+"/resend-invite", nil).expect(http.StatusUnauthorized)
		})
	})

	t.Run("invite validation", func(t *testing.T) {
		pending := createProgram(t, admin, "Unpublished Invite Program")
		e2e.fakes.addAccount(fakeAccount{Username: "dup-email", Email: admin.Email})
		cases := []struct {
			name   string
			path   string
			body   map[string]any
			status int
		}{
			{"no invitee", members, map[string]any{"member_type": "mentor"}, http.StatusBadRequest},
			{"user_id and lfid", members, map[string]any{"user_id": admin.ID, "lfid": "admin", "member_type": "mentor"}, http.StatusBadRequest},
			{"malformed email", members, map[string]any{"email": "not an email", "member_type": "mentor"}, http.StatusBadRequest},
			{"bad member type", members, map[string]any{"lfid": "admin", "member_type": "owner"}, http.StatusBadRequest},
			{"bad status", members, map[string]any{"lfid": "admin", "member_type": "mentor", "status": "bogus"}, http.StatusBadRequest},
			{"unpublished program", "/programs/" + pending.ID + "/members", map[string]any{"lfid": "admin", "member_type": "mentor"}, http.StatusBadRequest},
			{"unknown program", "/programs/00000000-0000-4000-8000-000000000000/members", map[string]any{"lfid": "admin", "member_type": "mentor"}, http.StatusNotFound},
			{"email with no LF account", members, map[string]any{"email": "nobody@nowhere.example.org", "member_type": "mentor"}, http.StatusUnprocessableEntity},
			{"LFID with no LF account", members, map[string]any{"lfid": "ghost", "member_type": "mentor"}, http.StatusUnprocessableEntity},
			{"LFID that is not a username", members, map[string]any{"lfid": "x", "member_type": "mentor"}, http.StatusBadRequest},
			{"primary email held by another user", members, map[string]any{"lfid": "dup-email", "member_type": "mentor"}, http.StatusUnprocessableEntity},
			{"unknown user ID", members, map[string]any{"user_id": "00000000-0000-4000-8000-000000000000", "member_type": "mentor"}, http.StatusUnprocessableEntity},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				admin.call(t, http.MethodPost, tc.path, tc.body).expect(tc.status)
			})
		}
		anonymous().call(t, http.MethodPost, members, map[string]any{"lfid": "admin", "member_type": "mentor"}).expect(http.StatusUnauthorized)
		anonymous().call(t, http.MethodGet, "/programs/"+pending.ID+"/members", nil).expect(http.StatusNotFound)
	})

	t.Run("bad invite tokens", func(t *testing.T) {
		admin.call(t, http.MethodPost, "/mentor-invites/garbage/accept", nil).expect(http.StatusBadRequest)
		admin.call(t, http.MethodPost, "/mentor-invites/garbage/decline", nil).expect(http.StatusBadRequest)
		anonymous().call(t, http.MethodPost, "/mentor-invites/garbage/accept", nil).expect(http.StatusUnauthorized)
	})
}

func TestE2EProgramMemberManagement(t *testing.T) {
	reset(t)
	admin := signIn(t, "admin")
	p := publishProgram(t, admin, "Members Program")
	members := "/programs/" + p.ID + "/members"
	mentor := signIn(t, "mentor-two")

	var m member
	admin.mustJSON(t, http.MethodPost, members, map[string]any{"user_id": mentor.ID, "member_type": "mentor"}, http.StatusCreated, &m)

	t.Run("status lifecycle", func(t *testing.T) {
		for _, step := range []struct{ to, want string }{{"pending", "pending"}, {"active", "active"}, {"withdrawn", "withdrawn"}} {
			admin.mustJSON(t, http.MethodPatch, members+"/"+m.ID, map[string]any{"status": step.to}, http.StatusOK, &m)
			if m.Status != step.want {
				t.Fatalf("after %s: %+v", step.to, m)
			}
		}
		admin.call(t, http.MethodPatch, members+"/"+m.ID, map[string]any{"status": "active"}).expect(http.StatusConflict)
		admin.call(t, http.MethodPatch, members+"/"+m.ID, map[string]any{"status": "bogus"}).expect(http.StatusBadRequest)
		admin.mustJSON(t, http.MethodPatch, members+"/"+m.ID, map[string]any{"email": "mentor-two@work.example.org"}, http.StatusOK, &m)
		if m.Email == nil || *m.Email != "mentor-two@work.example.org" {
			t.Fatalf("email update: %+v", m)
		}
		admin.call(t, http.MethodPatch, members+"/00000000-0000-4000-8000-000000000000", map[string]any{"status": "active"}).expect(http.StatusNotFound)
		other := publishProgram(t, admin, "Other Members Program")
		admin.call(t, http.MethodPatch, "/programs/"+other.ID+"/members/"+m.ID, map[string]any{"status": "pending"}).expect(http.StatusNotFound)
		admin.call(t, http.MethodDelete, "/programs/"+other.ID+"/members/"+m.ID, nil).expect(http.StatusNotFound)
		admin.call(t, http.MethodPost, "/programs/"+other.ID+"/members/"+m.ID+"/resend-invite", nil).expect(http.StatusNotFound)
		admin.call(t, http.MethodPost, members+"/"+m.ID+"/resend-invite", nil).expect(http.StatusConflict)
		anonymous().call(t, http.MethodPatch, members+"/"+m.ID, map[string]any{"status": "active"}).expect(http.StatusUnauthorized)
	})

	t.Run("program admins are active on creation", func(t *testing.T) {
		coAdmin := signIn(t, "co-admin")
		var a member
		admin.mustJSON(t, http.MethodPost, members, map[string]any{"user_id": coAdmin.ID, "member_type": "program_admin"}, http.StatusCreated, &a)
		if a.Status != "active" || a.MemberType != "program_admin" {
			t.Fatalf("co-admin: %+v", a)
		}
		var mine programPage
		coAdmin.mustJSON(t, http.MethodGet, "/me/programs", nil, http.StatusOK, &mine)
		if len(mine.Data) != 1 {
			t.Fatalf("co-admin programs: %+v", mine)
		}
	})

	t.Run("management list", func(t *testing.T) {
		var rows struct {
			Data []struct {
				UserID         string `json:"user_id"`
				Status         string `json:"status"`
				ProfileCreated bool   `json:"profile_created"`
			} `json:"data"`
		}
		admin.mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/member-management", nil, http.StatusOK, &rows)
		if len(rows.Data) != 1 || rows.Data[0].UserID != mentor.ID {
			t.Fatalf("mentor management: %+v", rows)
		}
		admin.mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/member-management?status=active", nil, http.StatusOK, &rows)
		if len(rows.Data) != 0 {
			t.Fatalf("status filter: %+v", rows)
		}
		admin.mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/member-management?search=mentor-two", nil, http.StatusOK, &rows)
		admin.call(t, http.MethodGet, "/programs/"+p.ID+"/member-management?limit=x", nil).expect(http.StatusBadRequest)
		anonymous().call(t, http.MethodGet, "/programs/"+p.ID+"/member-management", nil).expect(http.StatusUnauthorized)
	})

	t.Run("delete", func(t *testing.T) {
		admin.call(t, http.MethodDelete, members+"/"+m.ID, nil).expect(http.StatusNoContent)
		admin.call(t, http.MethodDelete, members+"/"+m.ID, nil).expect(http.StatusNotFound)
		anonymous().call(t, http.MethodDelete, members+"/"+m.ID, nil).expect(http.StatusUnauthorized)
	})
}

func TestE2EMentorRequests(t *testing.T) {
	reset(t)
	admin := signIn(t, "admin")
	p := publishProgram(t, admin, "Request Program")
	mentor := signIn(t, "volunteer")

	var req member
	mentor.mustJSON(t, http.MethodPost, "/me/program-memberships", map[string]any{"program_id": p.ID}, http.StatusCreated, &req)
	if req.Status != "requested" || req.UserID != mentor.ID {
		t.Fatalf("request: %+v", req)
	}
	mentor.call(t, http.MethodPost, "/me/program-memberships", map[string]any{"program_id": p.ID}).expect(http.StatusConflict)

	t.Run("only the requester can withdraw", func(t *testing.T) {
		signIn(t, "intruder").call(t, http.MethodPost, "/me/program-memberships/"+req.ID+"/withdraw", nil).expect(http.StatusNotFound)
		mentor.call(t, http.MethodPost, "/me/program-memberships/not-a-uuid/withdraw", nil).expect(http.StatusNotFound)
	})

	mentor.call(t, http.MethodPost, "/me/program-memberships/"+req.ID+"/withdraw", nil).expect(http.StatusNoContent)
	mentor.call(t, http.MethodPost, "/me/program-memberships/"+req.ID+"/withdraw", nil).expect(http.StatusConflict)

	// A withdrawn request is reopened in place.
	var again member
	mentor.mustJSON(t, http.MethodPost, "/me/program-memberships", map[string]any{"program_id": p.ID}, http.StatusCreated, &again)
	if again.ID != req.ID || again.Status != "requested" {
		t.Fatalf("re-request: %+v", again)
	}

	t.Run("declining a request tells the mentor", func(t *testing.T) {
		admin.mustJSON(t, http.MethodPatch, "/programs/"+p.ID+"/members/"+req.ID, map[string]any{"status": "declined"}, http.StatusOK, nil)
		awaitEmail(t, mentor.Email, "was declined", 1)
		mentor.call(t, http.MethodPost, "/me/program-memberships", map[string]any{"program_id": p.ID}).expect(http.StatusConflict)
	})

	t.Run("request validation", func(t *testing.T) {
		pending := createProgram(t, admin, "Pending Request Program")
		mentor.call(t, http.MethodPost, "/me/program-memberships", map[string]any{"program_id": pending.ID}).expect(http.StatusBadRequest)
		submitted := createProgram(t, admin, "Submitted Request Program")
		uploadLogo(t, admin, submitted.ID).expect(http.StatusCreated)
		admin.mustJSON(t, http.MethodPost, "/programs/"+submitted.ID+"/submit", nil, http.StatusOK, nil)
		mentor.call(t, http.MethodPost, "/me/program-memberships", map[string]any{"program_id": submitted.ID}).expect(http.StatusNotFound)
		mentor.call(t, http.MethodPost, "/me/program-memberships", map[string]any{"program_id": "nope"}).expect(http.StatusBadRequest)
		mentor.call(t, http.MethodPost, "/me/program-memberships", map[string]any{"program_id": "00000000-0000-4000-8000-000000000000"}).expect(http.StatusNotFound)
		anonymous().call(t, http.MethodPost, "/me/program-memberships", map[string]any{"program_id": p.ID}).expect(http.StatusUnauthorized)
		mentor.call(t, http.MethodGet, "/me/program-memberships?limit=x", nil).expect(http.StatusBadRequest)
	})
}

func TestE2EMentorCandidates(t *testing.T) {
	reset(t)
	admin := signIn(t, "admin")
	p := publishProgram(t, admin, "Candidates Program")
	signIn(t, "carla")
	e2e.fakes.addAccount(fakeAccount{Username: "dirk", Email: "dirk@lf.example.org", Name: "Dirk Directory"})
	path := "/programs/" + p.ID + "/mentor-candidates?search="

	type candidates struct {
		Data []struct {
			LFID string `json:"lfid"`
			Name string `json:"name"`
		} `json:"data"`
	}
	lfids := func(c candidates) []string {
		var out []string
		for _, d := range c.Data {
			out = append(out, d.LFID)
		}
		return out
	}

	var got candidates
	admin.mustJSON(t, http.MethodGet, path+"car", nil, http.StatusOK, &got)
	if !contains(lfids(got), "carla") {
		t.Fatalf("local search: %+v", got)
	}
	admin.mustJSON(t, http.MethodGet, path+"dirk", nil, http.StatusOK, &got)
	if len(got.Data) != 1 || got.Data[0].LFID != "dirk" || got.Data[0].Name != "Dirk Directory" {
		t.Fatalf("directory LFID search: %+v", got)
	}
	admin.mustJSON(t, http.MethodGet, path+url.QueryEscape("dirk@lf.example.org"), nil, http.StatusOK, &got)
	if len(got.Data) != 1 || got.Data[0].LFID != "dirk" {
		t.Fatalf("email search: %+v", got)
	}
	admin.mustJSON(t, http.MethodGet, path+url.QueryEscape("missing@lf.example.org"), nil, http.StatusOK, &got)
	if len(got.Data) != 0 {
		t.Fatalf("unknown email: %+v", got)
	}
	admin.mustJSON(t, http.MethodGet, path+"zz-nobody", nil, http.StatusOK, &got)
	if len(got.Data) != 0 {
		t.Fatalf("unknown LFID: %+v", got)
	}
	admin.call(t, http.MethodGet, path+"x", nil).expect(http.StatusBadRequest)
	pending := createProgram(t, admin, "Pending Candidates Program")
	admin.call(t, http.MethodGet, "/programs/"+pending.ID+"/mentor-candidates?search=carla", nil).expect(http.StatusBadRequest)
	admin.call(t, http.MethodGet, "/programs/00000000-0000-4000-8000-000000000000/mentor-candidates?search=carla", nil).expect(http.StatusNotFound)
	anonymous().call(t, http.MethodGet, path+"carla", nil).expect(http.StatusUnauthorized)
}
