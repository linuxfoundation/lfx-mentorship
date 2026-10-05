// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

type stubFileSvc struct {
	uploadProgramLogo   func(context.Context, string, []byte) (*models.UploadedFile, error)
	downloadProgramLogo func(context.Context, *models.Program, string) (*domain.StoredObject, error)
	uploadTaskFile      func(context.Context, string, string, string, []byte) (*models.UploadedFile, error)
	downloadTaskFile    func(context.Context, string, string, string) (*domain.StoredObject, string, error)
	deleteErr           error
}

func (s *stubFileSvc) UploadProgramLogo(ctx context.Context, id string, data []byte) (*models.UploadedFile, error) {
	return s.uploadProgramLogo(ctx, id, data)
}

func (s *stubFileSvc) DownloadProgramLogo(ctx context.Context, p *models.Program, byteRange string) (*domain.StoredObject, error) {
	return s.downloadProgramLogo(ctx, p, byteRange)
}

func (s *stubFileSvc) UploadProfileLogo(context.Context, string, string, []byte) (*models.UploadedFile, error) {
	return nil, domain.ErrFileNotFound
}

func (s *stubFileSvc) DownloadProfileLogo(context.Context, string, string) (*domain.StoredObject, error) {
	return nil, domain.ErrFileNotFound
}

func (s *stubFileSvc) DeleteProgramLogo(context.Context, string) error { return s.deleteErr }

func (s *stubFileSvc) DeleteProfileLogo(context.Context, string, string) error { return s.deleteErr }

func (s *stubFileSvc) DeleteTaskFile(context.Context, string, string) error { return s.deleteErr }

func (s *stubFileSvc) UploadTaskFile(ctx context.Context, id, actorID, filename string, data []byte) (*models.UploadedFile, error) {
	return s.uploadTaskFile(ctx, id, actorID, filename, data)
}

func (s *stubFileSvc) DownloadTaskFile(ctx context.Context, id, actorID, byteRange string) (*domain.StoredObject, string, error) {
	return s.downloadTaskFile(ctx, id, actorID, byteRange)
}

func withPrincipal(r *http.Request, userID string) *http.Request {
	return r.WithContext(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: userID, Username: userID}))
}

func multipartBody(t *testing.T, field, filename string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("caption", "ignored"); err != nil {
		t.Fatal(err)
	}
	part, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, w.FormDataContentType()
}

func TestFileHandler_UploadProgramLogo(t *testing.T) {
	var gotID string
	var gotData []byte
	h := handler.NewFileHandler(&stubFileSvc{
		uploadProgramLogo: func(_ context.Context, id string, data []byte) (*models.UploadedFile, error) {
			gotID, gotData = id, data
			return &models.UploadedFile{PublicURL: "https://cdn/x", Filename: "logo.png", ContentType: "image/png", Size: int64(len(data))}, nil
		},
	}, &stubProgramSvc{})

	r := httptest.NewRequest(http.MethodPost, "/v1/programs/p1/logo-upload", strings.NewReader("png-bytes"))
	r = withPrincipal(requestWithChiParam(r, "id", "p1"), "u1")
	w := httptest.NewRecorder()
	h.UploadProgramLogo(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d; want 201: %s", w.Code, w.Body)
	}
	if gotID != "p1" || string(gotData) != "png-bytes" {
		t.Fatalf("service got id=%q data=%q", gotID, gotData)
	}
	var body models.UploadedFile
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil || body.PublicURL != "https://cdn/x" {
		t.Fatalf("body = %+v, err = %v", body, err)
	}
}

func TestFileHandler_UploadProgramLogo_TooLarge(t *testing.T) {
	h := handler.NewFileHandler(&stubFileSvc{
		uploadProgramLogo: func(context.Context, string, []byte) (*models.UploadedFile, error) {
			t.Fatal("service must not be called for an oversized body")
			return nil, nil
		},
	}, &stubProgramSvc{})

	r := httptest.NewRequest(http.MethodPost, "/v1/programs/p1/logo-upload", bytes.NewReader(make([]byte, service.MaxLogoBytes+1)))
	r = withPrincipal(requestWithChiParam(r, "id", "p1"), "u1")
	w := httptest.NewRecorder()
	h.UploadProgramLogo(w, r)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d; want 413", w.Code)
	}
}

func TestFileHandler_PrincipalRoutesRequirePrincipal(t *testing.T) {
	h := handler.NewFileHandler(&stubFileSvc{}, &stubProgramSvc{})
	routes := map[string]http.HandlerFunc{
		"program logo upload": h.UploadProgramLogo,
		"program logo delete": h.DeleteProgramLogo,
		"profile logo upload": h.UploadProfileLogo,
		"profile logo delete": h.DeleteProfileLogo,
		"task file upload":    h.UploadTaskFile,
		"task file download":  h.DownloadTaskFile,
		"task file delete":    h.DeleteTaskFile,
	}
	for name, route := range routes {
		r := requestWithChiParam(httptest.NewRequest(http.MethodPost, "/v1/x", strings.NewReader("x")), "id", "x1")
		w := httptest.NewRecorder()
		route(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d; want 401", name, w.Code)
		}
	}
}

