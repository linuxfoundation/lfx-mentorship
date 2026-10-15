// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package auth provides JWT validation and invite-token helpers.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidInviteToken is returned when the token signature or payload is invalid.
var ErrInvalidInviteToken = errors.New("invalid invite token")

// ErrNoInviteSecret is returned when an invite token is signed or verified with
// an empty secret. HMAC accepts a zero-length key, so without this guard
// ValidateInviteToken would verify against sign(payload, "") — a signature any
// caller can compute, making the public accept/decline endpoints forgeable.
// MENTOR_INVITE_SECRET is required at startup (cmd/mentorship-api/config.go), so
// this is defence in depth: it keeps the token path failing closed if that check
// is ever relaxed.
var ErrNoInviteSecret = errors.New("invite secret is not configured")

// Invite tokens do not expire: accepting or declining needs the invitee signed in
// and their mentor row still invited, so an old link is harmless.
type inviteClaims struct {
	ProgramID string `json:"program_id"`
	UserID    string `json:"user_id"`
}

// GenerateInviteToken creates a signed HMAC-SHA256 invite token for the given mentor.
func GenerateInviteToken(programID, userID, secret string) (string, error) {
	if secret == "" {
		return "", ErrNoInviteSecret
	}
	claims := inviteClaims{
		ProgramID: programID,
		UserID:    userID,
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal invite claims: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	sig := sign(encoded, secret)
	return encoded + "." + sig, nil
}

// ValidateInviteToken parses and verifies a token produced by GenerateInviteToken.
// On success it returns the programID and userID embedded in the token.
func ValidateInviteToken(token, secret string) (programID, userID string, err error) {
	if secret == "" {
		return "", "", ErrNoInviteSecret
	}
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return "", "", ErrInvalidInviteToken
	}
	encoded, givenSig := parts[0], parts[1]

	expectedSig := sign(encoded, secret)
	if !hmac.Equal([]byte(givenSig), []byte(expectedSig)) {
		return "", "", ErrInvalidInviteToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", ErrInvalidInviteToken
	}

	var claims inviteClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", "", ErrInvalidInviteToken
	}
	return claims.ProgramID, claims.UserID, nil
}

func sign(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
