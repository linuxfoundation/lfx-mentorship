// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/db"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Server wraps the Chi router and all service dependencies.
type Server struct {
	router  *chi.Mux
	pool    *pgxpool.Pool
	cfg     *Config
	logger  *slog.Logger
	httpSrv *http.Server
}

// NewServer wires all dependencies and builds the Chi router.
func NewServer(ctx context.Context, cfg *Config, logger *slog.Logger) (*Server, error) {
	// Database pool
	pool, err := db.NewPool(ctx, db.PoolConfig{
		DSN:             cfg.Database.DSN,
		MaxConns:        cfg.Database.MaxConns,
		MinConns:        cfg.Database.MinConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
	})
	if err != nil {
		return nil, fmt.Errorf("database pool: %w", err)
	}

	// Repositories
	userRepo := db.NewUserRepository(pool)
	userProfileRepo := db.NewUserProfileRepository(pool)
	programRepo := db.NewProgramRepository(pool)
	programTermRepo := db.NewProgramTermRepository(pool)
	programMemberRepo := db.NewProgramMemberRepository(pool)
	applicationRepo := db.NewApplicationRepository(pool)
	taskRepo := db.NewTaskRepository(pool)

	// Notifier
	notifier := infrastructure.NewLogNotifier(logger)

	// Services
	userSvc := service.NewUserService(userRepo)
	userProfileSvc := service.NewUserProfileService(userProfileRepo)
	programSvc := service.NewProgramService(programRepo, programTermRepo, applicationRepo)
	programTermSvc := service.NewProgramTermService(programTermRepo, applicationRepo)
	programMemberSvc := service.NewProgramMemberService(programMemberRepo, programRepo, notifier, cfg.Local.InviteSecret)
	applicationSvc := service.NewApplicationService(applicationRepo, taskRepo, programTermRepo, programRepo, notifier)
	taskSvc := service.NewTaskService(taskRepo, applicationRepo, notifier)

	// Handlers
	userH := handler.NewUserHandler(userSvc)
	userProfileH := handler.NewUserProfileHandler(userProfileSvc)
	programH := handler.NewProgramHandler(programSvc)
	programTermH := handler.NewProgramTermHandler(programTermSvc)
	programMemberH := handler.NewProgramMemberHandler(programMemberSvc)
	applicationH := handler.NewApplicationHandler(applicationSvc)
	taskH := handler.NewTaskHandler(taskSvc)
	mentorInviteH := handler.NewMentorInviteHandler(programMemberSvc)

	// JWT authenticator
	jwtAuth, err := auth.NewJWTAuthenticator(ctx, cfg.jwtAuthConfig(), logger)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("JWT authenticator: %w", err)
	}

	// Chi router
	r := chi.NewRouter()
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.Recoverer)
	r.Use(otelhttp.NewMiddleware("mentorship-api"))
	r.Use(chimiddleware.Timeout(time.Duration(float64(cfg.Server.WriteTimeout) * 0.8)))

	// Health probes
	r.Get("/livez", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			handler.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "db unavailable"})
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	r.Route("/v1", func(r chi.Router) {
		// ── Public endpoints ─────────────────────────────────────────────────
		r.Get("/users", userH.List)
		r.Get("/users/{id}", userH.GetByID)

		r.Get("/user-profiles", userProfileH.List)
		r.Get("/user-profiles/{id}", userProfileH.GetByID)
		r.Get("/user-profiles/slug/{slug}", userProfileH.GetBySlug)

		r.Get("/programs", programH.List)
		r.Get("/programs/{id}", programH.GetByID)
		r.Get("/programs/{id}/skills", programH.ListSkills)
		r.Get("/programs/{id}/funding-stats", programH.GetFundingStats)
		r.Get("/programs/{id}/terms", programTermH.ListByProgram)
		r.Get("/programs/{id}/members", programMemberH.List)

		r.Get("/program-terms/{id}", programTermH.GetByID)
		r.Get("/program-terms/{id}/applications", applicationH.ListByProgramTerm)
		r.Get("/program-terms/{id}/tasks", taskH.ListByProgramTerm)

		r.Get("/applications/{id}", applicationH.GetByID)
		r.Get("/applications/{id}/tasks", taskH.ListByApplication)
		r.Get("/tasks/{id}", taskH.GetByID)

		// Mentor invite — token in path is the credential, no JWT required
		r.Post("/mentor-invites/{token}/accept", mentorInviteH.AcceptInvite)
		r.Post("/mentor-invites/{token}/decline", mentorInviteH.DeclineInvite)

		// ── Authenticated endpoints ────────────────────────────────────────
		r.Group(func(r chi.Router) {
			r.Use(jwtAuth.Middleware)

			// Users
			r.Post("/users", userH.Create)
			r.Patch("/users/{id}", userH.Update)
			r.Delete("/users/{id}", userH.Delete)
			r.Get("/users/{userId}/applications", applicationH.ListByUser)

			// User profiles
			r.Post("/user-profiles", userProfileH.Create)
			r.Patch("/user-profiles/{id}", userProfileH.Update)
			r.Delete("/user-profiles/{id}", userProfileH.Delete)

			// Programs
			r.Post("/programs", programH.Create)
			r.Patch("/programs/{id}", programH.Update)
			r.Delete("/programs/{id}", programH.Delete)
			r.Post("/programs/{id}/skills", programH.AddSkill)
			r.Delete("/programs/{id}/skills/{skillId}", programH.DeleteSkill)

			// Program members
			r.Post("/programs/{id}/members", programMemberH.Create)
			r.Patch("/programs/{id}/members/{memberId}", programMemberH.Update)
			r.Delete("/programs/{id}/members/{memberId}", programMemberH.Delete)

			// Program terms
			r.Post("/programs/{id}/terms", programTermH.Create)
			r.Patch("/program-terms/{id}", programTermH.Update)
			r.Delete("/program-terms/{id}", programTermH.Delete)

			// Applications
			r.Post("/program-terms/{id}/applications", applicationH.Create)
			r.Patch("/applications/{id}", applicationH.Update)
			r.Delete("/applications/{id}", applicationH.Delete)
			r.Post("/program-terms/{id}/applications/bulk-decline", applicationH.BulkDeclineByTerm)
			r.Get("/program-terms/{id}/applications/export", applicationH.ExportByTerm)
			r.Get("/program-terms/{id}/past-mentees", applicationH.PastMenteesByTerm)

			// Tasks
			r.Post("/applications/{id}/tasks", taskH.Create)
			r.Patch("/tasks/{id}", taskH.Update)
			r.Delete("/tasks/{id}", taskH.Delete)
		})
	})

	httpSrv := &http.Server{
		Addr:           fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:        r,
		ReadTimeout:    cfg.Server.ReadTimeout,
		WriteTimeout:   cfg.Server.WriteTimeout,
		IdleTimeout:    cfg.Server.IdleTimeout,
		MaxHeaderBytes: 1 << 20,
	}

	return &Server{
		router:  r,
		pool:    pool,
		cfg:     cfg,
		logger:  logger,
		httpSrv: httpSrv,
	}, nil
}

// Start begins listening for HTTP requests.
func (s *Server) Start() error {
	s.logger.Info("starting mentorship API", "addr", s.httpSrv.Addr)
	return s.httpSrv.ListenAndServe()
}

// Shutdown gracefully stops the server and closes the database pool.
func (s *Server) Shutdown(ctx context.Context) error {
	err := s.httpSrv.Shutdown(ctx)
	s.pool.Close()
	return err
}
