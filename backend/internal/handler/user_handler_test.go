// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

type stubUserSvc struct {
	getByID   func(context.Context, string) (*models.User, error)
	bootstrap func(context.Context, string, models.UserUpdateInput) (*models.User, error)
	update    func(context.Context, string, models.UserUpdateInput) (*models.User, error)
	delete    func(context.Context, string, string) error
}

func (s *stubUserSvc) GetByID(ctx context.Context, id string) (*models.User, error) {
	if s.getByID != nil {
		return s.getByID(ctx, id)
	}
	return &models.User{ID: id}, nil
}

func (s *stubUserSvc) Bootstrap(ctx context.Context, lfid string, input models.UserUpdateInput) (*models.User, error) {
	if s.bootstrap != nil {
		return s.bootstrap(ctx, lfid, input)
	}
	return &models.User{LFID: &lfid}, nil
}

func (s *stubUserSvc) Update(ctx context.Context, id string, input models.UserUpdateInput) (*models.User, error) {
	if s.update != nil {
		return s.update(ctx, id, input)
	}
	return &models.User{ID: id}, nil
}

func (s *stubUserSvc) Delete(ctx context.Context, id, actorID string) error {
	if s.delete != nil {
		return s.delete(ctx, id, actorID)
	}
	return nil
}

func TestUserHandler_GetMe_UsesPrincipalAsActor(t *testing.T) {
	svc := &stubUserSvc{
		getByID: func(_ context.Context, id string) (*models.User, error) {
			if id != "caller-user" {
				t.Fatalf("id = %q; want caller-user", id)
			}
			return &models.User{ID: id, Name: ptrString("Caller")}, nil
		},
	}

	h := handler.NewUserHandler(svc)
	r := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	r = r.WithContext(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: "caller-user"}))
	w := httptest.NewRecorder()

	h.GetMe(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	var body models.User
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ID != "caller-user" {
		t.Fatalf("body.ID = %q; want caller-user", body.ID)
	}
}

func TestUserHandler_UpdateMe_StoresPrincipalAsTarget(t *testing.T) {
	var gotID string
	var gotInput models.UserUpdateInput
	svc := &stubUserSvc{
		update: func(_ context.Context, id string, input models.UserUpdateInput) (*models.User, error) {
			gotID = id
			gotInput = input
			return &models.User{ID: id}, nil
		},
	}

	h := handler.NewUserHandler(svc)
	body, _ := json.Marshal(map[string]string{"name": "Updated Name"})
	r := httptest.NewRequest(http.MethodPatch, "/v1/me", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: "caller-user"}))
	w := httptest.NewRecorder()

	h.UpdateMe(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	if gotID != "caller-user" {
		t.Fatalf("gotID = %q; want caller-user", gotID)
	}
	if gotInput.Name == nil || *gotInput.Name != "Updated Name" {
		t.Fatalf("gotInput.Name = %#v; want Updated Name", gotInput.Name)
	}
}

func TestUserHandler_DeleteMe_RequiresPrincipalAndUsesSelf(t *testing.T) {
	var gotActor string
	svc := &stubUserSvc{
		delete: func(_ context.Context, id, actorID string) error {
			if id != "caller-user" || actorID != "caller-user" {
				return domain.ErrForbidden
			}
			gotActor = actorID
			return nil
		},
	}

	h := handler.NewUserHandler(svc)
	r := httptest.NewRequest(http.MethodDelete, "/v1/me", nil)
	r = r.WithContext(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: "caller-user"}))
	w := httptest.NewRecorder()

	h.DeleteMe(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("got %d; want 204", w.Code)
	}
	if gotActor != "caller-user" {
		t.Fatalf("gotActor = %q; want caller-user", gotActor)
	}
}

func TestUserHandler_UpdateMe_StripsLFIDFromPayload(t *testing.T) {
	var gotInput models.UserUpdateInput
	svc := &stubUserSvc{
		update: func(_ context.Context, id string, input models.UserUpdateInput) (*models.User, error) {
			gotInput = input
			return &models.User{ID: id}, nil
		},
	}
	h := handler.NewUserHandler(svc)
	body, _ := json.Marshal(map[string]string{"lfid": "evil-overwrite", "name": "Allowed"})
	r := httptest.NewRequest(http.MethodPatch, "/v1/me", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: "caller-user"}))
	w := httptest.NewRecorder()

	h.UpdateMe(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	if gotInput.LFID != nil {
		t.Fatalf("gotInput.LFID = %v; want nil", gotInput.LFID)
	}
}

func TestUserHandler_UpdateMe_StripsEmailFromPayload(t *testing.T) {
	var gotInput models.UserUpdateInput
	svc := &stubUserSvc{
		update: func(_ context.Context, id string, input models.UserUpdateInput) (*models.User, error) {
			gotInput = input
			return &models.User{ID: id}, nil
		},
	}
	h := handler.NewUserHandler(svc)
	body, _ := json.Marshal(map[string]string{"email": "victim@example.org", "name": "Allowed"})
	r := httptest.NewRequest(http.MethodPatch, "/v1/me", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: "caller-user", Email: "caller@example.org"}))
	w := httptest.NewRecorder()

	h.UpdateMe(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	if gotInput.Email != nil {
		t.Fatalf("gotInput.Email = %q; want nil", *gotInput.Email)
	}
}

