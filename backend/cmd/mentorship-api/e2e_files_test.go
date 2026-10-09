// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build e2e

package main

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/johannesboyne/gofakes3"
)

func pdfBytes() []byte {
	return []byte("%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\ntrailer << /Root 1 0 R >>\n%%EOF\n")
}

// uploadTaskFile posts data as the "file" part of a multipart body.
func uploadTaskFile(t *testing.T, a *actor, taskID, filename string, data []byte) *response {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("note", "ignored")
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = part.Write(data)
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return a.send(http.MethodPost, "/tasks/"+taskID+"/file-upload", &body, w.FormDataContentType(), nil)
}

// s3Keys returns the object keys in bucket.
func s3Keys(t *testing.T, bucket string) []string {
	t.Helper()
	list, err := e2e.s3.ListBucket(bucket, nil, gofakes3.ListBucketPage{})
	if err != nil {
		t.Fatalf("list bucket %s: %v", bucket, err)
	}
	keys := make([]string, 0, len(list.Contents))
	for _, c := range list.Contents {
		keys = append(keys, c.Key)
	}
	return keys
}

func TestE2EProgramLogoFiles(t *testing.T) {
	reset(t)
	admin := signIn(t, "admin")
	p := createProgram(t, admin, "Logo Program")
	logoPath := "/programs/" + p.ID

	t.Run("upload validation", func(t *testing.T) {
		admin.send(http.MethodPost, logoPath+"/logo-upload", strings.NewReader("<svg/>"), "image/svg+xml", nil).expect(http.StatusUnsupportedMediaType)
		admin.send(http.MethodPost, logoPath+"/logo-upload", bytes.NewReader(nil), "image/png", nil).expect(http.StatusBadRequest)
		huge := append(pngBytes(t), make([]byte, 3<<20)...)
		admin.send(http.MethodPost, logoPath+"/logo-upload", bytes.NewReader(huge), "image/png", nil).expect(http.StatusRequestEntityTooLarge)
		uploadLogo(t, admin, "00000000-0000-4000-8000-000000000000").expect(http.StatusNotFound)
		uploadLogo(t, anonymous(t), p.ID).expect(http.StatusUnauthorized)
	})

	anonymous(t).call(http.MethodGet, logoPath+"/logo-download", nil).expect(http.StatusNotFound)
	uploadLogo(t, admin, p.ID).expect(http.StatusCreated)
	if keys := s3Keys(t, e2eLogosBucket); len(keys) != 1 {
		t.Fatalf("logos bucket: %v", keys)
	}

	t.Run("download and range", func(t *testing.T) {
		res := anonymous(t).call(http.MethodGet, logoPath+"/logo-download", nil).expect(http.StatusOK)
		if !bytes.Equal(res.Body, pngBytes(t)) || res.Header.Get("Content-Type") != "image/png" || res.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("download: %d bytes, headers %v", len(res.Body), res.Header)
		}
		ranged := anonymous(t).send(http.MethodGet, logoPath+"/logo-download", nil, "", map[string]string{"Range": "bytes=0-7"}).expect(http.StatusPartialContent)
		if len(ranged.Body) != 8 || ranged.Header.Get("Content-Range") == "" {
			t.Fatalf("range: %d bytes, headers %v", len(ranged.Body), ranged.Header)
		}
		anonymous(t).send(http.MethodGet, logoPath+"/logo-download", nil, "", map[string]string{"Range": "bytes=999999-"}).expect(http.StatusRequestedRangeNotSatisfiable)
	})

	t.Run("replacing queues the old object for deletion", func(t *testing.T) {
		uploadLogo(t, admin, p.ID).expect(http.StatusCreated)
		if n := dbCount(t, "SELECT count(*) FROM object_deletions WHERE bucket = 'logos' AND state = 'pending' AND next_attempt_at <= NOW() + interval '1 minute'"); n != 1 {
			t.Fatalf("%d deletions due; want the replaced logo", n)
		}
		// The deletion relay polls every objectDeletionRelayInterval.
		eventually(t, objectDeletionRelayInterval+5*time.Second, "the replaced logo to be deleted from the bucket", func() bool {
			return len(s3Keys(t, e2eLogosBucket)) == 1
		})
	})

	t.Run("delete clears the logo", func(t *testing.T) {
		admin.call(http.MethodDelete, logoPath+"/logo", nil).expect(http.StatusNoContent)
		admin.call(http.MethodDelete, logoPath+"/logo", nil).expect(http.StatusNoContent)
		anonymous(t).call(http.MethodGet, logoPath+"/logo-download", nil).expect(http.StatusNotFound)
		admin.call(http.MethodDelete, "/programs/00000000-0000-4000-8000-000000000000/logo", nil).expect(http.StatusNotFound)
		anonymous(t).call(http.MethodDelete, logoPath+"/logo", nil).expect(http.StatusUnauthorized)
	})

	t.Run("an archived program's logo is locked", func(t *testing.T) {
		dbExec(t, "UPDATE programs SET status = 'archived' WHERE id = $1", p.ID)
		uploadLogo(t, admin, p.ID).expect(http.StatusConflict)
		admin.call(http.MethodDelete, logoPath+"/logo", nil).expect(http.StatusConflict)
	})
}

