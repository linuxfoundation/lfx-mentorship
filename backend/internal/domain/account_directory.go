// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// AccountDirectory looks up LF accounts, including people who have never used Mentorship.
// Lookups return ErrAccountNotFound when no account matches.
type AccountDirectory interface {
	UsernameByEmail(ctx context.Context, email string) (string, error)
	Account(ctx context.Context, username string) (*models.LFAccount, error)
	PrimaryEmail(ctx context.Context, username string) (string, error)
}
