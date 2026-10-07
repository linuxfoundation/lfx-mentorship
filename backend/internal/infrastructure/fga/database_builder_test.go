// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package fga

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

type builderPrograms struct {
	domain.ProgramRepository
	program *models.Program
}

func (r builderPrograms) GetByID(context.Context, string) (*models.Program, error) {
	return r.program, nil
}

type builderMembers struct {
	domain.ProgramMemberRepository
	members []*models.ProgramMember
}

func (r builderMembers) ListByProgram(context.Context, string, models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error) {
	return r.members, &models.PaginationMeta{Total: len(r.members)}, nil
}

type builderUsers struct {
	domain.UserRepository
	users map[string]*models.User
}

func (r builderUsers) GetByID(_ context.Context, id string) (*models.User, error) {
	if user, ok := r.users[id]; ok {
		return user, nil
	}
	return nil, domain.ErrUserNotFound
}

type builderTerms struct{ domain.ProgramTermRepository }

func (builderTerms) GetByID(context.Context, string) (*models.ProgramTerm, error) {
	return &models.ProgramTerm{ID: "term-1", ProgramID: "program-1"}, nil
}

type builderApplications struct{ domain.ApplicationRepository }

func (builderApplications) GetByID(context.Context, string) (*models.Application, error) {
	return &models.Application{ID: "application-1", ProgramTermID: "term-1", UserID: "mentee"}, nil
}

func activeMember(userID string, memberType models.MemberType) *models.ProgramMember {
	status := models.ProgramMemberStatusActive
	return &models.ProgramMember{ProgramID: "program-1", UserID: userID, MemberType: memberType, Status: &status}
}

func newTestBuilder(users map[string]*models.User, members ...*models.ProgramMember) *DatabaseBuilder {
	projectUID := "project-1"
	return NewDatabaseBuilder(
		builderPrograms{program: &models.Program{ID: "program-1", ProjectUID: &projectUID}},
		builderMembers{members: members},
		builderUsers{users: users},
		builderTerms{},
		builderApplications{},
		nil,
		nil,
	)
}

func lfidUser(lfid string) *models.User { return &models.User{LFID: &lfid} }

var programMarker = domain.FGAOutboxMarker{MarkerKind: "object", ObjectType: "mentorship_program", ObjectUID: "program-1"}

func TestBuildProgramIncludesAllMemberLFIDs(t *testing.T) {
	builder := newTestBuilder(
		map[string]*models.User{"admin": lfidUser("admin-lfid"), "mentor": lfidUser("mentor-lfid")},
		activeMember("admin", models.MemberTypeProgramAdmin),
		activeMember("mentor", models.MemberTypeMentor),
	)

	message, err := builder.Build(context.Background(), programMarker)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	relations := message.Data.(AccessData).Relations
	if !reflect.DeepEqual(relations["writer"], []string{"admin-lfid"}) || !reflect.DeepEqual(relations["mentor"], []string{"mentor-lfid"}) {
		t.Fatalf("unexpected relations: %+v", relations)
	}
}

func TestBuildProgramFailsClosedOnMemberWithoutLFID(t *testing.T) {
	builder := newTestBuilder(
		map[string]*models.User{"admin": lfidUser("admin-lfid"), "unmapped": {}},
		activeMember("admin", models.MemberTypeProgramAdmin),
		activeMember("unmapped", models.MemberTypeProgramAdmin),
	)

	_, err := builder.Build(context.Background(), programMarker)
	if err == nil || !strings.Contains(err.Error(), "user unmapped has no LFID") {
		t.Fatalf("expected a missing-LFID error naming the user, got %v", err)
	}
}

func TestBuildProgramReportsUserLookupFailure(t *testing.T) {
	builder := newTestBuilder(map[string]*models.User{}, activeMember("missing", models.MemberTypeProgramAdmin))

	_, err := builder.Build(context.Background(), programMarker)
	if !errors.Is(err, domain.ErrUserNotFound) || !strings.Contains(err.Error(), "get user missing") {
		t.Fatalf("expected a wrapped user lookup error naming the user, got %v", err)
	}
}

func TestBuildApplicationFailsClosedOnApplicantWithoutLFID(t *testing.T) {
	builder := newTestBuilder(map[string]*models.User{"mentee": {}})

	_, err := builder.Build(context.Background(), domain.FGAOutboxMarker{MarkerKind: "object", ObjectType: "mentorship_application", ObjectUID: "application-1"})
	if err == nil || !strings.Contains(err.Error(), "user mentee has no LFID") {
		t.Fatalf("expected a missing-LFID error naming the applicant, got %v", err)
	}
}
