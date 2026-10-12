// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package auth_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

const testSecret = "super-secret-for-tests"

func TestGenerateAndValidateInviteToken_RoundTrip(t *testing.T) {
	token, err := auth.GenerateInviteToken("prog-1", "user-1", testSecret)
	if err != nil {
		t.Fatalf("GenerateInviteToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	gotProgram, gotUser, err := auth.ValidateInviteToken(token, testSecret)
	if err != nil {
		t.Fatalf("ValidateInviteToken: %v", err)
	}
	if gotProgram != "prog-1" {
		t.Errorf("programID = %q; want %q", gotProgram, "prog-1")
	}
	if gotUser != "user-1" {
		t.Errorf("userID = %q; want %q", gotUser, "user-1")
	}
}

func TestGenerateInviteToken_EmptySecret(t *testing.T) {
	_, err := auth.GenerateInviteToken("prog-1", "user-1", "")
	if !errors.Is(err, auth.ErrNoInviteSecret) {
		t.Errorf("expected ErrNoInviteSecret, got %v", err)
	}
}

func TestValidateInviteToken_EmptySecret(t *testing.T) {
	token, err := auth.GenerateInviteToken("prog-1", "user-1", testSecret)
	if err != nil {
		t.Fatalf("GenerateInviteToken: %v", err)
	}
	_, _, err = auth.ValidateInviteToken(token, "")
	if !errors.Is(err, auth.ErrNoInviteSecret) {
		t.Errorf("expected ErrNoInviteSecret, got %v", err)
	}
}

// HMAC accepts a zero-length key, so an unconfigured secret would otherwise let
// anyone mint a token that verifies: the signature is sign(payload, "") and
// needs no secret to compute. This is the case the empty-secret guard exists to
// prevent, so it is asserted directly rather than only through the round trip.
func TestValidateInviteToken_EmptySecretRejectsForgedToken(t *testing.T) {
	forged, err := signToken(t, "", "prog-1", "user-1", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("forge token: %v", err)
	}
	if _, _, err := auth.ValidateInviteToken(forged, ""); !errors.Is(err, auth.ErrNoInviteSecret) {
		t.Errorf("forged token accepted or wrong error: got %v, want ErrNoInviteSecret", err)
	}
}

// signToken builds a token carrying an exp claim, as tokens issued before expiry
// was dropped do. An empty secret is what an attacker could sign with.
func signToken(t *testing.T, secret, programID, userID string, expires time.Time) (string, error) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"program_id": programID,
		"user_id":    userID,
		"exp":        expires.Unix(),
	})
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func TestValidateInviteToken_WrongSecret(t *testing.T) {
	token, err := auth.GenerateInviteToken("prog-1", "user-1", testSecret)
	if err != nil {
		t.Fatalf("GenerateInviteToken: %v", err)
	}
	_, _, err = auth.ValidateInviteToken(token, "wrong-secret")
	if !errors.Is(err, auth.ErrInvalidInviteToken) {
		t.Errorf("expected ErrInvalidInviteToken, got %v", err)
	}
}

func TestValidateInviteToken_Malformed(t *testing.T) {
	cases := []string{"", "nodot", "a.b.c"}
	for _, tc := range cases {
		_, _, err := auth.ValidateInviteToken(tc, testSecret)
		if !errors.Is(err, auth.ErrInvalidInviteToken) {
			t.Errorf("token %q: expected ErrInvalidInviteToken, got %v", tc, err)
		}
	}
}

func TestValidateInviteToken_OldLinkWithPastExpiryStaysValid(t *testing.T) {
	token, err := signToken(t, testSecret, "prog-1", "user-1", time.Now().Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	gotProgram, gotUser, err := auth.ValidateInviteToken(token, testSecret)
	if err != nil || gotProgram != "prog-1" || gotUser != "user-1" {
		t.Errorf("got %q %q %v; want prog-1 user-1 and no error", gotProgram, gotUser, err)
	}
}

func TestGenerateInviteToken_SameInviteSameToken(t *testing.T) {
	first, _ := auth.GenerateInviteToken("prog-1", "user-1", testSecret)
	second, _ := auth.GenerateInviteToken("prog-1", "user-1", testSecret)
	if first != second {
		t.Error("re-inviting the same person must not invalidate the link they already have")
	}
}
