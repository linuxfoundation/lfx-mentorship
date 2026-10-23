// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

func str(v string) *string { return &v }

type fakeStore struct {
	rows     []*models.User
	written  []models.UserCreateInput
	writeErr map[string]error
}

func (f *fakeStore) ListIncompleteIdentities(context.Context, int) ([]*models.User, error) {
	return f.rows, nil
}

func (f *fakeStore) BootstrapByLFID(_ context.Context, in models.UserCreateInput) (*models.User, error) {
	if err := f.writeErr[*in.LFID]; err != nil {
		return nil, err
	}
	f.written = append(f.written, in)
	return &models.User{LFID: in.LFID}, nil
}

// fakeResolver fills from a per-LFID directory record, as the service does.
type fakeResolver struct {
	directory map[string]models.UserUpdateInput
	calls     int
}

func (f *fakeResolver) ResolveIdentity(_ context.Context, lfid string, in models.UserUpdateInput) models.UserUpdateInput {
	f.calls++
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
	return in
}

func fixtures() (*fakeStore, *fakeResolver) {
	store := &fakeStore{rows: []*models.User{
		{ID: "1", LFID: str("ada")}, // directory has everything
		{ID: "2", LFID: str("grace"), Email: str("g@x.org"), Name: str("G")}, // directory adds given/family only
		{ID: "3", LFID: str("ghost")},                                        // directory knows nothing
		{ID: "4"},                                                            // legacy row without LFID: skipped
	}}
	resolver := &fakeResolver{directory: map[string]models.UserUpdateInput{
		"ada":   {Email: str("ada@x.org"), Name: str("Ada L"), GivenName: str("Ada"), FamilyName: str("L"), AvatarURL: str("https://p/ada")},
		"grace": {Email: str("other@x.org"), Name: str("Other"), GivenName: str("Grace"), FamilyName: str("H")},
	}}
	return store, resolver
}

func TestBackfill_DryRunReportsWithoutWriting(t *testing.T) {
	store, resolver := fixtures()
	var out bytes.Buffer
	b := &backfill{users: store, identity: resolver, apply: false, out: &out}

	s, err := b.run(context.Background(), 100)
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
	if resolver.calls != 3 {
		t.Errorf("directory consulted %d times; want 3 (once per row with an LFID)", resolver.calls)
	}
}

func TestBackfill_ApplyWritesOnlyImprovedRowsAndKeepsStoredValues(t *testing.T) {
	store, resolver := fixtures()
	b := &backfill{users: store, identity: resolver, apply: true, out: &bytes.Buffer{}}

	s, err := b.run(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if want := (summary{Seen: 3, Completed: 1, Partial: 1, Unchanged: 1}); s != want {
		t.Fatalf("summary = %+v; want %+v", s, want)
	}
	if len(store.written) != 2 || *store.written[0].LFID != "ada" || *store.written[1].LFID != "grace" {
		t.Fatalf("written = %+v; want ada and grace only", store.written)
	}
	grace := store.written[1]
	if *grace.Email != "g@x.org" || *grace.Name != "G" || *grace.GivenName != "Grace" || *grace.FamilyName != "H" || grace.AvatarURL != nil {
		t.Errorf("grace upsert = %+v; stored email and name must win, directory fills the rest", grace)
	}
}

func TestBackfill_CountsWriteFailuresAndContinues(t *testing.T) {
	store, resolver := fixtures()
	store.writeErr = map[string]error{"ada": errors.New("email in use")}
	var out bytes.Buffer
	b := &backfill{users: store, identity: resolver, apply: true, out: &out}

	s, err := b.run(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if s.Failed != 1 || s.Partial != 1 || len(store.written) != 1 || *store.written[0].LFID != "grace" {
		t.Fatalf("summary %+v, written %+v; want ada failed and grace still written", s, store.written)
	}
	if !strings.Contains(out.String(), "ada: FAILED email in use") {
		t.Errorf("report lacks the failure:\n%s", out.String())
	}
}
