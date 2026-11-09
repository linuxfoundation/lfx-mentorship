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

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/db"
)

func str(v string) *string { return &v }

type write struct {
	ID    string
	Input models.UserUpdateInput
}

// fakeStore pages rows in order, honouring the cursor as the repository does.
type fakeStore struct {
	rows     []*models.User
	listErr  error
	writeErr map[string]error
	written  []write
	listed   []int // page sizes requested, in order
}

func (f *fakeStore) ListIncompleteIdentities(_ context.Context, after *db.UserCursor, limit int) ([]*models.User, error) {
	f.listed = append(f.listed, limit)
	if f.listErr != nil {
		return nil, f.listErr
	}
	start := 0
	if after != nil {
		for i, r := range f.rows {
			if r.ID == after.ID {
				start = i + 1
			}
		}
	}
	end := min(start+limit, len(f.rows))
	return f.rows[start:end], nil
}

func (f *fakeStore) FillMissingIdentity(_ context.Context, id string, in models.UserUpdateInput) (*models.User, error) {
	if err := f.writeErr[id]; err != nil {
		return nil, err
	}
	f.written = append(f.written, write{id, in})
	return &models.User{ID: id}, nil
}

// fakeResolver fills from a per-LFID directory record, as the service does,
// and fails for LFIDs listed in down.
type fakeResolver struct {
	directory map[string]models.UserUpdateInput
	down      map[string]bool
	calls     int
}

func (f *fakeResolver) ResolveIdentity(_ context.Context, lfid string, in models.UserUpdateInput) (models.UserUpdateInput, error) {
	f.calls++
	if f.down[lfid] {
		return in, errors.New("nats: no responders")
	}
	d := f.directory[lfid]
	fill := func(field **string, v *string) {
		if *field == nil && v != nil {
			*field = v
		}
	}
	fill(&in.Email, d.Email)
	fill(&in.Name, d.Name)
	fill(&in.GivenName, d.GivenName)
	fill(&in.FamilyName, d.FamilyName)
	fill(&in.AvatarURL, d.AvatarURL)
	return in, nil
}

func row(id, lfid string, at int) *models.User {
	return &models.User{ID: id, LFID: str(lfid), CreatedOn: time.Date(2026, 1, 1, 0, 0, at, 0, time.UTC)}
}

func fixtures() (*fakeStore, *fakeResolver) {
	grace := row("2", "grace", 2)
	grace.Email, grace.Name = str("g@x.org"), str("G")
	store := &fakeStore{rows: []*models.User{
		row("1", "ada", 1), // directory has everything
		grace,              // directory adds given/family only
		row("3", "ghost", 3),
		{ID: "4", CreatedOn: time.Date(2026, 1, 1, 0, 0, 4, 0, time.UTC)}, // legacy row without LFID: skipped
	}}
	resolver := &fakeResolver{directory: map[string]models.UserUpdateInput{
		"ada":   {Email: str("ada@x.org"), Name: str("Ada L"), GivenName: str("Ada"), FamilyName: str("L"), AvatarURL: str("https://p/ada")},
		"grace": {Email: str("other@x.org"), Name: str("Other"), GivenName: str("Grace"), FamilyName: str("H")},
	}}
	return store, resolver
}

func newBackfill(store *fakeStore, resolver *fakeResolver, apply bool) (*backfill, *bytes.Buffer) {
	var out bytes.Buffer
	return &backfill{users: store, identity: resolver, apply: apply, pageSize: 2, out: &out}, &out
}

func TestBackfill_DryRunReportsWithoutWriting(t *testing.T) {
	store, resolver := fixtures()
	b, out := newBackfill(store, resolver, false)

	s, err := b.run(context.Background(), nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.written) != 0 {
		t.Fatalf("dry run wrote %d rows; want none", len(store.written))
	}
	if want := (summary{Seen: 3, Completed: 1, Partial: 1, Unchanged: 1}); s != want {
		t.Fatalf("summary = %+v; want %+v", s, want)
	}
	for _, line := range []string{
		"ada: completed [email name given_name family_name avatar_url]",
		"grace: filled [given_name family_name], still missing 1",
		"ghost: unchanged",
		"(dry run; nothing written)",
	} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("report lacks %q:\n%s", line, out.String())
		}
	}
	if strings.Contains(out.String(), "--cursor") {
		t.Errorf("a run that reached the end must not print a cursor:\n%s", out.String())
	}
	if resolver.calls != 3 {
		t.Errorf("directory consulted %d times; want 3 (once per row with an LFID)", resolver.calls)
	}
}

