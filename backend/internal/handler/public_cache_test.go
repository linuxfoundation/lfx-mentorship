// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
)

func TestPublicCacheMarksSuccessfulResponses(t *testing.T) {
	tests := []struct {
		name string
		next http.HandlerFunc
		want string
	}{
		{
			name: "explicit 200",
			next: func(w http.ResponseWriter, _ *http.Request) { handler.JSON(w, http.StatusOK, map[string]any{}) },
			want: handler.PublicCacheControl,
		},
		{
			name: "implicit 200 on first write",
			next: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("{}")) },
			want: handler.PublicCacheControl,
		},
		{
			name: "client error",
			next: func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "bad", http.StatusBadRequest) },
			want: "",
		},
		{
			name: "server error",
			next: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.PublicCache(tt.next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/programs", nil))

			if got := rec.Header().Get("Cache-Control"); got != tt.want {
				t.Fatalf("Cache-Control = %q, want %q", got, tt.want)
			}
		})
	}
}
