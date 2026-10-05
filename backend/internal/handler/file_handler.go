// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

// multipartOverheadBytes is the allowance for boundaries and part headers on top of the file cap.
const multipartOverheadBytes = 1 << 20

// taskFileFormField is the multipart part that carries a task submission.
const taskFileFormField = "file"

type fileService interface {
	UploadProgramLogo(ctx context.Context, programID string, data []byte) (*models.UploadedFile, error)
	DeleteProgramLogo(ctx context.Context, programID string) error
	DownloadProgramLogo(ctx context.Context, program *models.Program, byteRange string) (*domain.StoredObject, error)
	UploadProfileLogo(ctx context.Context, profileID, actorID string, data []byte) (*models.UploadedFile, error)
	DeleteProfileLogo(ctx context.Context, profileID, actorID string) error
	DownloadProfileLogo(ctx context.Context, profileID, byteRange string) (*domain.StoredObject, error)
	UploadTaskFile(ctx context.Context, taskID, actorID, filename string, data []byte) (*models.UploadedFile, error)
	DeleteTaskFile(ctx context.Context, taskID, actorID string) error
	DownloadTaskFile(ctx context.Context, taskID, actorID, byteRange string) (*domain.StoredObject, string, error)
}

// FileHandler holds Chi handlers for file upload and download routes.
type FileHandler struct {
	svc      fileService
	programs programLookup
}

// NewFileHandler creates a FileHandler.
func NewFileHandler(svc fileService, programs programLookup) *FileHandler {
	return &FileHandler{svc: svc, programs: programs}
}

// UploadProgramLogo handles POST /v1/programs/{id}/logo-upload. The body is the raw image.
func (h *FileHandler) UploadProgramLogo(w http.ResponseWriter, r *http.Request) {
	if auth.PrincipalFromContext(r.Context()) == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	data, ok := readUploadBody(w, r, service.MaxLogoBytes)
	if !ok {
		return
	}
	uploaded, err := h.svc.UploadProgramLogo(r.Context(), chi.URLParam(r, "id"), data)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, uploaded)
}

// UploadProfileLogo handles POST /v1/me/profiles/by-id/{id}/logo-upload. The body is the raw image.
func (h *FileHandler) UploadProfileLogo(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	data, ok := readUploadBody(w, r, service.MaxLogoBytes)
	if !ok {
		return
	}
	uploaded, err := h.svc.UploadProfileLogo(r.Context(), chi.URLParam(r, "id"), principal.UserID, data)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, uploaded)
}

// DownloadProgramLogo handles GET /v1/programs/{id}/logo-download, a fallback to the CDN URL.
func (h *FileHandler) DownloadProgramLogo(w http.ResponseWriter, r *http.Request) {
	program, ok := resolveVisibleProgram(w, r, h.programs)
	if !ok {
		return
	}
	obj, err := h.svc.DownloadProgramLogo(r.Context(), program, byteRange(r))
	if err != nil {
		Error(w, err)
		return
	}
	writeObject(w, obj, "")
}

// DownloadProfileLogo handles GET /v1/user-profiles/{id}/logo-download, a fallback to the CDN URL.
func (h *FileHandler) DownloadProfileLogo(w http.ResponseWriter, r *http.Request) {
	obj, err := h.svc.DownloadProfileLogo(r.Context(), chi.URLParam(r, "id"), byteRange(r))
	if err != nil {
		Error(w, err)
		return
	}
	writeObject(w, obj, "")
}

// UploadTaskFile handles POST /v1/tasks/{id}/file-upload. The body is multipart/form-data
// so the "file" part can carry the filename the download route serves it under.
func (h *FileHandler) UploadTaskFile(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	filename, data, ok := readMultipartFile(w, r, service.MaxTaskFileBytes)
	if !ok {
		return
	}
	uploaded, err := h.svc.UploadTaskFile(r.Context(), chi.URLParam(r, "id"), principal.UserID, filename, data)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, uploaded)
}

