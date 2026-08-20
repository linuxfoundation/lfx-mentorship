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
	"time"
)

// ErrInvalidInviteToken is returned when the token signature or payload is invalid.
var ErrInvalidInviteToken = errors.New("invalid invite token")

// inviteTokenTTL is how long mentor invite tokens remain valid.
const inviteTokenTTL = 7 * 24 * time.Hour

type inviteClaims struct {
	ProgramID string `json:"program_id"`
	UserID    string `json:"user_id"`
	ExpiresAt int64  `json:"exp"`
}

// GenerateInviteToken creates a signed HMAC-SHA256 invite token for the given mentor.
func GenerateInviteToken(programID, userID, secret string) (string, error) {
	claims := inviteClaims{
		ProgramID: programID,
		UserID:    userID,
		ExpiresAt: time.Now().Add(inviteTokenTTL).Unix(),
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
	if time.Now().Unix() > claims.ExpiresAt {
		return "", "", fmt.Errorf("%w: token expired", ErrInvalidInviteToken)
	}
	return claims.ProgramID, claims.UserID, nil
}

func sign(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
