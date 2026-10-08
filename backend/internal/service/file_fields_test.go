// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// The file routes are the only writers of file columns; generic routes must reject them.
func TestGenericRoutesRejectFileFields(t *testing.T) {
	url := "https://attacker.example/x.svg"
	ctx := context.Background()
	tests := map[string]func() error{
		"program update logo_url": func() error {
			_, err := newProgramSvc(&stubProgRepo{}, &stubTermRepo{}, &stubAppRepo{}).Update(ctx, "p1", models.ProgramUpdateInput{LogoURL: &url})
			return err
		},
		"program create logo_url": func() error {
			_, err := newProgramSvc(&stubProgRepo{}, &stubTermRepo{}, &stubAppRepo{}).Create(ctx, models.ProgramCreateInput{Name: "n", Slug: "s", LogoURL: &url})
			return err
		},
		"task update file": func() error {
			_, err := newTaskSvc(&stubTaskRepo{}, &stubAppRepo{}, &stubTermRepo{}, &stubMemberRepo{}).Update(ctx, "t1", models.TaskUpdateInput{File: &url})
			return err
		},
	}
	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("err = %v; want ErrInvalidInput", err)
			}
		})
	}
}

// Profile logos are hosted by another service, so profile writes store the URL they are given.
func TestProfileLogoURLIsSaved(t *testing.T) {
	logo := "https://images.example.org/profile.png"
	var saved *string
	repo := &stubUserProfileRepo{
		create: func(_ context.Context, in models.UserProfileCreateInput) (*models.UserProfile, error) {
			saved = in.LogoURL
			return &models.UserProfile{}, nil
		},
		upsertByUserAndType: func(_ context.Context, in models.UserProfileCreateInput) (*models.UserProfile, bool, error) {
			saved = in.LogoURL
			return &models.UserProfile{}, true, nil
		},
		update: func(_ context.Context, id string, in models.UserProfileUpdateInput) (*models.UserProfile, error) {
			saved = in.LogoURL
			return &models.UserProfile{ID: id}, nil
		},
	}
	svc := newUserProfileSvc(repo)
	for name, call := range map[string]func() error{
		"create": func() error {
			_, err := svc.Create(context.Background(), models.UserProfileCreateInput{UserID: "u1", ProfileType: "mentor", LogoURL: &logo})
			return err
		},
		"upsert": func() error {
			_, _, err := svc.Upsert(context.Background(), models.UserProfileCreateInput{UserID: "u1", ProfileType: "mentor", LogoURL: &logo})
			return err
		},
		"update": func() error {
			_, err := svc.Update(context.Background(), "prof1", models.UserProfileUpdateInput{LogoURL: &logo})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			saved = nil
			if err := call(); err != nil {
				t.Fatalf("err = %v", err)
			}
			if saved == nil || *saved != logo {
				t.Fatalf("saved logo_url = %v; want %q", saved, logo)
			}
		})
	}
}

func TestProfileLinksWithoutResumeAreAccepted(t *testing.T) {
	links := json.RawMessage(`{"linkedinProfileLink":"https://linkedin.com/in/x","githubProfileLink":"https://github.com/x"}`)
	if _, err := newUserProfileSvc(&stubUserProfileRepo{}).Update(context.Background(), "prof1", models.UserProfileUpdateInput{ProfileLinks: links}); err != nil {
		t.Fatalf("Update: %v", err)
	}
}

// A client echoing back a migrated profile still saves; the resume link is dropped, not stored.
func TestProfileResumeLinkIsDropped(t *testing.T) {
	links := json.RawMessage(`{"githubProfileLink":"https://github.com/x","resumeLink":"https://x/cv.pdf"}`)
	var saved json.RawMessage
	repo := &stubUserProfileRepo{
		update: func(_ context.Context, id string, in models.UserProfileUpdateInput) (*models.UserProfile, error) {
			saved = in.ProfileLinks
			return &models.UserProfile{ID: id}, nil
		},
		create: func(_ context.Context, in models.UserProfileCreateInput) (*models.UserProfile, error) {
			saved = in.ProfileLinks
			return &models.UserProfile{}, nil
		},
	}
	svc := newUserProfileSvc(repo)
	for name, call := range map[string]func() error{
		"update": func() error {
			_, err := svc.Update(context.Background(), "prof1", models.UserProfileUpdateInput{ProfileLinks: links})
			return err
		},
		"create": func() error {
			_, err := svc.Create(context.Background(), models.UserProfileCreateInput{UserID: "u1", ProfileType: "mentor", ProfileLinks: links})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			saved = nil
			if err := call(); err != nil {
				t.Fatalf("err = %v", err)
			}
			if string(saved) != `{"githubProfileLink":"https://github.com/x"}` {
				t.Fatalf("saved profile_links = %s", saved)
			}
		})
	}
}