// DownloadTaskFile handles GET /v1/tasks/{id}/file-download.
func (h *FileHandler) DownloadTaskFile(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	obj, filename, err := h.svc.DownloadTaskFile(r.Context(), chi.URLParam(r, "id"), principal.UserID, byteRange(r))
	if err != nil {
		Error(w, err)
		return
	}
	writeObject(w, obj, mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
}

// DeleteProgramLogo handles DELETE /v1/programs/{id}/logo.
func (h *FileHandler) DeleteProgramLogo(w http.ResponseWriter, r *http.Request) {
	if auth.PrincipalFromContext(r.Context()) == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	if err := h.svc.DeleteProgramLogo(r.Context(), chi.URLParam(r, "id")); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteProfileLogo handles DELETE /v1/me/profiles/by-id/{id}/logo.
func (h *FileHandler) DeleteProfileLogo(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	if err := h.svc.DeleteProfileLogo(r.Context(), chi.URLParam(r, "id"), principal.UserID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteTaskFile handles DELETE /v1/tasks/{id}/file.
func (h *FileHandler) DeleteTaskFile(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	if err := h.svc.DeleteTaskFile(r.Context(), chi.URLParam(r, "id"), principal.UserID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// readUploadBody reads a raw-body upload of at most limit bytes.
func readUploadBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, bool) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		Error(w, uploadReadError(err, limit))
		return nil, false
	}
	return data, true
}

// readMultipartFile streams the multipart body to the file part without spilling to disk.
func readMultipartFile(w http.ResponseWriter, r *http.Request, limit int64) (string, []byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, limit+multipartOverheadBytes)
	reader, err := r.MultipartReader()
	if err != nil {
		Error(w, fmt.Errorf("%w: body must be multipart/form-data", domain.ErrInvalidInput))
		return "", nil, false
	}
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			Error(w, fmt.Errorf("%w: a %q file part is required", domain.ErrInvalidInput, taskFileFormField))
			return "", nil, false
		}
		if err != nil {
			Error(w, uploadReadError(err, limit))
			return "", nil, false
		}
		if part.FormName() != taskFileFormField {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(part, limit+1))
		if err != nil {
			Error(w, uploadReadError(err, limit))
			return "", nil, false
		}
		if int64(len(data)) > limit {
			Error(w, uploadReadError(&http.MaxBytesError{Limit: limit}, limit))
			return "", nil, false
		}
		return part.FileName(), data, true
	}
}

func uploadReadError(err error, limit int64) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return fmt.Errorf("%w: file exceeds %d MB", domain.ErrPayloadTooLarge, limit>>20)
	}
	return fmt.Errorf("%w: could not read upload body", domain.ErrInvalidInput)
}

// byteRange passes a single bytes Range header through to the store, for PDF viewers.
func byteRange(r *http.Request) string {
	if v := r.Header.Get("Range"); strings.HasPrefix(v, "bytes=") {
		return v
	}
	return ""
}

// writeObject streams a stored object; a non-empty contentDisposition is set as-is.
func writeObject(w http.ResponseWriter, obj *domain.StoredObject, contentDisposition string) {
	defer func() { _ = obj.Body.Close() }()
	header := w.Header()
	header.Set("Content-Type", obj.ContentType)
	header.Set("Cache-Control", obj.CacheControl)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Accept-Ranges", "bytes")
	header.Set("Content-Length", strconv.FormatInt(obj.ContentLength, 10))
	if contentDisposition != "" {
		header.Set("Content-Disposition", contentDisposition)
	}
	if obj.ETag != "" {
		header.Set("ETag", obj.ETag)
	}
	if !obj.LastModified.IsZero() {
		header.Set("Last-Modified", obj.LastModified.UTC().Format(http.TimeFormat))
	}
	status := http.StatusOK
	if obj.ContentRange != "" {
		header.Set("Content-Range", obj.ContentRange)
		status = http.StatusPartialContent
	}
	w.WriteHeader(status)
	if _, err := io.Copy(w, obj.Body); err != nil {
		slog.Warn("file download interrupted", "error", err)
	}
}