func TestUserHandler_BootstrapMe_TakesEmailFromPrincipal(t *testing.T) {
	for name, tc := range map[string]struct {
		claim string
		want  *string
	}{
		"claim wins over the body": {claim: "caller@example.org", want: ptrString("caller@example.org")},
		"no claim stores no email": {claim: "", want: nil},
	} {
		t.Run(name, func(t *testing.T) {
			var gotInput models.UserUpdateInput
			svc := &stubUserSvc{
				bootstrap: func(_ context.Context, lfid string, input models.UserUpdateInput) (*models.User, error) {
					gotInput = input
					return &models.User{LFID: &lfid}, nil
				},
			}
			h := handler.NewUserHandler(svc)
			body, _ := json.Marshal(map[string]string{"email": "victim@example.org"})
			r := httptest.NewRequest(http.MethodPut, "/v1/me", bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r = r.WithContext(auth.ContextWithPrincipal(r.Context(), &models.Principal{Username: "caller", Email: tc.claim}))
			w := httptest.NewRecorder()

			h.BootstrapMe(w, r)

			if w.Code != http.StatusOK {
				t.Fatalf("got %d; want 200", w.Code)
			}
			if (gotInput.Email == nil) != (tc.want == nil) || (tc.want != nil && *gotInput.Email != *tc.want) {
				t.Fatalf("gotInput.Email = %v; want %v", gotInput.Email, tc.want)
			}
		})
	}
}

// The token's identity claims fill whatever the body leaves out, so a new user
// is stored with a name and avatar even when the client sends none (#360).
func TestUserHandler_BootstrapMe_FillsNameFromClaimsWhenBodyOmitsIt(t *testing.T) {
	principal := &models.Principal{
		Username: "caller", Email: "caller@example.org",
		Name: "Claim Name", GivenName: "Claim", FamilyName: "Name", Picture: "https://img.example.org/claim.png",
	}
	for name, tc := range map[string]struct {
		body map[string]string
		want models.UserUpdateInput
	}{
		"empty body takes every claim": {
			body: map[string]string{},
			want: models.UserUpdateInput{
				Email: ptrString("caller@example.org"), LFID: ptrString("caller"),
				Name: ptrString("Claim Name"), GivenName: ptrString("Claim"), FamilyName: ptrString("Name"), AvatarURL: ptrString("https://img.example.org/claim.png"),
			},
		},
		"body wins where set, claims fill the rest": {
			body: map[string]string{"name": "Body Name", "given_name": "Body", "email": "victim@example.org"},
			want: models.UserUpdateInput{
				Email: ptrString("caller@example.org"), LFID: ptrString("caller"),
				Name: ptrString("Body Name"), GivenName: ptrString("Body"), FamilyName: ptrString("Name"), AvatarURL: ptrString("https://img.example.org/claim.png"),
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			var gotInput models.UserUpdateInput
			svc := &stubUserSvc{bootstrap: func(_ context.Context, lfid string, input models.UserUpdateInput) (*models.User, error) {
				gotInput = input
				return &models.User{LFID: &lfid}, nil
			}}
			body, _ := json.Marshal(tc.body)
			r := httptest.NewRequest(http.MethodPut, "/v1/me", bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r = r.WithContext(auth.ContextWithPrincipal(r.Context(), principal))
			w := httptest.NewRecorder()

			handler.NewUserHandler(svc).BootstrapMe(w, r)

			if w.Code != http.StatusOK {
				t.Fatalf("got %d; want 200", w.Code)
			}
			if !reflect.DeepEqual(gotInput, tc.want) {
				t.Fatalf("input = %s; want %s", describeInput(gotInput), describeInput(tc.want))
			}
		})
	}
}

// Without claims and with an empty body, nothing is invented: the row keeps
// only the identity-bound fields.
func TestUserHandler_BootstrapMe_NoClaimsNoBodyStoresOnlyLFID(t *testing.T) {
	var gotInput models.UserUpdateInput
	svc := &stubUserSvc{bootstrap: func(_ context.Context, lfid string, input models.UserUpdateInput) (*models.User, error) {
		gotInput = input
		return &models.User{LFID: &lfid}, nil
	}}
	r := httptest.NewRequest(http.MethodPut, "/v1/me", bytes.NewReader([]byte(`{}`)))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(auth.ContextWithPrincipal(r.Context(), &models.Principal{Username: "caller"}))
	w := httptest.NewRecorder()

	handler.NewUserHandler(svc).BootstrapMe(w, r)

	if want := (models.UserUpdateInput{LFID: ptrString("caller")}); !reflect.DeepEqual(gotInput, want) {
		t.Fatalf("input = %s; want only lfid", describeInput(gotInput))
	}
}

func describeInput(in models.UserUpdateInput) string {
	b, _ := json.Marshal(in)
	return string(b)
}

func ptrString(s string) *string { return &s }
