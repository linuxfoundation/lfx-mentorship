// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/db"
)

func str(v string) *string { return &v }

type fakeStore struct {
	programs []db.ProgramProjectMetadata
	listErr  error
	syncErr  map[string]error
	synced   map[string]models.ProjectMetadata
}

func (f *fakeStore) ListProgramProjectMetadata(context.Context) ([]db.ProgramProjectMetadata, error) {
	return f.programs, f.listErr
}

// SyncProjectMetadata updates the matching rows the way the repository does and returns their IDs.
func (f *fakeStore) SyncProjectMetadata(_ context.Context, uid string, project models.ProjectMetadata) ([]string, error) {
	if err := f.syncErr[uid]; err != nil {
		return nil, err
	}
	if f.synced == nil {
		f.synced = map[string]models.ProjectMetadata{}
	}
	f.synced[uid] = project
	var ids []string
	for _, p := range f.programs {
		if p.ProjectUID == uid {
			ids = append(ids, staleProgramIDs([]db.ProgramProjectMetadata{p}, &project)...)
		}
	}
	return ids, nil
}

type fakeProjects map[string]any // uid -> *models.ProjectMetadata or error

func (f fakeProjects) GetProject(_ context.Context, uid string) (*models.ProjectMetadata, error) {
	switch v := f[uid].(type) {
	case *models.ProjectMetadata:
		return v, nil
	case error:
		return nil, v
	}
	return nil, domain.ErrProjectNotFound
}

func newTestSyncer(store *fakeStore, projects fakeProjects, dryRun bool) *syncer {
	return &syncer{store: store, projects: projects, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), dryRun: dryRun}
}

// Three projects: one renamed (two programs, one already current), one unchanged, one whose
// logo was removed in Project Service.
func scenario() (*fakeStore, fakeProjects) {
	store := &fakeStore{programs: []db.ProgramProjectMetadata{
		{ProgramID: "p1", ProjectUID: "renamed", Slug: str("old-slug"), Name: str("Old Name"), LogoURL: str("https://logo/a.png")},
		{ProgramID: "p2", ProjectUID: "renamed", Slug: str("new-slug"), Name: str("New Name"), LogoURL: str("https://logo/a.png")},
		{ProgramID: "p3", ProjectUID: "same", Slug: str("same"), Name: str("Same"), LogoURL: nil},
		{ProgramID: "p4", ProjectUID: "unlogoed", Slug: str("u"), Name: str("U"), LogoURL: str("https://logo/u.png")},
	}}
	projects := fakeProjects{
		"renamed":  &models.ProjectMetadata{Slug: "new-slug", Name: "New Name", LogoURL: str("https://logo/a.png")},
		"same":     &models.ProjectMetadata{Slug: "same", Name: "Same"},
		"unlogoed": &models.ProjectMetadata{Slug: "u", Name: "U"},
	}
	return store, projects
}

func TestSyncer_RefreshesOnlyProjectsThatChanged(t *testing.T) {
	store, projects := scenario()

	result, err := newTestSyncer(store, projects, false).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := syncResult{Projects: 3, Programs: 4, Unchanged: 2, Stale: 2, Updated: 2}
	if result != want {
		t.Fatalf("result = %+v; want %+v", result, want)
	}
	if _, ok := store.synced["same"]; ok {
		t.Error("an unchanged project was written")
	}
	if got := store.synced["unlogoed"]; got.LogoURL != nil {
		t.Errorf("logo synced as %v; want it cleared, as Project Service has none", *got.LogoURL)
	}
	if got := store.synced["renamed"]; got.Name != "New Name" || got.Slug != "new-slug" {
		t.Errorf("renamed project synced as %+v", got)
	}
}

func TestSyncer_DryRunWritesNothing(t *testing.T) {
	store, projects := scenario()

	result, err := newTestSyncer(store, projects, true).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Stale != 2 || result.Updated != 0 || len(store.synced) != 0 {
		t.Fatalf("result = %+v, synced = %v; want 2 stale programs reported and nothing written", result, store.synced)
	}
}

// A project Project Service does not know keeps its stored values; one it cannot answer for, or
// that cannot be written, is counted as failed while the other projects are still refreshed.
func TestSyncer_MissingAndFailedProjects(t *testing.T) {
	store, projects := scenario()
	delete(projects, "same")
	projects["renamed"] = fmt.Errorf("nats timeout: %w", domain.ErrUpstreamUnavailable)
	store.syncErr = map[string]error{"unlogoed": errors.New("connection reset")}

	result, err := newTestSyncer(store, projects, false).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := syncResult{Projects: 3, Programs: 4, Stale: 1, Missing: 1, Failed: 2}
	if result != want {
		t.Fatalf("result = %+v; want %+v", result, want)
	}
	if len(store.synced) != 0 {
		t.Fatalf("synced = %v; want nothing written", store.synced)
	}
}

func TestSyncer_StopsWhenListingFails(t *testing.T) {
	store := &fakeStore{listErr: errors.New("db down")}
	if _, err := newTestSyncer(store, fakeProjects{}, false).Run(context.Background()); err == nil {
		t.Fatal("Run succeeded; want the listing error")
	}
}

func TestGroupByProject(t *testing.T) {
	programs := []db.ProgramProjectMetadata{{ProgramID: "a", ProjectUID: "x"}, {ProgramID: "b", ProjectUID: "x"}, {ProgramID: "c", ProjectUID: "y"}}
	var got [][]string
	for _, group := range groupByProject(programs) {
		var ids []string
		for _, p := range group {
			ids = append(ids, p.ProgramID)
		}
		got = append(got, ids)
	}
	if want := [][]string{{"a", "b"}, {"c"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("groups = %v; want %v", got, want)
	}
}