func TestFileHandler_DownloadProgramLogo_HiddenProgramIsNotFound(t *testing.T) {
	h := handler.NewFileHandler(&stubFileSvc{
		downloadProgramLogo: func(context.Context, *models.Program, string) (*domain.StoredObject, error) {
			t.Fatal("hidden program logo must not be read")
			return nil, nil
		},
	}, &stubProgramSvc{
		getByID: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Status: models.ProgramStatusHidden}, nil
		},
	})
	r := requestWithChiParam(httptest.NewRequest(http.MethodGet, "/v1/programs/x/logo-download", nil), "id", "0b9a3f4e-6d5c-4b3a-9f8e-7d6c5b4a3f2e")
	w := httptest.NewRecorder()
	h.DownloadProgramLogo(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", w.Code)
	}
}

func TestFileHandler_DownloadProgramLogo_StreamsPublicImage(t *testing.T) {
	h := handler.NewFileHandler(&stubFileSvc{
		downloadProgramLogo: func(context.Context, *models.Program, string) (*domain.StoredObject, error) {
			return &domain.StoredObject{
				Body:          io.NopCloser(strings.NewReader("PNG")),
				ContentType:   "image/png",
				CacheControl:  "public, max-age=86400",
				ContentLength: 3,
			}, nil
		},
	}, &stubProgramSvc{})
	r := requestWithChiParam(httptest.NewRequest(http.MethodGet, "/v1/programs/x/logo-download", nil), "id", "0b9a3f4e-6d5c-4b3a-9f8e-7d6c5b4a3f2e")
	w := httptest.NewRecorder()
	h.DownloadProgramLogo(w, r)
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "public, max-age=86400" || w.Header().Get("Content-Disposition") != "" {
		t.Fatalf("status = %d, headers = %v", w.Code, w.Header())
	}
}

func TestFileHandler_DeleteRoutes(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{nil, http.StatusNoContent},
		{domain.ErrStateLocked, http.StatusConflict},
		{domain.ErrForbidden, http.StatusForbidden},
	} {
		h := handler.NewFileHandler(&stubFileSvc{deleteErr: tc.err}, &stubProgramSvc{})
		for name, route := range map[string]http.HandlerFunc{
			"program logo": h.DeleteProgramLogo,
			"profile logo": h.DeleteProfileLogo,
			"task file":    h.DeleteTaskFile,
		} {
			r := withPrincipal(requestWithChiParam(httptest.NewRequest(http.MethodDelete, "/v1/x", nil), "id", "x1"), "u1")
			w := httptest.NewRecorder()
			route(w, r)
			if w.Code != tc.want {
				t.Errorf("%s with %v: status = %d; want %d", name, tc.err, w.Code, tc.want)
			}
		}
	}
}

func TestFileHandler_UploadTaskFile(t *testing.T) {
	var gotActor, gotName string
	var gotData []byte
	h := handler.NewFileHandler(&stubFileSvc{
		uploadTaskFile: func(_ context.Context, _, actorID, filename string, data []byte) (*models.UploadedFile, error) {
			gotActor, gotName, gotData = actorID, filename, data
			return &models.UploadedFile{Filename: "essay.pdf", ContentType: "application/pdf", Size: int64(len(data))}, nil
		},
	}, &stubProgramSvc{})

	body, contentType := multipartBody(t, "file", "essay.pdf", []byte("%PDF-1.7"))
	r := httptest.NewRequest(http.MethodPost, "/v1/tasks/t1/file-upload", body)
	r.Header.Set("Content-Type", contentType)
	r = withPrincipal(requestWithChiParam(r, "id", "t1"), "mentee")
	w := httptest.NewRecorder()
	h.UploadTaskFile(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d; want 201: %s", w.Code, w.Body)
	}
	if gotActor != "mentee" || gotName != "essay.pdf" || string(gotData) != "%PDF-1.7" {
		t.Fatalf("service got actor=%q name=%q data=%q", gotActor, gotName, gotData)
	}
}

