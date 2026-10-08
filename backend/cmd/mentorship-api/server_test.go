// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFileTransfer_OutlivesServerWriteTimeout(t *testing.T) {
	const writeTimeout = 50 * time.Millisecond
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * writeTimeout)
		if err := r.Context().Err(); err != nil {
			t.Errorf("request context ended early: %v", err)
		}
		_, _ = io.WriteString(w, "done")
	})

	for name, tc := range map[string]struct {
		handler http.Handler
		wantOK  bool
	}{
		"plain route":   {handler: slow},
		"file transfer": {handler: fileTransfer(slow), wantOK: true},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewUnstartedServer(tc.handler)
			srv.Config.WriteTimeout = writeTimeout
			srv.Start()
			defer srv.Close()

			var body []byte
			resp, err := http.Get(srv.URL)
			if err == nil {
				body, err = io.ReadAll(resp.Body)
				_ = resp.Body.Close()
			}
			if gotOK := err == nil && string(body) == "done"; gotOK != tc.wantOK {
				t.Fatalf("completed = %v (body %q, err %v); want %v", gotOK, body, err, tc.wantOK)
			}
		})
	}
}
