// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// fakeRoster is an in-memory roster keyed by user ID, recording the writes.
type fakeRoster struct {
	members map[string]string // user ID → LFID
	added   []string
	removed []string
	listErr error
	addErr  error
}

func (f *fakeRoster) ListApprovers(context.Context) ([]*models.RosterMember, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []*models.RosterMember
	for id, lfid := range f.members {
		out = append(out, &models.RosterMember{ObjectUID: "global", UserID: id, LFID: lfid, CreatedOn: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)})
	}
	return out, nil
}

func (f *fakeRoster) AddApprover(_ context.Context, userID string) (*models.RosterMember, error) {
	if f.addErr != nil {
		return nil, f.addErr
	}
	f.added = append(f.added, userID)
	f.members[userID] = "lfid-of-" + userID
	return &models.RosterMember{UserID: userID}, nil
}

func (f *fakeRoster) RemoveApprover(_ context.Context, userID string) error {
	if _, ok := f.members[userID]; !ok {
		return domain.ErrUserNotFound
	}
	f.removed = append(f.removed, userID)
	delete(f.members, userID)
	return nil
}

// fakeUsers resolves LFIDs to user IDs of the form "id-<lfid>".
type fakeUsers struct {
	known map[string]bool
	err   error
}

func (f fakeUsers) GetByLFID(_ context.Context, lfid string) (userRef, error) {
	if f.err != nil {
		return userRef{}, f.err
	}
	if !f.known[lfid] {
		return userRef{}, domain.ErrUserNotFound
	}
	return userRef{ID: "id-" + lfid}, nil
}

func newAdmin(roster *fakeRoster, users fakeUsers) (*admin, *bytes.Buffer) {
	out := &bytes.Buffer{}
	return &admin{roster: roster, users: users, out: out}, out
}

