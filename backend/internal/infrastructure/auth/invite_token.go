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

// InviteClaims are what an invite token names: the program, and either the
// invited user or, for an invitation sent by email to someone with no LF account
// yet, the mentor row that holds the address. Exactly one of UserID and MemberID
// is set.
//
// Invite tokens do not expire: accepting or declining needs the invitee signed in
// and their mentor row still invited, so an old link is harmless.
type InviteClaims struct {
	ProgramID string `json:"program_id"`
	UserID    string `json:"user_id,omitempty"`
	MemberID  string `json:"member_id,omitempty"`
}

// GenerateInviteToken creates a signed HMAC-SHA256 invite token for the given mentor.
func GenerateInviteToken(programID, userID, secret string) (string, error) {
	return generateInviteToken(InviteClaims{ProgramID: programID, UserID: userID}, secret)
}

// GenerateEmailInviteToken creates a signed invite token for an email-only mentor
// row, which has no user until the invitee accepts.
func GenerateEmailInviteToken(programID, memberID, secret string) (string, error) {
	return generateInviteToken(InviteClaims{ProgramID: programID, MemberID: memberID}, secret)
}

// generateInviteToken signs claims. A user token's payload omits member_id, so it
// is byte-for-byte the token earlier releases issued for the same invite.
func generateInviteToken(claims InviteClaims, secret string) (string, error) {
	if secret == "" {
		return "", ErrNoInviteSecret
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
// On success it returns the programID and userID embedded in the token; an email
// invite token is rejected, since it names no user.
func ValidateInviteToken(token, secret string) (programID, userID string, err error) {
	claims, err := ParseInviteToken(token, secret)
	if err != nil {
		return "", "", err
	}
	if claims.UserID == "" {
		return "", "", ErrInvalidInviteToken
	}
	return claims.ProgramID, claims.UserID, nil
}

// ParseInviteToken verifies a token produced by GenerateInviteToken or
// GenerateEmailInviteToken and returns its claims.
func ParseInviteToken(token, secret string) (*InviteClaims, error) {
	if secret == "" {
		return nil, ErrNoInviteSecret
	}
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, ErrInvalidInviteToken
	}
	encoded, givenSig := parts[0], parts[1]

	expectedSig := sign(encoded, secret)
	if !hmac.Equal([]byte(givenSig), []byte(expectedSig)) {
		return nil, ErrInvalidInviteToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, ErrInvalidInviteToken
	}

	var claims InviteClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrInvalidInviteToken
	}
	if claims.ProgramID == "" || (claims.UserID == "") == (claims.MemberID == "") {
		return nil, ErrInvalidInviteToken
	}
	return &claims, nil
}

func sign(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
