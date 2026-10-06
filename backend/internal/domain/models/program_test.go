// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models_test

import (
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

func TestProgramPubliclyVisibleAt(t *testing.T) {
	const (
		id   = "3f2b9c1e-8d4a-4b6f-9e21-5c7a0d9ea71d"
		slug = "my-program"
	)
	tests := []struct {
		status models.ProgramStatus
		ref    string
		want   bool
	}{
		{models.ProgramStatusPublished, id, true},
		{models.ProgramStatusPublished, slug, true},
		{models.ProgramStatusDraft, id, true},
		{models.ProgramStatusDraft, "3F2B9C1E-8D4A-4B6F-9E21-5C7A0D9EA71D", true},
		{models.ProgramStatusDraft, slug, false},
		{models.ProgramStatusSubmitted, id, false},
		{models.ProgramStatusHidden, id, false},
		{models.ProgramStatusRejected, id, false},
		{models.ProgramStatusArchived, id, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.status)+"/"+tt.ref, func(t *testing.T) {
			program := &models.Program{ID: id, Slug: slug, Status: tt.status}
			if got := program.PubliclyVisibleAt(tt.ref); got != tt.want {
				t.Errorf("PubliclyVisibleAt(%q) = %v; want %v", tt.ref, got, tt.want)
			}
		})
	}
}
