// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// ProjectLookup resolves LF project metadata from lfx-v2-project-service, the source of truth for projects.
type ProjectLookup interface {
	// GetProject returns ErrProjectNotFound when no project has the UID,
	// and ErrUpstreamUnavailable when Project Service cannot answer.
	GetProject(ctx context.Context, uid string) (*models.ProjectMetadata, error)
	// GetParentUIDs returns the parent UID of each project in uids that
	// Project Service knows, keyed by project UID: "" for a top-level project,
	// and no entry for a UID it does not know. It returns
	// ErrUpstreamUnavailable when Project Service cannot answer.
	GetParentUIDs(ctx context.Context, uids []string) (map[string]string, error)
}
