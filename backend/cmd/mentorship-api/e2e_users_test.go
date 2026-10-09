// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build e2e

package main

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestE2EProbes(t *testing.T) {
	reset(t)
	for _, path := range []string{"/livez", "/healthz", "/readyz", "/internal/metrics"} {
		res, err := http.Get(e2e.baseURL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: got %d; want 200", path, res.StatusCode)
		}
	}
}

func TestE2EGatewayAuthentication(t *testing.T) {
	reset(t)
	signIn(t, "ada")

	t.Run("anonymous caller is rejected on an authenticated route", func(t *testing.T) {
		res := anonymous(t).call(http.MethodGet, "/me", nil).expect(http.StatusUnauthorized)
		if msg := res.errorMessage(); !strings.Contains(msg, "principal is required") {
			t.Fatalf("got error %q", msg)
		}
	})

	t.Run("anonymous caller reads public routes", func(t *testing.T) {
		anonymous(t).call(http.MethodGet, "/programs", nil).expect(http.StatusOK)
	})

	bad := map[string]tokenClaims{
		"expired": func() tokenClaims {
			c := validClaims("ada", "", "")
			c.ExpiresAt = time.Now().Add(-time.Hour).Unix()
			return c
		}(),
		"wrong issuer":   func() tokenClaims { c := validClaims("ada", "", ""); c.Issuer = "someone-else"; return c }(),
		"wrong audience": func() tokenClaims { c := validClaims("ada", "", ""); c.Audience = "other-service"; return c }(),
		"no principal":   validClaims("", "", ""),
	}
	for name, claims := range bad {
		t.Run(name+" token is rejected", func(t *testing.T) {
			a := &actor{t: t, token: mintToken(t, claims)}
			// Validation runs before routing, so even a public route refuses a bad token.
			a.call(http.MethodGet, "/programs", nil).expect(http.StatusUnauthorized)
		})
	}

	t.Run("malformed authorization header is rejected", func(t *testing.T) {
		a := anonymous(t)
		a.send(http.MethodGet, "/programs", nil, "", map[string]string{"Authorization": "Basic abc"}).expect(http.StatusUnauthorized)
	})

	t.Run("valid token for an unknown user is rejected until PUT /me", func(t *testing.T) {
		stranger := unprovisioned(t, "grace")
		res := stranger.call(http.MethodGet, "/me", nil).expect(http.StatusUnauthorized)
		if msg := res.errorMessage(); !strings.Contains(msg, "not provisioned") {
			t.Fatalf("got error %q", msg)
		}
	})
}

func TestE2EUsers(t *testing.T) {
	reset(t)

	ada := unprovisioned(t, "ada")
	var created struct {
		ID         string `json:"id"`
		LFID       string `json:"lfid"`
		Email      string `json:"email"`
		Name       string `json:"name"`
		GivenName  string `json:"given_name"`
		FamilyName string `json:"family_name"`
	}
	ada.mustJSON(http.MethodPut, "/me", map[string]any{"name": "Ada Lovelace", "given_name": "Ada", "lfid": "mallory"}, http.StatusOK, &created)
	if created.ID == "" || created.LFID != "ada" || created.Email != ada.Email || created.Name != "Ada Lovelace" {
		t.Fatalf("bootstrap: got %+v", created)
	}
	ada.ID = created.ID

	t.Run("bootstrap is idempotent and keeps the user ID", func(t *testing.T) {
		var again struct {
			ID         string `json:"id"`
			FamilyName string `json:"family_name"`
			Name       string `json:"name"`
		}
		ada.mustJSON(http.MethodPut, "/me", map[string]any{"family_name": "Lovelace"}, http.StatusOK, &again)
		if again.ID != ada.ID || again.FamilyName != "Lovelace" || again.Name != "Ada Lovelace" {
			t.Fatalf("re-bootstrap: got %+v", again)
		}
	})

	t.Run("GET /me returns the caller", func(t *testing.T) {
		var me struct {
			ID   string `json:"id"`
			LFID string `json:"lfid"`
		}
		ada.mustJSON(http.MethodGet, "/me", nil, http.StatusOK, &me)
		if me.ID != ada.ID || me.LFID != "ada" {
			t.Fatalf("got %+v", me)
		}
	})

	t.Run("PATCH /me updates fields but never the LFID", func(t *testing.T) {
		var updated struct {
			LFID      string `json:"lfid"`
			AvatarURL string `json:"avatar_url"`
		}
		ada.mustJSON(http.MethodPatch, "/me", map[string]any{"avatar_url": "https://img.example.org/ada.png", "lfid": "mallory"}, http.StatusOK, &updated)
		if updated.LFID != "ada" || updated.AvatarURL != "https://img.example.org/ada.png" {
			t.Fatalf("got %+v", updated)
		}
	})

	t.Run("PATCH /me rejects a malformed body", func(t *testing.T) {
		ada.send(http.MethodPatch, "/me", strings.NewReader("{"), "application/json", nil).expect(http.StatusBadRequest)
	})

	t.Run("bootstrap with an email another user holds conflicts", func(t *testing.T) {
		grace := unprovisioned(t, "grace")
		grace.call(http.MethodPut, "/me", map[string]any{"email": ada.Email}).expect(http.StatusConflict)
	})

	t.Run("DELETE /me removes the caller", func(t *testing.T) {
		ada.call(http.MethodDelete, "/me", nil).expect(http.StatusNoContent)
		ada.call(http.MethodGet, "/me", nil).expect(http.StatusUnauthorized)
		if n := dbCount(t, "SELECT count(*) FROM users WHERE lfid = 'ada'"); n != 0 {
			t.Fatalf("user row still present")
		}
	})
}
