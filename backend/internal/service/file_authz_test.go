// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

type accessCall struct{ object, relation, username string }

type fakeAccessChecker struct {
	allowed bool
	err     error
	calls   []accessCall
}

func (f *fakeAccessChecker) HasAccess(_ context.Context, object, relation, username string) (bool, error) {
	f.calls = append(f.calls, accessCall{object, relation, username})
	return f.allowed, f.err
}

func principalCtx(userID, username string) context.Context {
	return auth.ContextWithPrincipal(context.Background(), &models.Principal{UserID: userID, Username: username})
}

func adminRepo(adminUserID string, err error) *stubMemberRepo {
	return &stubMemberRepo{findActiveAdmin: func(_ context.Context, _, userID string) (*models.ProgramMember, error) {
		if err != nil {
			return nil, err
		}
		if userID == adminUserID {
			return &models.ProgramMember{UserID: userID, MemberType: models.MemberTypeProgramAdmin}, nil
		}
		return nil, domain.ErrProgramMemberNotFound
	}}
}

func TestFileService_AuthorizeProgramLogoUpload(t *testing.T) {
	dbDown := errors.New("db down")
	for _, tc := range []struct {
		name      string
		ctx       context.Context
		admins    *stubMemberRepo
		access    *fakeAccessChecker
		want      error
		wantCheck bool
	}{
		{name: "no principal", ctx: context.Background(), access: &fakeAccessChecker{allowed: true}, want: domain.ErrUnauthorized},
		{name: "direct program admin skips fga-sync", ctx: principalCtx("u1", "alice"), admins: adminRepo("u1", nil), access: &fakeAccessChecker{}},
		{name: "inherited writer", ctx: principalCtx("u2", "bob"), admins: adminRepo("u1", nil), access: &fakeAccessChecker{allowed: true}, wantCheck: true},
		{name: "not a writer", ctx: principalCtx("u2", "bob"), admins: adminRepo("u1", nil), access: &fakeAccessChecker{}, want: domain.ErrForbidden, wantCheck: true},
		{name: "not an admin and no fga-sync", ctx: principalCtx("u2", "bob"), admins: adminRepo("u1", nil), want: domain.ErrForbidden},
		{name: "fga-sync unreachable", ctx: principalCtx("u2", "bob"), admins: adminRepo("u1", nil), access: &fakeAccessChecker{err: errors.New("timeout")}, want: domain.ErrUpstreamUnavailable, wantCheck: true},
		{name: "membership lookup failure is not a denial", ctx: principalCtx("u1", "alice"), admins: adminRepo("", dbDown), access: &fakeAccessChecker{allowed: true}, want: dbDown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := (&fileSvcDeps{admins: tc.admins}).build()
			if tc.access != nil {
				svc.SetAccessChecker(tc.access)
			}
			err := svc.AuthorizeProgramLogoUpload(tc.ctx, "p1")
			if tc.want == nil && err != nil || tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("err = %v; want %v", err, tc.want)
			}
			if tc.access == nil {
				return
			}
			if !tc.wantCheck {
				if len(tc.access.calls) != 0 {
					t.Fatalf("fga-sync was asked %+v; want no check", tc.access.calls)
				}
				return
			}
			want := accessCall{"mentorship_program:p1", "writer", "bob"}
			if len(tc.access.calls) != 1 || tc.access.calls[0] != want {
				t.Fatalf("checks = %+v; want [%+v]", tc.access.calls, want)
			}
		})
	}
}

func TestFileService_AuthorizeProgramLogoUpload_M2MUsesFGAOnly(t *testing.T) {
	admins := &stubMemberRepo{findActiveAdmin: func(context.Context, string, string) (*models.ProgramMember, error) {
		t.Fatal("an M2M principal has no local membership to look up")
		return nil, nil
	}}
	access := &fakeAccessChecker{allowed: true}
	svc := (&fileSvcDeps{admins: admins}).build()
	svc.SetAccessChecker(access)

	if err := svc.AuthorizeProgramLogoUpload(principalCtx("svc@clients", "svc@clients"), "p1"); err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(access.calls) != 1 || access.calls[0].username != "svc@clients" {
		t.Fatalf("checks = %+v", access.calls)
	}
}
