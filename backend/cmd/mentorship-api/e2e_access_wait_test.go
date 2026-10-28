// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build e2e

package main

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// TestE2ECreateWaitsForCreatorAccess covers issue #354: the API answers a
// create only once fga-sync says the creator can act on the new object, so the
// client's next request is not denied at the gateway while the outbox catches up.
// The wait may make exactly one check, after the tuple is applied: a check before
// that is denied, and the fake, like OpenFGA's check-query cache, would keep
// serving that denial for the cache TTL (TestE2EFakeFGASyncRemembersDenials).
func TestE2ECreateWaitsForCreatorAccess(t *testing.T) {
	reset(t)
	// Applying lags the relay's publish by this much; the wait has to notice, not guess.
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
		assertFirstCheckAllowedAfterApply(t, "mentorship_program:"+p.ID, "wait-admin", "writer", hold, checks)
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
		assertFirstCheckAllowedAfterApply(t, "mentorship_application:"+app.ID, "wait-mentee", "auditor", hold, checks)
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
		if checks := e2e.fakes.accessChecksFor("mentorship_program:" + p.ID); len(checks) != 0 {
			t.Fatalf("access checks = %+v; want none: a check before the tuple is applied would be cached as a denial", checks)
		}
		if p.Status != "pending" {
			t.Fatalf("program status = %q; want pending (saved despite the wait timing out)", p.Status)
		}
		// The logo upload checks the creator's admin row, so the FGA lag does not block it.
		uploadLogo(t, admin, p.ID).expect(http.StatusCreated)
	})
}

// TestE2EFakeFGASyncRemembersDenials pins the fake's cache model, which the wait tests
// rely on: a check denied from the store stays denied until the entry lapses, even once
// the tuple is applied, and a hit on the entry does not extend it, as in OpenFGA.
func TestE2EFakeFGASyncRemembersDenials(t *testing.T) {
	reset(t)
	const ttl = 300 * time.Millisecond
	e2e.fakes.setCheckCacheTTL(ttl)
	const object, user = "mentorship_program:cache-test", "cache-user"
	check := object + "#writer@user:" + user

	ask := func() bool {
		t.Helper()
		reply, err := e2e.nc.Request("lfx.access_check.request", []byte(check), time.Second)
		if err != nil {
			t.Fatalf("access check: %v", err)
		}
		return strings.TrimSpace(string(reply.Data)) == check+"\ttrue"
	}
	// Taken before the ask: the fake stamps the entry before it replies, so this is
	// never later than the fake's own clock and lapsed >= ttl below always holds.
	denied := time.Now()
	if ask() {
		t.Fatal("a user with no tuple must be denied")
	}
	// Apply the tuple directly, as the relay's member_put would.
	e2e.fakes.recordTuples(&nats.Msg{Data: []byte(`{"object_type":"mentorship_program","operation":"member_put","data":{"uid":"cache-test","username":"` + user + `","relations":["writer"]}}`)})

	// Hits on the cached denial must neither allow nor extend it.
	for time.Since(denied) < ttl/2 {
		if ask() {
			t.Fatal("the cached denial was not served while its entry was live")
		}
		time.Sleep(20 * time.Millisecond)
	}
	eventually(t, ttl+time.Second, "the denial to lapse and the applied tuple to be seen", ask)
	if lapsed := time.Since(denied); lapsed < ttl {
		t.Fatalf("denial lapsed after %s; want at least the %s TTL", lapsed, ttl)
	}
}

// assertFirstCheckAllowedAfterApply fails unless the wait's one and only check on object
// was allowed and came after the tuple was applied (the relay's grant plus the apply hold).
// A check before that is answered false and, like OpenFGA's cache, stays false.
func assertFirstCheckAllowedAfterApply(t *testing.T, object, user, relation string, hold time.Duration, checks []accessCheck) {
	t.Helper()
	granted, ok := e2e.fakes.grantedAt(object, user)
	if !ok {
		t.Fatalf("no grant for %s on %s reached fga-sync", user, object)
	}
	if len(checks) != 1 || !checks[0].Allowed {
		t.Fatalf("access checks = %+v; want exactly one, allowed", checks)
	}
	if want := object + "#" + relation + "@user:" + user; checks[0].Check != want {
		t.Fatalf("checked %q; want %q", checks[0].Check, want)
	}
	if applied := granted.Add(hold); checks[0].At.Before(applied) {
		t.Fatalf("check at %s preceded the tuple's application at %s", checks[0].At.Format(time.StampMicro), applied.Format(time.StampMicro))
	}
}
