// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import "context"

// AccessWaiter waits for a relation written through the FGA outbox to become
// visible to authorization checks. The outbox writes tuples asynchronously, so
// without it a client's first request on an object it just created can be
// denied at the gateway.
type AccessWaiter interface {
	// WaitForAccess returns nil once user:{username} has relation on object
	// (for example "mentorship_program:{uid}"), or an error when ctx ends first
	// or the check cannot be made.
	WaitForAccess(ctx context.Context, object, relation, username string) error
}

// AccessChecker reports whether user:{username} has relation on object in
// OpenFGA, through fga-sync's cached check.
type AccessChecker interface {
	HasAccess(ctx context.Context, object, relation, username string) (bool, error)
}
