// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package fga

import (
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

func TestProgramAccessIncludesProjectAndApproverReferences(t *testing.T) {
	projectUID := "project-1"
	program := &models.Program{ID: "program-1", ProjectUID: &projectUID, Status: models.ProgramStatusPublished}

	message, err := ProgramAccess(program, []string{"admin-1"}, []string{"mentor-1"})
	if err != nil {
		t.Fatalf("ProgramAccess: %v", err)
	}

	data := message.Data.(AccessData)
	if message.ObjectType != "mentorship_program" || message.Operation != "update_access" {
		t.Fatalf("unexpected envelope: %+v", message)
	}
	if !data.Public || data.References["project"][0] != projectUID {
		t.Fatalf("unexpected program access data: %+v", data)
	}
	if data.References["auditor"][0] != "mentorship_approver_team:global#member" {
		t.Fatalf("approver userset must be a reference: %+v", data.References)
	}
}

func TestProgramAccessRequiresProjectUID(t *testing.T) {
	_, err := ProgramAccess(&models.Program{ID: "program-1"}, nil, nil)
	if err != errMissingProjectUID {
		t.Fatalf("expected missing project UID error, got %v", err)
	}
}

func TestMemberRemoveRequiresPreciseRelation(t *testing.T) {
	message, err := MemberRemove("mentorship_program", "program-1", "mentor-1", "mentor")
	if err != nil {
		t.Fatalf("MemberRemove: %v", err)
	}
	data := message.Data.(MemberData)
	if len(data.Relations) != 1 || data.Relations[0] != "mentor" {
		t.Fatalf("unexpected membership data: %+v", data)
	}
}
