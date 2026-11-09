// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"log/slog"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

// The FGA object and the relation Heimdall checks on the first request a
// client makes after each create: the program's settings routes, and the
// mentee overview's read of the new application's tasks.
const (
	programFGAObjectPrefix     = "mentorship_program:"
	programWriterRelation      = "writer"
	programCreatorRelation     = programWriterRelation
	applicationFGAObjectPrefix = "mentorship_application:"
	applicationCreatorRelation = "auditor"
)

// awaitCreatorAccess holds a create's response until the creator's relation on
// the new object reaches OpenFGA, so the client's next request on it (a program
// settings change, a read of a new application) passes the gateway instead of getting a
// 403 while the outbox catches up. The object is already saved, so a timeout is
// logged and the response goes out anyway; the client's retry covers the rest.
func awaitCreatorAccess(ctx context.Context, waiter domain.AccessWaiter, object, relation string) {
	if waiter == nil {
		return
	}
	principal := auth.PrincipalFromContext(ctx)
	if principal == nil || principal.Username == "" || principal.IsM2M() {
		return
	}
	if err := waiter.WaitForAccess(ctx, object, relation, principal.Username); err != nil {
		slog.WarnContext(ctx, "creator access not visible yet; responding anyway", "object", object, "relation", relation, "error", err)
	}
}