func TestValidateArgs(t *testing.T) {
	cases := map[string]struct {
		command string
		lfids   []string
		wantErr string
	}{
		"list":                     {command: "list"},
		"list with args":           {command: "list", lfids: []string{"ada"}, wantErr: "takes no arguments"},
		"add":                      {command: "add", lfids: []string{"ada", "grace"}},
		"add without lfids":        {command: "add", wantErr: "needs at least one LFID"},
		"remove without lfids":     {command: "remove", wantErr: "needs at least one LFID"},
		"sync without lfids":       {command: "sync", wantErr: "needs at least one LFID"},
		"blank lfid":               {command: "sync", lfids: []string{"ada", " "}, wantErr: "empty LFID"},
		"duplicate lfid":           {command: "add", lfids: []string{"ada", "ada"}, wantErr: `"ada" given more than once`},
		"unknown command":          {command: "promote", lfids: []string{"ada"}, wantErr: `unknown command "promote"`},
		"sync refuses to empty it": {command: "sync", lfids: nil, wantErr: "needs at least one LFID"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := validateArgs(tc.command, tc.lfids)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateArgs = %v; want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validateArgs = %v; want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestList(t *testing.T) {
	t.Run("prints each member", func(t *testing.T) {
		a, out := newAdmin(&fakeRoster{members: map[string]string{"id-ada": "ada"}}, fakeUsers{})
		if err := a.run(context.Background(), "list", nil); err != nil {
			t.Fatal(err)
		}
		if got := out.String(); got != "ada\tid-ada\t2026-10-20\n" {
			t.Fatalf("output = %q", got)
		}
	})
	t.Run("says when empty", func(t *testing.T) {
		a, out := newAdmin(&fakeRoster{members: map[string]string{}}, fakeUsers{})
		if err := a.run(context.Background(), "list", nil); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "empty") {
			t.Fatalf("output = %q", out.String())
		}
	})
	t.Run("propagates repository errors", func(t *testing.T) {
		boom := errors.New("boom")
		a, _ := newAdmin(&fakeRoster{listErr: boom}, fakeUsers{})
		if err := a.run(context.Background(), "list", nil); !errors.Is(err, boom) {
			t.Fatalf("err = %v; want %v", err, boom)
		}
	})
}

func TestAdd(t *testing.T) {
	t.Run("resolves LFIDs to user IDs", func(t *testing.T) {
		roster := &fakeRoster{members: map[string]string{}}
		a, out := newAdmin(roster, fakeUsers{known: map[string]bool{"ada": true, "grace": true}})
		if err := a.run(context.Background(), "add", []string{"ada", "grace"}); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(roster.added, ","); got != "id-ada,id-grace" {
			t.Fatalf("added = %q", got)
		}
		if !strings.Contains(out.String(), "added ada\n") || !strings.Contains(out.String(), "added grace\n") {
			t.Fatalf("output = %q", out.String())
		}
	})
	t.Run("adds the known LFIDs and then reports the unknown ones", func(t *testing.T) {
		roster := &fakeRoster{members: map[string]string{}}
		a, _ := newAdmin(roster, fakeUsers{known: map[string]bool{"ada": true}})
		err := a.run(context.Background(), "add", []string{"nobody", "ada", "ghost"})
		var unknown errUnknownLFIDs
		if !errors.As(err, &unknown) {
			t.Fatalf("err = %v; want errUnknownLFIDs", err)
		}
		if got := strings.Join(unknown, ","); got != "nobody,ghost" {
			t.Fatalf("unknown = %q", got)
		}
		if !strings.Contains(err.Error(), "sign in to Self Serve") {
			t.Fatalf("error should explain the fix: %v", err)
		}
		if got := strings.Join(roster.added, ","); got != "id-ada" {
			t.Fatalf("added = %q; want the known LFID still added", got)
		}
	})
	t.Run("stops on a lookup failure", func(t *testing.T) {
		boom := errors.New("db down")
		roster := &fakeRoster{members: map[string]string{}}
		a, _ := newAdmin(roster, fakeUsers{err: boom})
		if err := a.run(context.Background(), "add", []string{"ada"}); !errors.Is(err, boom) {
			t.Fatalf("err = %v; want %v", err, boom)
		}
		if len(roster.added) != 0 {
			t.Fatalf("added = %v; want nothing", roster.added)
		}
	})
	t.Run("stops on a write failure", func(t *testing.T) {
		boom := errors.New("write failed")
		a, _ := newAdmin(&fakeRoster{members: map[string]string{}, addErr: boom}, fakeUsers{known: map[string]bool{"ada": true}})
		if err := a.run(context.Background(), "add", []string{"ada"}); !errors.Is(err, boom) {
			t.Fatalf("err = %v; want %v", err, boom)
		}
	})
}

func TestRemove(t *testing.T) {
	t.Run("removes by LFID and ignores non-members", func(t *testing.T) {
		roster := &fakeRoster{members: map[string]string{"id-ada": "ada", "id-grace": "grace"}}
		a, out := newAdmin(roster, fakeUsers{})
		if err := a.run(context.Background(), "remove", []string{"ada", "linus"}); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(roster.removed, ","); got != "id-ada" {
			t.Fatalf("removed = %q", got)
		}
		if !strings.Contains(out.String(), "removed ada\n") || !strings.Contains(out.String(), "linus is not an approver\n") {
			t.Fatalf("output = %q", out.String())
		}
		if _, still := roster.members["id-grace"]; !still {
			t.Fatal("grace should be untouched")
		}
	})
}

func TestSync(t *testing.T) {
	t.Run("adds the missing and removes the extra members", func(t *testing.T) {
		roster := &fakeRoster{members: map[string]string{"id-ada": "ada", "id-old": "old"}}
		a, _ := newAdmin(roster, fakeUsers{known: map[string]bool{"ada": true, "grace": true}})
		if err := a.run(context.Background(), "sync", []string{"ada", "grace"}); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(roster.added, ","); got != "id-grace" {
			t.Fatalf("added = %q", got)
		}
		if got := strings.Join(roster.removed, ","); got != "id-old" {
			t.Fatalf("removed = %q", got)
		}
	})
	t.Run("is a no-op when the roster already matches", func(t *testing.T) {
		roster := &fakeRoster{members: map[string]string{"id-ada": "ada", "id-grace": "grace"}}
		a, out := newAdmin(roster, fakeUsers{known: map[string]bool{"ada": true, "grace": true}})
		if err := a.run(context.Background(), "sync", []string{"grace", "ada"}); err != nil {
			t.Fatal(err)
		}
		if len(roster.added)+len(roster.removed) != 0 {
			t.Fatalf("added %v removed %v; want no writes", roster.added, roster.removed)
		}
		if !strings.Contains(out.String(), "already matches (2 members)") {
			t.Fatalf("output = %q", out.String())
		}
	})
	t.Run("applies every other change before failing on unknown LFIDs", func(t *testing.T) {
		roster := &fakeRoster{members: map[string]string{"id-old": "old"}}
		a, _ := newAdmin(roster, fakeUsers{known: map[string]bool{"ada": true}})
		err := a.run(context.Background(), "sync", []string{"ada", "ghost"})
		var unknown errUnknownLFIDs
		if !errors.As(err, &unknown) || strings.Join(unknown, ",") != "ghost" {
			t.Fatalf("err = %v; want unknown ghost", err)
		}
		if got := strings.Join(roster.added, ","); got != "id-ada" {
			t.Fatalf("added = %q", got)
		}
		if got := strings.Join(roster.removed, ","); got != "id-old" {
			t.Fatalf("removed = %q", got)
		}
	})
	t.Run("does not touch the roster when it cannot be read", func(t *testing.T) {
		boom := errors.New("boom")
		roster := &fakeRoster{listErr: boom}
		a, _ := newAdmin(roster, fakeUsers{known: map[string]bool{"ada": true}})
		if err := a.run(context.Background(), "sync", []string{"ada"}); !errors.Is(err, boom) {
			t.Fatalf("err = %v; want %v", err, boom)
		}
		if len(roster.added)+len(roster.removed) != 0 {
			t.Fatal("no writes expected")
		}
	})
}
