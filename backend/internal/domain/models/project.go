// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

// ProjectMetadata is the LF project identity a program stores alongside its project_uid.
type ProjectMetadata struct {
	Slug    string
	Name    string
	LogoURL *string // nil when the project has no logo
}
