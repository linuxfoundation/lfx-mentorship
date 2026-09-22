// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"expvar"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/clients"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/db"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/fga"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Server wraps the Chi router and all service dependencies.
type Server struct {
	router      *chi.Mux
	pool        *pgxpool.Pool
	cfg         *Config
	logger      *slog.Logger
	httpSrv     *http.Server
	natsConn    *nats.Conn
	relayCancel context.CancelFunc
}

// NewServer wires all dependencies and builds the Chi router.
func NewServer(ctx context.Context, cfg *Config, logger *slog.Logger) (*Server, error) {
	// Database pool
	pool, err := db.NewPool(ctx, db.PoolConfig{
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
	menteeRepo := db.NewMenteeRepository(pool)
	mentorRepo := db.NewMentorRepository(pool)
	platformSummaryRepo := db.NewPlatformSummaryRepository(pool)
	rosterRepo := db.NewRosterRepository(pool)

	// Notifier
	notifier := infrastructure.NewLogNotifier(logger)

	// Services
	userSvc := service.NewUserService(userRepo)
	userProfileSvc := service.NewUserProfileService(userProfileRepo)
	programSvc := service.NewProgramService(programRepo, programTermRepo, applicationRepo, programMemberRepo)
	if cfg.Crowdfunding.IsConfigured() {
		programSvc.SetCrowdfundingClient(clients.NewCrowdfundingClient(clients.CrowdfundingConfig{
			BaseURL:      cfg.Crowdfunding.BaseURL,
			TokenURL:     cfg.Crowdfunding.TokenURL,
			ClientID:     cfg.Crowdfunding.ClientID,
			ClientSecret: cfg.Crowdfunding.ClientSecret,
			Audience:     cfg.Crowdfunding.Audience,
			Scope:        cfg.Crowdfunding.Scope,
			Timeout:      cfg.Crowdfunding.Timeout,
		}))
	}
	programTermSvc := service.NewProgramTermService(programTermRepo, applicationRepo)
	programMemberSvc := service.NewProgramMemberService(programMemberRepo, programRepo, notifier, cfg.Local.InviteSecret)
	applicationSvc := service.NewApplicationService(applicationRepo, taskRepo, programTermRepo, programRepo, programMemberRepo, notifier, rosterRepo)
	taskSvc := service.NewTaskService(taskRepo, applicationRepo, programTermRepo, programMemberRepo, notifier)
	menteeSvc := service.NewMenteeService(menteeRepo)
	mentorSvc := service.NewMentorService(mentorRepo)
	platformSummarySvc := service.NewPlatformSummaryService(platformSummaryRepo)
	rosterSvc := service.NewRosterService(rosterRepo)
	fundingStatsSvc := service.NewFundingStatsService(programRepo)

	var natsConn *nats.Conn
	var relayCancel context.CancelFunc
	if cfg.FGA.NATSURL != "" {
		natsConn, err = nats.Connect(cfg.FGA.NATSURL)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("FGA NATS connection: %w", err)
		}
		js, jsErr := jetstream.New(natsConn)
		if jsErr != nil {
			natsConn.Close()
			pool.Close()
			return nil, fmt.Errorf("FGA JetStream client: %w", jsErr)
		}
		outbox := db.NewFGAOutboxRepository(pool)
		approverRepo := db.NewApproverRepository(pool)
		rosterRepo := db.NewRosterRepository(pool)
		builder := fga.NewDatabaseBuilder(programRepo, programMemberRepo, userRepo, programTermRepo, applicationRepo, taskRepo, approverRepo, rosterRepo)
		publisher := fga.NewJetStreamPublisher(js)
		relay := fga.NewRelay(outbox, builder, publisher, cfg.FGA.RelayBatch, cfg.FGA.RelayRetryDelay)
		relay.SetLogger(logger)
		relay.SetMaxAttempts(cfg.FGA.RelayMaxAttempts)
		var relayCtx context.Context
		relayCtx, relayCancel = context.WithCancel(ctx)
		go relay.Run(relayCtx, cfg.FGA.RelayInterval)
	}

	// Handlers
	userH := handler.NewUserHandler(userSvc)
	userProfileH := handler.NewUserProfileHandler(userProfileSvc)
	programH := handler.NewProgramHandler(programSvc)
	programTermH := handler.NewProgramTermHandler(programTermSvc)
	programMemberH := handler.NewProgramMemberHandler(programMemberSvc, programSvc)
	applicationH := handler.NewApplicationHandler(applicationSvc, programTermSvc)
	taskH := handler.NewTaskHandler(taskSvc, programTermSvc)
	mentorInviteH := handler.NewMentorInviteHandler(programMemberSvc)
	menteeH := handler.NewMenteeHandler(menteeSvc)
	mentorH := handler.NewMentorHandler(mentorSvc)
	platformSummaryH := handler.NewPlatformSummaryHandler(platformSummarySvc)
	rosterH := handler.NewRosterHandler(rosterSvc)
	fundingStatsH := handler.NewFundingStatsHandler(fundingStatsSvc)

	// JWT authenticator
	jwtAuth, err := auth.NewJWTAuthenticator(ctx, cfg.jwtAuthConfig(), logger)
	if err != nil {
		if relayCancel != nil {
			relayCancel()
		}
		if natsConn != nil {
			natsConn.Close()
		}
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

	var resolveInterimPrincipal func(http.Handler) http.Handler
	routes := func(r chi.Router) {
		optionalJWT := jwtAuth.OptionalMiddleware
		// ── Public endpoints ─────────────────────────────────────────────────
		r.Get("/programs", programH.List)
		r.Get("/programs/catalog", programH.ListCatalog)
		r.With(optionalJWT).Get("/programs/resolve/{id}", programH.ResolveID)
		r.With(optionalJWT).Get("/programs/{id}", programH.GetByID)
		r.Get("/programs/{id}/catalog", programH.GetCatalog)
		r.With(optionalJWT).Get("/programs/{id}/mentees", programH.ListCatalogMentees)
		r.Get("/programs/{id}/skills", programH.ListSkills)

		r.Get("/mentees", menteeH.List)
		r.Get("/mentees/summary", menteeH.Summary)
		r.Get("/mentees/{id}", menteeH.GetByID)
		r.Get("/mentors", mentorH.List)
		r.Get("/mentors/summary", mentorH.Summary)
		r.Get("/mentors/{id}", mentorH.GetByID)
		r.Get("/summary", platformSummaryH.Get)
		r.Get("/funding-stats/total", fundingStatsH.GetTotal)
		r.Get("/programs/{id}/funding-stats", programH.GetFundingStats)
		r.With(optionalJWT).Get("/programs/{id}/transactions", programH.GetCategorizedTransactions)
		r.With(optionalJWT).Get("/programs/{id}/sponsors", programH.GetProgramSponsors)
		r.Get("/programs/{id}/terms", programTermH.ListByProgram)
		r.Get("/programs/{id}/members", programMemberH.List)

		r.Get("/programs/{programID}/terms/{termID}", programTermH.GetByID)

		// ── Authenticated endpoints ────────────────────────────────────────
		r.Group(func(r chi.Router) {
			r.Use(jwtAuth.Middleware)
			r.Use(resolveInterimPrincipal)

			// Mentor invite — both the invite token and signed principal are required.
			r.Post("/mentor-invites/{token}/accept", mentorInviteH.AcceptInvite)
			r.Post("/mentor-invites/{token}/decline", mentorInviteH.DeclineInvite)

			// Users
			r.Get("/me", userH.GetMe)
			r.Put("/me", userH.BootstrapMe)
			r.Patch("/me", userH.UpdateMe)
			r.Delete("/me", userH.DeleteMe)
			r.Get("/me/applications", applicationH.ListByMe)

			// User profiles
			r.Get("/me/profiles", userProfileH.ListMe)
			r.Get("/me/profiles/{profileType}", userProfileH.GetMeByType)
			r.Post("/me/profiles", userProfileH.Create)
			r.Patch("/me/profiles/{profileType}", userProfileH.UpdateMeByType)
			r.Delete("/me/profiles/{profileType}", userProfileH.DeleteMeByType)
			r.Patch("/me/profiles/by-id/{id}", userProfileH.UpdateMeByID)
			r.Delete("/me/profiles/by-id/{id}", userProfileH.DeleteMeByID)

			// Programs
			r.Post("/programs", programH.Create)
			r.Patch("/programs/{id}", programH.Update)
			r.Post("/programs/{id}/submit", programH.Submit)
			r.Post("/programs/{id}/decision", programH.Decision)
			r.Delete("/programs/{id}", programH.Delete)
			r.Post("/programs/{id}/skills", programH.AddSkill)
			r.Delete("/programs/{id}/skills/{skillId}", programH.DeleteSkill)

			// Program members
			r.Post("/programs/{id}/members", programMemberH.Create)
			r.Patch("/programs/{id}/members/{memberId}", programMemberH.Update)
			r.Delete("/programs/{id}/members/{memberId}", programMemberH.Delete)

			// Program terms
			r.Post("/programs/{id}/terms", programTermH.Create)
			r.Patch("/programs/{programID}/terms/{termID}", programTermH.Update)
			r.Delete("/programs/{programID}/terms/{termID}", programTermH.Delete)

			// Applications
			r.Get("/programs/{programID}/terms/{id}/applications", applicationH.ListByProgramTerm)
			r.Get("/applications/{id}", applicationH.GetByID)
			r.Post("/programs/{programID}/terms/{id}/applications", applicationH.Create)
			r.Patch("/applications/{id}", applicationH.Update)
			r.Patch("/applications/{id}/status", applicationH.UpdateStatus)
			r.Post("/applications/{id}/withdraw", applicationH.Withdraw)
			r.Post("/applications/{id}/withdraw-for-mentee", applicationH.WithdrawForMentee)
			r.Post("/applications/{id}/reapply", applicationH.Reapply)
			r.Delete("/applications/{id}", applicationH.Delete)
			r.Post("/programs/{programID}/terms/{id}/applications/bulk-decline", applicationH.BulkDeclineByTerm)
			r.Get("/programs/{programID}/terms/{id}/applications/export", applicationH.ExportByTerm)
			r.Get("/programs/{programID}/terms/{id}/past-mentees", applicationH.PastMenteesByTerm)
			r.Put("/applications/{id}/evaluation", applicationH.UpdateEvaluation)
			r.Get("/applications/{id}/note", applicationH.GetNote)
			r.Put("/applications/{id}/note", applicationH.UpdateNote)

			// Tasks
			r.Get("/applications/{id}/tasks", taskH.ListByApplication)
			r.Get("/programs/{programID}/terms/{id}/tasks", taskH.ListByProgramTerm)
			r.Get("/tasks/{id}", taskH.GetByID)
			r.Post("/applications/{id}/tasks", taskH.Create)
			r.Patch("/tasks/{id}", taskH.Update)
			r.Patch("/tasks/{id}/submission", taskH.UpdateSubmission)
			r.Patch("/tasks/{id}/review", taskH.UpdateReview)
			r.Delete("/tasks/{id}", taskH.Delete)

			// Platform-authorized authorization roster management.
			r.Get("/admin/approver-team/members", rosterH.ListApprovers)
			r.Post("/admin/approver-team/members", rosterH.AddApprover)
			r.Delete("/admin/approver-team/members/{userID}", rosterH.RemoveApprover)
			r.Get("/projects/{projectUID}/mentorship-program-admins", rosterH.ListProjectAdmins)
			r.Post("/projects/{projectUID}/mentorship-program-admins", rosterH.AddProjectAdmin)
			r.Delete("/projects/{projectUID}/mentorship-program-admins/{userID}", rosterH.RemoveProjectAdmin)
		})
	}
	resolveGatewayPrincipal := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			principal := auth.PrincipalFromContext(req.Context())
			if !auth.IsGatewayPrincipal(req.Context()) || principal == nil || principal.UserID == "_anonymous" {
				next.ServeHTTP(w, req)
				return
			}
			if req.Method == http.MethodPut && strings.HasSuffix(req.URL.Path, "/me") {
				next.ServeHTTP(w, req)
				return
			}
			// M2M principals are authorization identities, not local human users.
			if strings.HasSuffix(principal.Username, "@clients") {
				next.ServeHTTP(w, req)
				return
			}
			lfid := principal.Username
			if lfid == "" {
				lfid = principal.UserID
			}
			user, err := userRepo.GetByLFID(req.Context(), lfid)
			if err != nil {
				handler.JSON(w, http.StatusUnauthorized, map[string]any{"error": "local user is not provisioned"})
				return
			}
			if user.LFID == nil || *user.LFID == "" {
				handler.JSON(w, http.StatusUnauthorized, map[string]any{"error": "local user has no LFID"})
				return
			}
			resolved := *principal
			resolved.UserID = user.ID
			resolved.Username = *user.LFID
			next.ServeHTTP(w, req.WithContext(auth.ContextWithPrincipal(req.Context(), &resolved)))
		})
	}
	resolveInterimPrincipal = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			principal := auth.PrincipalFromContext(req.Context())
			if principal == nil || auth.IsGatewayPrincipal(req.Context()) || (req.Method == http.MethodPut && strings.HasSuffix(req.URL.Path, "/me")) {
				next.ServeHTTP(w, req)
				return
			}
			if principal.Username == "" {
				handler.JSON(w, http.StatusUnauthorized, map[string]any{"error": "local user identity is missing"})
				return
			}
			user, err := userRepo.GetByLFID(req.Context(), principal.Username)
			if err != nil || user.LFID == nil || *user.LFID == "" {
				handler.JSON(w, http.StatusUnauthorized, map[string]any{"error": "local user is not provisioned"})
				return
			}
			resolved := *principal
			resolved.UserID = user.ID
			next.ServeHTTP(w, req.WithContext(auth.ContextWithPrincipal(req.Context(), &resolved)))
		})
	}

	// Guide 07 §1.1: serve both the interim and gateway path prefixes on the
	// same handlers until the interim host is retired. No StripPrefix/URLRewrite.
	// Heimdall-only operations are intentionally unavailable on the interim
	// Auth0 host because this service does not duplicate their FGA checks.
	r.Route("/v1", func(r chi.Router) {
		r.Use(denyInterimGatewayOperations)
		routes(r)
	})
	if cfg.JWT.HeimdallIssuer != "" {
		r.Route("/mentorship/v1", func(r chi.Router) {
			r.Use(jwtAuth.GatewayMiddleware)
			r.Use(resolveGatewayPrincipal)
			r.Get("/internal/metrics", func(w http.ResponseWriter, req *http.Request) {
				if !auth.HasScope(req.Context(), auth.ScopeReadMetrics()) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				expvar.Handler().ServeHTTP(w, req)
			})
			routes(r)
		})
	}

	httpSrv := &http.Server{
		Addr:           fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:        r,
		ReadTimeout:    cfg.Server.ReadTimeout,
		WriteTimeout:   cfg.Server.WriteTimeout,
		IdleTimeout:    cfg.Server.IdleTimeout,
		MaxHeaderBytes: 1 << 20,
	}

	return &Server{
		router:      r,
		pool:        pool,
		cfg:         cfg,
		logger:      logger,
		httpSrv:     httpSrv,
		natsConn:    natsConn,
		relayCancel: relayCancel,
	}, nil
}

