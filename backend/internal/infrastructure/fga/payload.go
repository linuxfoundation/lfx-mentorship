// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package fga

import (
	"errors"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

const (
	programObject         = "mentorship_program"
	applicationObject     = "mentorship_application"
	taskObject            = "mentorship_task"
	approverTeamObject    = "mentorship_approver_team"
	updateAccessOperation = "update_access"
	deleteAccessOperation = "delete_access"
	memberPutOperation    = "member_put"
	memberRemoveOperation = "member_remove"
)

var errMissingProjectUID = errors.New("program project_uid is required for FGA access")

// Message is the generic envelope consumed by lfx-v2-fga-sync.
type Message struct {
	ObjectType string `json:"object_type"`
	Operation  string `json:"operation"`
	Data       any    `json:"data"`
}

// AccessData is the payload for update_access.
type AccessData struct {
	UID              string              `json:"uid"`
	Public           bool                `json:"public"`
	Relations        map[string][]string `json:"relations"`
	References       map[string][]string `json:"references,omitempty"`
	ExcludeRelations []string            `json:"exclude_relations,omitempty"`
}

// DeleteData is the payload for delete_access.
type DeleteData struct {
	UID string `json:"uid"`
}

// MemberData is the payload for member_put and member_remove.
type MemberData struct {
	UID                   string   `json:"uid"`
	Username              string   `json:"username"`
	Relations             []string `json:"relations"`
	MutuallyExclusiveWith []string `json:"mutually_exclusive_with,omitempty"`
}

// ProgramAccess builds a complete current-state program access message.
func ProgramAccess(program *models.Program, writers, mentors []string) (Message, error) {
	if program == nil || program.ID == "" {
		return Message{}, errors.New("program id is required for FGA access")
	}
	if program.ProjectUID == nil || *program.ProjectUID == "" {
		return Message{}, errMissingProjectUID
	}
	return Message{
		ObjectType: programObject,
		Operation:  updateAccessOperation,
		Data: AccessData{
			UID:    program.ID,
			Public: program.Status == models.ProgramStatusPublished,
			Relations: map[string][]string{
				"writer": writers,
				"mentor": mentors,
			},
			References: map[string][]string{
				"project":                    {*program.ProjectUID},
				"global_mentorship_approver": {"mentorship_approver_team:global#member"},
			},
		},
	}, nil
}

// ApplicationAccess builds the current-state application access message.
func ApplicationAccess(application *models.Application, programID, applicantLFID string) (Message, error) {
	if application == nil || application.ID == "" || programID == "" || applicantLFID == "" {
		return Message{}, errors.New("application, program, and applicant IDs are required for FGA access")
	}
	return Message{
		ObjectType: applicationObject,
		Operation:  updateAccessOperation,
		Data: AccessData{
			UID:        application.ID,
			Relations:  map[string][]string{"mentee": {applicantLFID}},
			References: map[string][]string{"mentorship_program": {programID}},
		},
	}, nil
}

// TaskAccess builds the current-state task access message.
func TaskAccess(task *models.Task, applicationID, assigneeLFID string) (Message, error) {
	if task == nil || task.ID == "" || applicationID == "" || assigneeLFID == "" {
		return Message{}, errors.New("task, application, and assignee IDs are required for FGA access")
	}
	return Message{
		ObjectType: taskObject,
		Operation:  updateAccessOperation,
		Data: AccessData{
			UID:        task.ID,
			Relations:  map[string][]string{"assignee": {assigneeLFID}},
			References: map[string][]string{"mentorship_application": {applicationID}},
		},
	}, nil
}

// DeleteAccess builds a delete_access message for a resource.
func DeleteAccess(objectType, uid string) (Message, error) {
	if objectType == "" || uid == "" {
		return Message{}, errors.New("object type and uid are required for FGA deletion")
	}
	return Message{ObjectType: objectType, Operation: deleteAccessOperation, Data: DeleteData{UID: uid}}, nil
}

// MemberPut builds a precise relation grant for a resource member.
func MemberPut(objectType, uid, username string, relations []string) (Message, error) {
	if objectType == "" || uid == "" || username == "" || len(relations) == 0 {
		return Message{}, errors.New("object type, uid, username, and relations are required for FGA membership")
	}
	return Message{ObjectType: objectType, Operation: memberPutOperation, Data: MemberData{UID: uid, Username: username, Relations: relations}}, nil
}

// MemberRemove builds a precise relation removal for a resource member.
func MemberRemove(objectType, uid, username, relation string) (Message, error) {
	if objectType == "" || uid == "" || username == "" || relation == "" {
		return Message{}, errors.New("object type, uid, username, and relation are required for FGA membership removal")
	}
	return Message{ObjectType: objectType, Operation: memberRemoveOperation, Data: MemberData{UID: uid, Username: username, Relations: []string{relation}}}, nil
}

// ApproverTeamObject returns the fixed object type used for approver membership.
func ApproverTeamObject() string { return approverTeamObject }