func TestFileHandler_UploadTaskFile_BadBodies(t *testing.T) {
	h := handler.NewFileHandler(&stubFileSvc{
		uploadTaskFile: func(context.Context, string, string, string, []byte) (*models.UploadedFile, error) {
			t.Fatal("service must not be called")
			return nil, nil
		},
	}, &stubProgramSvc{})

	t.Run("not multipart", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/v1/tasks/t1/file-upload", strings.NewReader("%PDF"))
		r.Header.Set("Content-Type", "application/pdf")
		w := httptest.NewRecorder()
		h.UploadTaskFile(w, withPrincipal(requestWithChiParam(r, "id", "t1"), "mentee"))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d; want 400", w.Code)
		}
	})

	t.Run("missing file part", func(t *testing.T) {
		body, contentType := multipartBody(t, "attachment", "essay.pdf", []byte("%PDF"))
		r := httptest.NewRequest(http.MethodPost, "/v1/tasks/t1/file-upload", body)
		r.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		h.UploadTaskFile(w, withPrincipal(requestWithChiParam(r, "id", "t1"), "mentee"))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d; want 400", w.Code)
		}
	})

	t.Run("truncated body", func(t *testing.T) {
		body, contentType := multipartBody(t, "file", "essay.pdf", []byte("%PDF"))
		truncated := bytes.NewReader(body.Bytes()[:body.Len()/2])
		r := httptest.NewRequest(http.MethodPost, "/v1/tasks/t1/file-upload", truncated)
		r.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		h.UploadTaskFile(w, withPrincipal(requestWithChiParam(r, "id", "t1"), "mentee"))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d; want 400", w.Code)
		}
	})

	t.Run("file over the cap", func(t *testing.T) {
		body, contentType := multipartBody(t, "file", "big.pdf", make([]byte, service.MaxTaskFileBytes+1))
		r := httptest.NewRequest(http.MethodPost, "/v1/tasks/t1/file-upload", body)
		r.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		h.UploadTaskFile(w, withPrincipal(requestWithChiParam(r, "id", "t1"), "mentee"))
		if w.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d; want 413", w.Code)
		}
	})
}

func TestFileHandler_DownloadTaskFile(t *testing.T) {
	modified := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	var gotRange string
	h := handler.NewFileHandler(&stubFileSvc{
		downloadTaskFile: func(_ context.Context, _, _, byteRange string) (*domain.StoredObject, string, error) {
			gotRange = byteRange
			return &domain.StoredObject{
				Body:          io.NopCloser(strings.NewReader("PDF")),
				ContentType:   "application/pdf",
				CacheControl:  "private, no-store",
				ContentLength: 3,
				ContentRange:  "bytes 0-2/10",
				ETag:          `"abc"`,
				LastModified:  modified,
			}, "essay.pdf", nil
		},
	}, &stubProgramSvc{})

	r := httptest.NewRequest(http.MethodGet, "/v1/tasks/t1/file-download", nil)
	r.Header.Set("Range", "bytes=0-2")
	w := httptest.NewRecorder()
	h.DownloadTaskFile(w, withPrincipal(requestWithChiParam(r, "id", "t1"), "reviewer"))

	if w.Code != http.StatusPartialContent {
		t.Fatalf("status = %d; want 206", w.Code)
	}
	if gotRange != "bytes=0-2" {
		t.Fatalf("range = %q", gotRange)
	}
	for header, want := range map[string]string{
		"Content-Type":           "application/pdf",
		"Cache-Control":          "private, no-store",
		"Content-Disposition":    "attachment; filename=essay.pdf",
		"Content-Range":          "bytes 0-2/10",
		"Content-Length":         "3",
		"X-Content-Type-Options": "nosniff",
		"Last-Modified":          modified.Format(http.TimeFormat),
	} {
		if got := w.Header().Get(header); got != want {
			t.Errorf("%s = %q; want %q", header, got, want)
		}
	}
	if w.Body.String() != "PDF" {
		t.Fatalf("body = %q", w.Body.String())
	}
}

func TestFileHandler_DownloadTaskFile_MapsErrors(t *testing.T) {
	for err, want := range map[error]int{
		domain.ErrFileNotFound:        http.StatusNotFound,
		domain.ErrForbidden:           http.StatusForbidden,
		domain.ErrRangeNotSatisfiable: http.StatusRequestedRangeNotSatisfiable,
		domain.ErrUpstreamUnavailable: http.StatusServiceUnavailable,
	} {
		h := handler.NewFileHandler(&stubFileSvc{
			downloadTaskFile: func(context.Context, string, string, string) (*domain.StoredObject, string, error) {
				return nil, "", err
			},
		}, &stubProgramSvc{})
		r := httptest.NewRequest(http.MethodGet, "/v1/tasks/t1/file-download", nil)
		w := httptest.NewRecorder()
		h.DownloadTaskFile(w, withPrincipal(requestWithChiParam(r, "id", "t1"), "u1"))
		if w.Code != want {
			t.Errorf("%v: status = %d; want %d", err, w.Code, want)
		}
	}
}
