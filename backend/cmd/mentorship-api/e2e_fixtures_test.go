// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build e2e

package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// day returns the UTC date offset days from today, as the enrollment form sends it.
func day(offset int) string {
	return time.Now().UTC().AddDate(0, 0, offset).Format(time.DateOnly)
}

// at returns the UTC midnight offset days from today, as the term routes take it.
func at(offset int) time.Time {
	return time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, offset)
}

// newProject registers a Project Service project and returns its UID.
func newProject(t *testing.T, slug string) string {
	t.Helper()
	uid := uuid.NewString()
	e2e.fakes.addProject(uid, fakeProject{Slug: slug, Name: strings.ToUpper(slug[:1]) + slug[1:], Logo: "https://projects.example.org/" + slug + ".png"})
	return uid
}

// openTermBody is an enrollment term whose application window is open today.
func openTermBody(name string) map[string]any {
	return map[string]any{
		"name": name, "startDate": day(20), "endDate": day(120),
		"applicationStartDate": day(-1), "applicationEndDate": day(10),
	}
}

// enrollmentBody is a complete POST /programs body for the given project.
func enrollmentBody(name, projectUID string) map[string]any {
	return map[string]any{
		"projectId":        projectUID,
		"name":             name,
		"description":      "Mentorship program " + name,
		"repositoryUrl":    "https://github.com/e2e/" + strings.ReplaceAll(strings.ToLower(name), " ", "-"),
		"websiteUrl":       "https://e2e.example.org",
		"codeOfConductUrl": "https://e2e.example.org/coc",
		"industry":         "Open Source",
		"skills":           []string{"Go", "Kubernetes"},
		"terms":            []any{openTermBody("Spring")},
		"prerequisites": []any{
			map[string]any{"name": "Introduce yourself", "description": "Tell us about you", "required": true, "requireFile": true, "dueDate": day(15)},
			map[string]any{"name": "Optional reading", "description": "Not cloned", "required": false},
		},
		"termsAccepted": true,
	}
}

type program struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Status string `json:"status"`
	// TermID is the program's first open term.
	TermID string `json:"-"`
	// Admin is the creator, an active program admin.
	Admin *actor `json:"-"`
}

// createProgram enrolls a pending program with one open term, created by admin.
func createProgram(t *testing.T, admin *actor, name string) *program {
	t.Helper()
	var p program
	admin.mustJSON(t, http.MethodPost, "/programs", enrollmentBody(name, newProject(t, "proj-"+uuid.NewString()[:8])), http.StatusCreated, &p)
	p.Admin = admin
	var terms struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	admin.mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/terms", nil, http.StatusOK, &terms)
	if len(terms.Data) == 0 {
		t.Fatalf("program %s has no terms", p.ID)
	}
	p.TermID = terms.Data[0].ID
	return &p
}

// approver is a member of the platform approver team; Heimdall alone authorizes decisions.
func approver(t *testing.T) *actor {
	return signIn(t, "lf-approver")
}

// publishProgram takes a new program through logo upload, submission and approval.
func publishProgram(t *testing.T, admin *actor, name string) *program {
	t.Helper()
	p := createProgram(t, admin, name)
	uploadLogo(t, admin, p.ID).expect(http.StatusCreated)
	admin.mustJSON(t, http.MethodPost, "/programs/"+p.ID+"/submit", nil, http.StatusOK, nil)
	approver(t).mustJSON(t, http.MethodPost, "/programs/"+p.ID+"/decision", map[string]any{"status": "published"}, http.StatusOK, p)
	return p
}

// pngBytes returns a small valid PNG.
func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func uploadLogo(t *testing.T, a *actor, programID string) *response {
	t.Helper()
	return a.send(t, http.MethodPost, "/programs/"+programID+"/logo-upload", bytes.NewReader(pngBytes(t)), "image/png", nil)
}
