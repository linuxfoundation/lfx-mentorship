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
}
