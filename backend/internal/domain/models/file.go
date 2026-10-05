// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

// UploadedFile is the metadata an upload route returns. It never carries a private object key.
type UploadedFile struct {
	// PublicURL is the CDN URL of a public-class file; empty for private files.
	PublicURL   string `json:"public_url,omitempty"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}
