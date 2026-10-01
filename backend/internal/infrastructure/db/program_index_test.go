// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

func TestNewProgramIndexDocumentIncludesProjectMapping(t *testing.T) {
	projectSlug := "platform-project"
	projectName := "Platform Project"
	document := NewProgramIndexDocument(&models.Program{
		ID:          "program-1",
		ProjectSlug: &projectSlug,
		ProjectName: &projectName,
	})
	if document.ProjectSlug == nil || *document.ProjectSlug != projectSlug || document.ProjectName == nil || *document.ProjectName != projectName {
		t.Fatalf("project mapping = %#v", document)
	}
}

func TestNewProgramIndexConfigSupportsQueryServiceDirectGrantFilter(t *testing.T) {
	const id = "00000000-0000-0000-0000-000000000010"
	config := NewProgramIndexConfig(id, nil, "Program", "program", "published")
	if _, ok := config["object_ref"]; ok {
		t.Fatalf("object_ref must be derived, got %v", config["object_ref"])
	}
	if _, ok := config["object_type"]; ok {
		t.Fatalf("object_type must be derived, got %v", config["object_type"])
	}
	if config["access_check_object"] != "mentorship_program:"+id || config["access_check_relation"] != "viewer" {
		t.Fatalf("access metadata=%v/%v", config["access_check_object"], config["access_check_relation"])
	}
}