func TestE2ETaskFiles(t *testing.T) {
	reset(t)
	admin := signIn(t, "admin")
	p := publishProgram(t, admin, "Task File Program")
	mentee := signIn(t, "mentee")
	app := apply(t, mentee, p)
	prereq := tasksOf(t, mentee, app.ID)[0]
	filePath := "/tasks/" + prereq.ID

	t.Run("upload validation", func(t *testing.T) {
		uploadTaskFile(t, mentee, prereq.ID, "evil.exe", []byte("MZ\x90\x00binary")).expect(http.StatusUnsupportedMediaType)
		uploadTaskFile(t, mentee, prereq.ID, "empty.pdf", nil).expect(http.StatusBadRequest)
		uploadTaskFile(t, admin, prereq.ID, "intro.pdf", pdfBytes()).expect(http.StatusForbidden)
		uploadTaskFile(t, mentee, "00000000-0000-4000-8000-000000000000", "intro.pdf", pdfBytes()).expect(http.StatusNotFound)
		mentee.send(http.MethodPost, filePath+"/file-upload", strings.NewReader("raw"), "application/pdf", nil).expect(http.StatusBadRequest)

		var body bytes.Buffer
		w := multipart.NewWriter(&body)
		_ = w.WriteField("other", "x")
		_ = w.Close()
		mentee.send(http.MethodPost, filePath+"/file-upload", &body, w.FormDataContentType(), nil).expect(http.StatusBadRequest)

		huge := append(pdfBytes(), bytes.Repeat([]byte("a"), 20<<20)...)
		uploadTaskFile(t, mentee, prereq.ID, "huge.pdf", huge).expect(http.StatusRequestEntityTooLarge)
	})

	mentee.call(http.MethodGet, filePath+"/file-download", nil).expect(http.StatusNotFound)
	var uploaded struct {
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
		Size        int    `json:"size"`
	}
	uploadTaskFile(t, mentee, prereq.ID, "My Intro.pdf", pdfBytes()).expect(http.StatusCreated).decode(&uploaded)
	if uploaded.ContentType != "application/pdf" || uploaded.Size != len(pdfBytes()) || !strings.HasSuffix(uploaded.Filename, ".pdf") {
		t.Fatalf("uploaded: %+v", uploaded)
	}

	t.Run("assignee and reviewers download", func(t *testing.T) {
		for _, a := range []*actor{mentee, admin} {
			res := a.call(http.MethodGet, filePath+"/file-download", nil).expect(http.StatusOK)
			if !bytes.Equal(res.Body, pdfBytes()) || res.Header.Get("Cache-Control") != "private, no-store" ||
				!strings.HasPrefix(res.Header.Get("Content-Disposition"), "attachment") {
				t.Fatalf("download as %s: headers %v", a.LFID, res.Header)
			}
		}
		ranged := admin.send(http.MethodGet, filePath+"/file-download", nil, "", map[string]string{"Range": "bytes=0-3"}).expect(http.StatusPartialContent)
		if string(ranged.Body) != "%PDF" {
			t.Fatalf("range body %q", ranged.Body)
		}
		// A multi-range request is served whole.
		admin.send(http.MethodGet, filePath+"/file-download", nil, "", map[string]string{"Range": "bytes=0-1,4-5"}).expect(http.StatusOK)
		anonymous(t).call(http.MethodGet, filePath+"/file-download", nil).expect(http.StatusUnauthorized)
	})

	t.Run("the assignee removes an unsubmitted file", func(t *testing.T) {
		admin.call(http.MethodDelete, filePath+"/file", nil).expect(http.StatusForbidden)
		mentee.call(http.MethodDelete, filePath+"/file", nil).expect(http.StatusNoContent)
		mentee.call(http.MethodDelete, filePath+"/file", nil).expect(http.StatusNoContent)
		mentee.call(http.MethodGet, filePath+"/file-download", nil).expect(http.StatusNotFound)
		mentee.call(http.MethodDelete, "/tasks/00000000-0000-4000-8000-000000000000/file", nil).expect(http.StatusNotFound)
		anonymous(t).call(http.MethodDelete, filePath+"/file", nil).expect(http.StatusUnauthorized)
	})

	t.Run("deleting the program queues its task files", func(t *testing.T) {
		uploadTaskFile(t, mentee, prereq.ID, "again.pdf", pdfBytes()).expect(http.StatusCreated)
		admin.call(http.MethodDelete, "/programs/"+p.ID, nil).expect(http.StatusNoContent)
		if n := dbCount(t, "SELECT count(*) FROM object_deletions WHERE bucket = 'attachments' AND next_attempt_at <= NOW() + interval '1 minute'"); n == 0 {
			t.Fatalf("task file not queued for deletion")
		}
	})
}
