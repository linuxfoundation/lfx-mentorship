// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler

import "net/http"

// PublicCacheControl is the Cache-Control value for the fully public
// collections and aggregates. It matches what Query Service sends for
// public documents (docs/rewrite/05-heimdall-gateway.md).
const PublicCacheControl = "public, max-age=300"

// PublicCache marks successful responses as publicly cacheable. Apply it only
// to routes whose response does not depend on the caller.
func PublicCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&publicCacheWriter{ResponseWriter: w}, r)
	})
}

type publicCacheWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *publicCacheWriter) WriteHeader(status int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		if status >= 200 && status < 300 {
			w.Header().Set("Cache-Control", PublicCacheControl)
		}
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *publicCacheWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *publicCacheWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