// Apply sends only what the directory added, so the NULL-only UPDATE cannot
// disturb a value stored since the row was read.
func TestBackfill_ApplyWritesOnlyDirectoryAdditions(t *testing.T) {
	store, resolver := fixtures()
	b, _ := newBackfill(store, resolver, true)

	s, err := b.run(context.Background(), nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	if want := (summary{Seen: 3, Completed: 1, Partial: 1, Unchanged: 1}); s != want {
		t.Fatalf("summary = %+v; want %+v", s, want)
	}
	if len(store.written) != 2 || store.written[0].ID != "1" || store.written[1].ID != "2" {
		t.Fatalf("written = %+v; want ada and grace only", store.written)
	}
	grace := store.written[1].Input
	if grace.Email != nil || grace.Name != nil || *grace.GivenName != "Grace" || *grace.FamilyName != "H" || grace.AvatarURL != nil {
		t.Errorf("grace update = %+v; want only given_name and family_name, never the stored email or name", grace)
	}
}

func TestBackfill_CountsFailuresAndContinues(t *testing.T) {
	store, resolver := fixtures()
	store.writeErr = map[string]error{"1": errors.New("email in use")}
	resolver.down = map[string]bool{"ghost": true}
	b, out := newBackfill(store, resolver, true)

	s, err := b.run(context.Background(), nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	if want := (summary{Seen: 3, Partial: 1, Failed: 2}); s != want {
		t.Fatalf("summary = %+v; want %+v (ada's write and ghost's lookup failed, grace written)", s, want)
	}
	if len(store.written) != 1 || store.written[0].ID != "2" {
		t.Fatalf("written = %+v; want grace only", store.written)
	}
	for _, line := range []string{"ada: FAILED email in use", "ghost: FAILED directory lookup: nats: no responders"} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("report lacks %q:\n%s", line, out.String())
		}
	}
}

// A directory that does not answer is a failure, not "nothing more to fill".
func TestBackfill_DirectoryDownIsNotSuccess(t *testing.T) {
	store, resolver := fixtures()
	resolver.down = map[string]bool{"ada": true, "grace": true, "ghost": true}
	b, out := newBackfill(store, resolver, true)

	s, err := b.run(context.Background(), nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	if s.Failed != 3 || s.Unchanged != 0 || len(store.written) != 0 {
		t.Fatalf("summary = %+v, written %d; want every row failed and nothing written", s, len(store.written))
	}
	if strings.Contains(out.String(), "unchanged (directory") {
		t.Errorf("lookup failures must not be reported as unchanged:\n%s", out.String())
	}
}

func TestBackfill_ListFailureStopsBeforeAnyLookupOrWrite(t *testing.T) {
	store, resolver := fixtures()
	store.listErr = errors.New("db down")
	b, _ := newBackfill(store, resolver, true)

	_, err := b.run(context.Background(), nil, 100)
	if err == nil || !strings.Contains(err.Error(), "db down") {
		t.Fatalf("err = %v; want the list error", err)
	}
	if resolver.calls != 0 || len(store.written) != 0 {
		t.Fatalf("lookups %d, writes %d; want none after a failed list", resolver.calls, len(store.written))
	}
}

// --limit bounds a run; the printed cursor continues it past rows the
// directory could not complete, so later users still get repaired.
func TestBackfill_LimitPrintsCursorAndCursorResumes(t *testing.T) {
	store, resolver := fixtures()
	b, out := newBackfill(store, resolver, false)

	first, err := b.run(context.Background(), nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Seen != 1 || first.Next == nil || first.Next.ID != "1" {
		t.Fatalf("first run = %+v; want one row seen and a cursor after ada", first)
	}
	if want := "continue with --cursor " + first.Next.String(); !strings.Contains(out.String(), want) {
		t.Fatalf("report lacks %q:\n%s", want, out.String())
	}
	parsed, err := db.ParseUserCursor(first.Next.String())
	if err != nil || parsed != *first.Next {
		t.Fatalf("cursor does not round-trip: %v %+v", err, parsed)
	}

	out.Reset()
	second, err := b.run(context.Background(), &parsed, 100)
	if err != nil {
		t.Fatal(err)
	}
	if second.Seen != 2 || second.Next != nil || strings.Contains(out.String(), "ada:") {
		t.Fatalf("second run = %+v; want grace and ghost only, no further cursor:\n%s", second, out.String())
	}
}

func TestBackfill_CancelledContextStopsWithError(t *testing.T) {
	store, resolver := fixtures()
	b, _ := newBackfill(store, resolver, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := b.run(ctx, nil, 100)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v; want context.Canceled", err)
	}
	if len(store.written) != 0 {
		t.Fatalf("wrote %d rows after cancellation", len(store.written))
	}
}
