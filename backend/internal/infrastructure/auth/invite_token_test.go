// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package auth_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
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
	return signEncoded(secret, base64.RawURLEncoding.EncodeToString(payload)), nil
}

// signEncoded appends a valid signature to an already encoded payload, so a
// test can reach the checks that run after the signature is verified.
func signEncoded(secret, encoded string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
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

func TestEmailInviteToken_RoundTrip(t *testing.T) {
	token, err := auth.GenerateEmailInviteToken("prog-1", "member-1", testSecret)
	if err != nil {
		t.Fatalf("GenerateEmailInviteToken: %v", err)
	}
	claims, err := auth.ParseInviteToken(token, testSecret)
	if err != nil {
		t.Fatalf("ParseInviteToken: %v", err)
	}
	if claims.ProgramID != "prog-1" || claims.MemberID != "member-1" || claims.UserID != "" {
		t.Errorf("claims = %+v; want prog-1 and member-1 with no user", claims)
	}
}

func TestValidateInviteToken_RejectsEmailInviteToken(t *testing.T) {
	token, err := auth.GenerateEmailInviteToken("prog-1", "member-1", testSecret)
	if err != nil {
		t.Fatalf("GenerateEmailInviteToken: %v", err)
	}
	if _, _, err := auth.ValidateInviteToken(token, testSecret); !errors.Is(err, auth.ErrInvalidInviteToken) {
		t.Errorf("err = %v; want ErrInvalidInviteToken for a token that names no user", err)
	}
}

// A correctly signed token must still name a program and exactly one of a user
// or an email invitation's member row.
func TestParseInviteToken_RejectsIncompleteClaims(t *testing.T) {
	for name, claims := range map[string]map[string]string{
		"no program":              {"user_id": "user-1"},
		"neither user nor member": {"program_id": "prog-1"},
		"both user and member":    {"program_id": "prog-1", "user_id": "user-1", "member_id": "member-1"},
	} {
		payload, err := json.Marshal(claims)
		if err != nil {
			t.Fatalf("%s: marshal: %v", name, err)
		}
		token := signEncoded(testSecret, base64.RawURLEncoding.EncodeToString(payload))
		if _, err := auth.ParseInviteToken(token, testSecret); !errors.Is(err, auth.ErrInvalidInviteToken) {
			t.Errorf("%s: err = %v; want ErrInvalidInviteToken", name, err)
		}
	}
}

func TestParseInviteToken_RejectsUndecodablePayload(t *testing.T) {
	for name, encoded := range map[string]string{
		"not base64": "not*base64",
		"not JSON":   base64.RawURLEncoding.EncodeToString([]byte("not json")),
	} {
		if _, err := auth.ParseInviteToken(signEncoded(testSecret, encoded), testSecret); !errors.Is(err, auth.ErrInvalidInviteToken) {
			t.Errorf("%s: err = %v; want ErrInvalidInviteToken", name, err)
		}
	}
}

func TestParseInviteToken_UserTokenUnchanged(t *testing.T) {
	token, err := auth.GenerateInviteToken("prog-1", "user-1", testSecret)
	if err != nil {
		t.Fatalf("GenerateInviteToken: %v", err)
	}
	claims, err := auth.ParseInviteToken(token, testSecret)
	if err != nil {
		t.Fatalf("ParseInviteToken: %v", err)
	}
	if claims.UserID != "user-1" || claims.MemberID != "" {
		t.Errorf("claims = %+v; want user-1 with no member", claims)
	}
	payload, _, _ := strings.Cut(token, ".")
	if decoded, _ := base64.RawURLEncoding.DecodeString(payload); strings.Contains(string(decoded), "member_id") {
		t.Errorf("a user token payload must not change: %s", decoded)
	}
}
