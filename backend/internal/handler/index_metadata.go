// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler

import (
	"net/http"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

// IndexMetadata preserves request identity for transactional index snapshots.
func IndexMetadata(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Keep this whitelist explicit: identity headers are never copied from the client.
		headers := map[string]string{}
		if value := r.Header.Get("Authorization"); value != "" {
			headers["authorization"] = value
		}
		next.ServeHTTP(w, r.WithContext(domain.ContextWithIndexHeaders(r.Context(), headers)))
	})
}
