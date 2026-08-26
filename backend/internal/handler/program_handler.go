// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

type programService interface {
	GetByID(ctx context.Context, id string) (*models.Program, error)
	GetBySlug(ctx context.Context, slug string) (*models.Program, error)
	List(ctx context.Context, filter models.ProgramFilter) ([]*models.Program, *models.PaginationMeta, error)
	ListCatalog(ctx context.Context, filter models.ProgramFilter) ([]*models.ProgramCatalogItem, *models.PaginationMeta, error)
	GetCatalog(ctx context.Context, id string) (*models.ProgramCatalogItem, error)
	ListCatalogMentees(ctx context.Context, programID string) ([]*models.ProgramCatalogMentee, error)
	Create(ctx context.Context, input models.ProgramCreateInput) (*models.Program, error)
	Update(ctx context.Context, id string, input models.ProgramUpdateInput) (*models.Program, error)
	Delete(ctx context.Context, id string) error
	ListSkills(ctx context.Context, programID string) ([]*models.ProgramSkill, error)
	AddSkill(ctx context.Context, programID string, input models.ProgramSkillCreateInput) (*models.ProgramSkill, error)
	DeleteSkill(ctx context.Context, skillID string) error
	GetFundingStats(ctx context.Context, programID string) (*models.ProgramFundingStats, error)
}

// ProgramHandler holds Chi handlers for the programs resource.
type ProgramHandler struct {
	svc programService
}

// NewProgramHandler creates a ProgramHandler.
func NewProgramHandler(svc programService) *ProgramHandler {
	return &ProgramHandler{svc: svc}
}

// List handles GET /v1/programs.
func (h *ProgramHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	programs, meta, err := h.svc.List(r.Context(), models.ProgramFilter{
		Limit:  limit,
		Offset: offset,
		Status: r.URL.Query().Get("status"),
		Search: r.URL.Query().Get("search"),
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": programs, "meta": meta})
}

func catalogSortParam(r *http.Request) string {
	if v := r.URL.Query().Get("sort_by"); v != "" {
		return v
	}
	return r.URL.Query().Get("sortBy")
}

// ListCatalog handles GET /v1/programs/catalog.
func (h *ProgramHandler) ListCatalog(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	items, meta, err := h.svc.ListCatalog(r.Context(), models.ProgramFilter{
		Limit:           limit,
		Offset:          offset,
		Search:          r.URL.Query().Get("search"),
		Skill:           r.URL.Query().Get("skill"),
		DiscoveryStatus: r.URL.Query().Get("status"),
		SortBy:          catalogSortParam(r),
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": items, "meta": meta})
}

// GetCatalog handles GET /v1/programs/{id}/catalog.
func (h *ProgramHandler) GetCatalog(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	item, err := h.svc.GetCatalog(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	if item.Status == models.ProgramStatusHidden {
		principal := auth.PrincipalFromContext(r.Context())
		isOwner := principal != nil && item.LFID != nil && *item.LFID == principal.Username
		if !isOwner {
			Error(w, domain.ErrProgramNotFound)
			return
		}
	}
	JSON(w, http.StatusOK, item)
}

// ListCatalogMentees handles GET /v1/programs/{id}/mentees.
func (h *ProgramHandler) ListCatalogMentees(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	program, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		program, err = h.svc.GetBySlug(r.Context(), id)
		if err != nil {
			Error(w, err)
			return
		}
	}
	if program.Status == models.ProgramStatusHidden {
		principal := auth.PrincipalFromContext(r.Context())
		isOwner := principal != nil && program.LFID != nil && *program.LFID == principal.Username
		if !isOwner {
			Error(w, domain.ErrProgramNotFound)
			return
		}
	}
	mentees, err := h.svc.ListCatalogMentees(r.Context(), program.ID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": mentees})
}

// GetByID handles GET /v1/programs/{id}.
func (h *ProgramHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	program, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		// Try by slug if not found by UUID
		program, err = h.svc.GetBySlug(r.Context(), id)
		if err != nil {
			Error(w, err)
			return
		}
	}
	// FR-009: hidden programs return 404 to everyone except the owner (matched by LFID).
	if program.Status == models.ProgramStatusHidden {
		principal := auth.PrincipalFromContext(r.Context())
		isOwner := principal != nil && program.LFID != nil && *program.LFID == principal.Username
		if !isOwner {
			Error(w, domain.ErrProgramNotFound)
			return
		}
	}
	JSON(w, http.StatusOK, program)
}

// Create handles POST /v1/programs — requires JWT.
func (h *ProgramHandler) Create(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	var input models.ProgramCreateInput
	if !decodeBody(w, r, &input) {
		return
	}

	program, err := h.svc.Create(r.Context(), input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, program)
}

// Update handles PATCH /v1/programs/{id} — requires JWT.
func (h *ProgramHandler) Update(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	var input models.ProgramUpdateInput
	if !decodeBody(w, r, &input) {
		return
	}

	program, err := h.svc.Update(r.Context(), id, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, program)
}

// Delete handles DELETE /v1/programs/{id} — requires JWT.
func (h *ProgramHandler) Delete(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.svc.Delete(r.Context(), id); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListSkills handles GET /v1/programs/{id}/skills.
func (h *ProgramHandler) ListSkills(w http.ResponseWriter, r *http.Request) {
	programID := chi.URLParam(r, "id")
	skills, err := h.svc.ListSkills(r.Context(), programID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": skills})
}

// AddSkill handles POST /v1/programs/{id}/skills — requires JWT.
func (h *ProgramHandler) AddSkill(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	programID := chi.URLParam(r, "id")
	var input models.ProgramSkillCreateInput
	if !decodeBody(w, r, &input) {
		return
	}

	skill, err := h.svc.AddSkill(r.Context(), programID, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, skill)
}

// DeleteSkill handles DELETE /v1/programs/{id}/skills/{skillId} — requires JWT.
func (h *ProgramHandler) DeleteSkill(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	skillID := chi.URLParam(r, "skillId")
	if err := h.svc.DeleteSkill(r.Context(), skillID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetFundingStats handles GET /v1/programs/{id}/funding-stats.
func (h *ProgramHandler) GetFundingStats(w http.ResponseWriter, r *http.Request) {
	programID := chi.URLParam(r, "id")
	stats, err := h.svc.GetFundingStats(r.Context(), programID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, stats)
}
