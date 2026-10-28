// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build e2e

package main

import (
	"net/http"
	"slices"
	"testing"
	"time"
)

// TestE2ECreateWaitsForCreatorAccess covers issue #354: the API answers a
// create only once fga-sync says the creator can act on the new object, so the
// client's next request is not denied at the gateway while the outbox catches up.
// No check may be asked before the relay's grant arrives: OpenFGA and fga-sync
// would cache that denial past the end of the wait.
func TestE2ECreateWaitsForCreatorAccess(t *testing.T) {
	reset(t)
	// Longer than the waiter's grace after publication, so it has to poll.
	const hold = time.Second
	e2e.fakes.holdAccess(hold)

	t.Run("program creator holds writer before the enrollment returns", func(t *testing.T) {
		admin := signIn(t, "wait-admin")
		started := time.Now()
		p := createProgram(t, admin, "Waited Program")
		elapsed := time.Since(started)

		if elapsed < hold {
			t.Fatalf("enrollment returned after %s; want it to wait at least the %s access hold", elapsed, hold)
		}
		checks := e2e.fakes.accessChecksFor("mentorship_program:" + p.ID)
		if len(checks) < 2 || checks[0].Allowed || !checks[len(checks)-1].Allowed {
			t.Fatalf("access checks = %+v; want a denied check first and an allowed one last", checks)
		}
		if want := "mentorship_program:" + p.ID + "#writer@user:wait-admin"; checks[0].Check != want {
			t.Fatalf("checked %q; want %q", checks[0].Check, want)
		}
		assertNoCheckBeforeGrant(t, "mentorship_program:"+p.ID, "wait-admin", checks)
		// The creator's membership arrives as member_put, whose relations are an
		// array, unlike update_access; the fake must accept both grant shapes.
		var memberPut bool
		for _, g := range e2e.fakes.grantsFor("mentorship_program:" + p.ID) {
			memberPut = memberPut || (g.Operation == "member_put" && slices.Contains(g.Users, "wait-admin"))
		}
		if !memberPut {
			t.Fatalf("grants = %+v; want the creator's member_put accepted", e2e.fakes.grantsFor("mentorship_program:"+p.ID))
		}
		// The whole point: the logo upload right after the create goes through first time.
		uploadLogo(t, admin, p.ID).expect(http.StatusCreated)
	})

	t.Run("applicant holds auditor before the application returns", func(t *testing.T) {
		admin := signIn(t, "wait-program-admin")
		p := publishProgram(t, admin, "Waited Applications")
		mentee := signInMentee(t, "wait-mentee")

		started := time.Now()
		app := apply(t, mentee, p)
		if elapsed := time.Since(started); elapsed < hold {
			t.Fatalf("application returned after %s; want it to wait at least the %s access hold", elapsed, hold)
		}
		checks := e2e.fakes.accessChecksFor("mentorship_application:" + app.ID)
		if len(checks) < 2 || checks[0].Allowed || !checks[len(checks)-1].Allowed {
			t.Fatalf("access checks = %+v; want a denied check first and an allowed one last", checks)
		}
		if want := "mentorship_application:" + app.ID + "#auditor@user:wait-mentee"; checks[0].Check != want {
			t.Fatalf("checked %q; want %q", checks[0].Check, want)
		}
		assertNoCheckBeforeGrant(t, "mentorship_application:"+app.ID, "wait-mentee", checks)
		if tasks := tasksOf(t, mentee, app.ID); len(tasks) == 0 {
			t.Fatal("the new application has no tasks to read")
		}
	})

	t.Run("a create still succeeds when access never arrives in time", func(t *testing.T) {
		e2e.fakes.holdAccess(time.Hour)
		admin := signIn(t, "wait-timeout-admin")
		started := time.Now()
		p := createProgram(t, admin, "Unwaited Program")
		elapsed := time.Since(started)

		if elapsed < e2eAccessWaitTimeout || elapsed > e2eAccessWaitTimeout+2*time.Second {
			t.Fatalf("enrollment returned after %s; want it to give up at about the %s timeout", elapsed, e2eAccessWaitTimeout)
		}
		if checks := e2e.fakes.accessChecksFor("mentorship_program:" + p.ID); len(checks) == 0 || checks[len(checks)-1].Allowed {
			t.Fatalf("access checks = %+v; want only denied checks", checks)
		}
		if p.Status != "pending" {
			t.Fatalf("program status = %q; want pending (saved despite the wait timing out)", p.Status)
		}
		// The logo upload checks the creator's admin row, so the FGA lag does not block it.
		uploadLogo(t, admin, p.ID).expect(http.StatusCreated)
	})
}

// assertNoCheckBeforeGrant fails if any check on object reached fga-sync before the relay's grant to user did.
func assertNoCheckBeforeGrant(t *testing.T, object, user string, checks []accessCheck) {
	t.Helper()
	granted, ok := e2e.fakes.grantedAt(object, user)
	if !ok {
		t.Fatalf("no grant for %s on %s reached fga-sync", user, object)
	}
	if first := checks[0].At; first.Before(granted) {
		t.Fatalf("first check at %s preceded the grant at %s; that denial would be cached past the wait", first.Format(time.StampMicro), granted.Format(time.StampMicro))
	}
}