func denyInterimGatewayOperations(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		protected := (r.Method != http.MethodGet && strings.HasPrefix(path, "/v1/programs/")) ||
			(r.Method != http.MethodGet && (strings.Contains(path, "/applications/") || strings.Contains(path, "/tasks") || strings.Contains(path, "/mentor-invites/") || strings.Contains(path, "/program-terms/"))) ||
			strings.Contains(path, "/decision") || strings.HasSuffix(path, "/submit") ||
			(r.Method == http.MethodDelete && strings.Contains(path, "/applications/")) ||
			strings.Contains(path, "/applications/") && (strings.HasSuffix(path, "/status") || strings.HasSuffix(path, "/note") || strings.HasSuffix(path, "/evaluation") || strings.HasSuffix(path, "/withdraw-for-mentee") || strings.HasSuffix(path, "/reapply")) ||
			strings.Contains(path, "/tasks") || strings.HasSuffix(path, "/applications") && strings.Contains(path, "/terms/")
		if protected {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Start begins listening for HTTP requests.
func (s *Server) Start() error {
	s.logger.Info("starting mentorship API", "addr", s.httpSrv.Addr)
	return s.httpSrv.ListenAndServe()
}

// Shutdown gracefully stops the server and closes the database pool.
func (s *Server) Shutdown(ctx context.Context) error {
	err := s.httpSrv.Shutdown(ctx)
	if s.relayCancel != nil {
		s.relayCancel()
	}
	if s.natsConn != nil {
		s.natsConn.Close()
	}
	s.pool.Close()
	return err
}
