// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build e2e

package main

import (
	"net/http"
	"testing"
	"time"
)

// TestE2ECreateWaitsForCreatorAccess covers issue #354: the API answers a
// create only once fga-sync says the creator can act on the new object, so the
// client's next request is not denied at the gateway while the outbox catches up.
func TestE2ECreateWaitsForCreatorAccess(t *testing.T) {
	reset(t)
	const hold = 300 * time.Millisecond
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
	})
}
