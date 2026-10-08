// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/authservice"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/clients"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/db"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/email"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/fga"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/indexer"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/objectstore"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/projects"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// objectDeletionRelayInterval paces the object deletion relay; deletes are not latency-sensitive.
const objectDeletionRelayInterval = 10 * time.Second

// objectDeletionRelayBatch is independent of the FGA and index relays: deletes are cheap and idempotent.
const objectDeletionRelayBatch = 50

// fileTransferTimeout matches the gateway's upload timeout (chart value uploads.timeout).
const fileTransferTimeout = 120 * time.Second

// fileTransfer replaces the API-wide request timeout on file routes and lifts the
// server's read and write deadlines, so a slow client can finish a 20 MB transfer.
func fileTransfer(next http.Handler) http.Handler {
	timed := chimiddleware.Timeout(fileTransferTimeout)(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadline := time.Now().Add(fileTransferTimeout)
		rc := http.NewResponseController(w)
		if err := errors.Join(rc.SetReadDeadline(deadline), rc.SetWriteDeadline(deadline)); err != nil {
			slog.WarnContext(r.Context(), "could not extend file transfer deadline", "error", err)
		}
		timed.ServeHTTP(w, r)
	})
}

// Server wraps the Chi router and all service dependencies.
type Server struct {
	router      *chi.Mux
	pool        *pgxpool.Pool
	cfg         *Config
	logger      *slog.Logger
	httpSrv     *http.Server
	natsConn    *nats.Conn
	relayCancel context.CancelFunc
	// emailNotifier is nil unless email notifications are enabled.
	emailNotifier *email.Notifier
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

	// Object storage is checked before serving traffic; a configured bucket that is unreachable fails startup.
	logoStore, err := newObjectStore(ctx, cfg.Storage.Region, cfg.Storage.Logos, logger)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("logos object store: %w", err)
	}
	attachmentStore, err := newObjectStore(ctx, cfg.Storage.Region, cfg.Storage.Attachments, logger)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("attachments object store: %w", err)
	}
	var logoBucket *service.LogoBucket
	if logoStore != nil {
		logoBucket = &service.LogoBucket{Store: logoStore, CDNURLPrefix: cfg.Storage.Logos.CDNURLPrefix}
	}
	var attachments domain.ObjectStore
	if attachmentStore != nil {
		attachments = attachmentStore
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

	var natsConn *nats.Conn
	if cfg.FGA.NATSURL != "" {
		natsConn, err = nats.Connect(cfg.FGA.NATSURL)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("FGA NATS connection: %w", err)
		}
	}

	// Notifier
	var notifier domain.Notifier = infrastructure.NewLogNotifier(logger)
	var emailNotifier *email.Notifier
	var accounts domain.AccountDirectory
	if natsConn != nil {
		accounts = authservice.NewClient(natsConn, 0)
		emailNotifier = email.NewNotifier(email.NewClient(natsConn, email.Config{}), email.Repositories{
			Users:        userRepo,
			Profiles:     userProfileRepo,
			Programs:     programRepo,
			Terms:        programTermRepo,
			Members:      programMemberRepo,
			Applications: applicationRepo,
		}, email.NotifierConfig{
			PublicSiteURL: cfg.Email.PublicSiteURL,
			SelfServeURL:  cfg.Email.SelfServeURL,
			HRInbox:       cfg.Email.HRInbox,

			AllowedRecipients: cfg.Email.AllowedRecipients,
		}, logger)
		notifier = emailNotifier
		if len(cfg.Email.AllowedRecipients) > 0 {
			logger.Warn("email restricted to EMAIL_ALLOWED_RECIPIENTS; all other recipients are suppressed", "allowed", len(cfg.Email.AllowedRecipients))
		}
	}

	// Services
	userSvc := service.NewUserService(userRepo)
	userProfileSvc := service.NewUserProfileService(userProfileRepo)
	programSvc := service.NewProgramService(programRepo, programTermRepo, applicationRepo, programMemberRepo)
	if cfg.Crowdfunding.IsConfigured() {
		programSvc.SetCrowdfundingClient(clients.NewCrowdfundingClient(clients.CrowdfundingConfig{
			BaseURL: cfg.Crowdfunding.BaseURL,
			Timeout: cfg.Crowdfunding.Timeout,
		}))
	}
	if natsConn != nil {
		programSvc.SetProjectLookup(projects.NewClient(natsConn))
	}
	programTermSvc := service.NewProgramTermService(programTermRepo, applicationRepo)
	programMemberSvc := service.NewProgramMemberService(programMemberRepo, programRepo, userRepo, accounts, notifier, cfg.Local.InviteSecret)
	applicationSvc := service.NewApplicationService(applicationRepo, taskRepo, programTermRepo, programRepo, programMemberRepo, notifier)
	taskSvc := service.NewTaskService(taskRepo, applicationRepo, programTermRepo, programMemberRepo, notifier)
	menteeSvc := service.NewMenteeService(menteeRepo)
	mentorSvc := service.NewMentorService(mentorRepo)
	platformSummarySvc := service.NewPlatformSummaryService(platformSummaryRepo)
	rosterSvc := service.NewRosterService(rosterRepo)
	fundingStatsSvc := service.NewFundingStatsService(programRepo)
	objectDeletions := db.NewObjectDeletionRepository(pool)
	fileSvc := service.NewFileService(db.NewFileRepository(pool), objectDeletions, programRepo, taskSvc, logoBucket, attachments)

	relayCtx, relayCancel := context.WithCancel(ctx)
	if logoStore != nil || attachmentStore != nil {
		relayBuckets := map[domain.ObjectBucket]objectstore.RelayBucket{}
		if logoStore != nil {
			relayBuckets[domain.ObjectBucketLogos] = objectstore.RelayBucket{Store: logoStore, CDNURLPrefix: cfg.Storage.Logos.CDNURLPrefix}
		}
		if attachmentStore != nil {
			relayBuckets[domain.ObjectBucketAttachments] = objectstore.RelayBucket{Store: attachmentStore}
		}
		go objectstore.NewRelay(objectDeletions, relayBuckets, objectDeletionRelayBatch, logger).Run(relayCtx, objectDeletionRelayInterval)
	}
	if natsConn != nil {
		js, jsErr := jetstream.New(natsConn)
		if jsErr != nil {
			relayCancel()
			natsConn.Close()
			pool.Close()
			return nil, fmt.Errorf("FGA JetStream client: %w", jsErr)
		}
		outbox := db.NewFGAOutboxRepository(pool)
		outbox.SetMaxAttempts(cfg.FGA.RelayMaxAttempts)
		approverRepo := db.NewApproverRepository(pool)
		builder := fga.NewDatabaseBuilder(programRepo, programMemberRepo, userRepo, programTermRepo, applicationRepo, taskRepo, approverRepo)
		publisher := fga.NewJetStreamPublisher(js)
		relay := fga.NewRelay(outbox, builder, publisher, cfg.FGA.RelayBatch, cfg.FGA.RelayRetryDelay)
		relay.SetLogger(logger)
		relay.SetMaxAttempts(cfg.FGA.RelayMaxAttempts)
		go relay.Run(relayCtx, cfg.FGA.RelayInterval)
		indexOutbox := db.NewIndexOutboxRepository(pool)
		indexOutbox.SetMaxAttempts(cfg.Indexer.MaxAttempts)
		indexOutbox.SetRetryDelay(cfg.Indexer.RetryDelay)
		indexRelay := indexer.NewRelay(indexOutbox, js, cfg.FGA.RelayBatch, cfg.Indexer.ServiceToken)
		indexRelay.SetLogger(logger)
		go indexRelay.Run(relayCtx, cfg.FGA.RelayInterval)
	}

	// Handlers
	userH := handler.NewUserHandler(userSvc)
	userProfileH := handler.NewUserProfileHandler(userProfileSvc)
	programH := handler.NewProgramHandler(programSvc)
	programTermH := handler.NewProgramTermHandler(programTermSvc, programSvc)
	programMemberH := handler.NewProgramMemberHandler(programMemberSvc, programSvc)
	applicationH := handler.NewApplicationHandler(applicationSvc, programTermSvc)
	taskH := handler.NewTaskHandler(taskSvc, programTermSvc)
	mentorInviteH := handler.NewMentorInviteHandler(programMemberSvc)
	menteeH := handler.NewMenteeHandler(menteeSvc)
	mentorH := handler.NewMentorHandler(mentorSvc)
	platformSummaryH := handler.NewPlatformSummaryHandler(platformSummarySvc)
	rosterH := handler.NewRosterHandler(rosterSvc)
	fundingStatsH := handler.NewFundingStatsHandler(fundingStatsSvc)
	fileH := handler.NewFileHandler(fileSvc, programSvc)

	// JWT authenticator
	jwtAuth, err := auth.NewJWTAuthenticator(ctx, cfg.jwtAuthConfig(), logger)
	if err != nil {
		relayCancel()
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
	// Applied per route rather than globally so file routes can use fileTransfer instead.
	requestTimeout := chimiddleware.Timeout(time.Duration(float64(cfg.Server.WriteTimeout) * 0.8))
	probes := r.With(requestTimeout)

	// Health probes
	probes.Get("/livez", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	probes.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	probes.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		// Object storage is checked at startup only: an S3 outage fails the file routes, not the whole API.
		if err := pool.Ping(r.Context()); err != nil {
			handler.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "db unavailable"})
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	probes.Handle("/internal/metrics", expvar.Handler())

	var requireGatewayPrincipal func(http.Handler) http.Handler
	routes := func(r chi.Router) {
		transfer := r.With(fileTransfer)
		authTransfer := transfer.With(requireGatewayPrincipal)
		r = r.With(requestTimeout)

		// ── Public endpoints ─────────────────────────────────────────────────
		// The fully public lists and aggregates stay service-owned and are
		// publicly cacheable (docs/rewrite/05-heimdall-gateway.md).
		public := r.With(handler.PublicCache)
		r.With(requireGatewayPrincipal).Get("/programs/name-availability", programH.NameAvailable)
		public.Get("/programs", programH.List)
		public.Get("/programs/catalog", programH.ListCatalog)
		r.Get("/programs/resolve/{id}", programH.ResolveID)
		r.Get("/programs/{id}", programH.GetByID)
		transfer.Get("/programs/{id}/logo-download", fileH.DownloadProgramLogo)
		r.Get("/programs/{id}/header", programH.GetHeaderProjection)
		r.Get("/programs/{id}/management-summary", programH.GetManagementSummary)
		r.Get("/programs/{id}/catalog", programH.GetCatalog)
		r.Get("/programs/{id}/mentees", programH.ListCatalogMentees)
		r.Get("/programs/{id}/skills", programH.ListSkills)

		public.Get("/mentees", menteeH.List)
		public.Get("/mentees/summary", menteeH.Summary)
		r.Get("/mentees/{id}", menteeH.GetByID)
		public.Get("/mentors", mentorH.List)
		public.Get("/mentors/summary", mentorH.Summary)
		r.Get("/mentors/{id}", mentorH.GetByID)
		public.Get("/summary", platformSummaryH.Get)
		public.Get("/funding-stats/total", fundingStatsH.GetTotal)
		r.Get("/programs/{id}/funding-stats", programH.GetFundingStats)
		r.Get("/programs/{id}/transactions", programH.GetCategorizedTransactions)
		r.Get("/programs/{id}/sponsors", programH.GetProgramSponsors)
		r.Get("/programs/{id}/terms", programTermH.ListByProgram)
		r.Get("/programs/{id}/term-management", programTermH.ListManagementByProgram)
		r.Get("/programs/{id}/members", programMemberH.List)
		r.Get("/programs/{id}/member-management", programMemberH.ListMentorManagement)

		r.Get("/programs/{programID}/terms/{termID}", programTermH.GetByID)

		// ── Authenticated endpoints ────────────────────────────────────────
		r.Group(func(r chi.Router) {
			r.Use(requireGatewayPrincipal)

			r.Get("/programs/{id}/enroll-template", programH.GetEnrollmentTemplate)

			// Mentor invite — both the invite token and signed principal are required.
			r.Post("/mentor-invites/{token}/accept", mentorInviteH.AcceptInvite)
			r.Post("/mentor-invites/{token}/decline", mentorInviteH.DeclineInvite)

			// Users
			r.Get("/me", userH.GetMe)
			r.Put("/me", userH.BootstrapMe)
			r.Patch("/me", userH.UpdateMe)
			r.Delete("/me", userH.DeleteMe)
			r.Get("/me/applications", applicationH.ListByMe)
			r.Get("/me/programs", programH.ListMine)
			r.Get("/me/mentor-programs", mentorH.ListMine)
			r.Get("/me/program-memberships", programMemberH.ListMine)
			r.Post("/me/program-memberships", programMemberH.RequestMine)
			r.Post("/me/program-memberships/{id}/withdraw", programMemberH.WithdrawMine)

			// User profiles
			r.Get("/me/profiles", userProfileH.ListMe)
			r.Get("/me/profiles/{profileType}", userProfileH.GetMeByType)
			r.Post("/me/profiles", userProfileH.Create)
			r.Put("/me/profiles/{profileType}", userProfileH.PutMeByType)
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
			authTransfer.Post("/programs/{id}/logo-upload", fileH.UploadProgramLogo)
			r.Delete("/programs/{id}/logo", fileH.DeleteProgramLogo)
			r.Post("/programs/{id}/skills", programH.AddSkill)
			r.Delete("/programs/{id}/skills/{skillId}", programH.DeleteSkill)

			// Program members
			r.Get("/programs/{id}/mentor-candidates", programMemberH.SearchCandidates)
			r.Post("/programs/{id}/members", programMemberH.Create)
			r.Patch("/programs/{id}/members/{memberId}", programMemberH.Update)
			r.Delete("/programs/{id}/members/{memberId}", programMemberH.Delete)
			r.Post("/programs/{id}/members/{memberId}/resend-invite", programMemberH.ResendInvite)

			// Program terms
			r.Post("/programs/{id}/terms", programTermH.Create)
			r.Patch("/programs/{programID}/terms/{termID}", programTermH.Update)
			r.Post("/programs/{programID}/terms/{termID}/close", programTermH.Close)
			r.Post("/programs/{programID}/terms/{termID}/reopen", programTermH.Reopen)
			r.Delete("/programs/{programID}/terms/{termID}", programTermH.Delete)

			// Applications
			r.Get("/programs/{programID}/terms/{id}/applications", applicationH.ListByProgramTerm)
			r.Get("/programs/{id}/applications", applicationH.ListByProgram)
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
			authTransfer.Post("/tasks/{id}/file-upload", fileH.UploadTaskFile)
			authTransfer.Get("/tasks/{id}/file-download", fileH.DownloadTaskFile)
			r.Delete("/tasks/{id}/file", fileH.DeleteTaskFile)

			// These cluster-local platform-management routes use backend scope checks;
			// they are intentionally not exposed through the Heimdall RuleSet yet.
			r.Get("/admin/approver-team/members", rosterH.ListApprovers)
			r.Post("/admin/approver-team/members", rosterH.AddApprover)
			r.Delete("/admin/approver-team/members/{userID}", rosterH.RemoveApprover)
		})
	}
	requireGatewayPrincipal = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			principal := auth.PrincipalFromContext(req.Context())
			if principal == nil || principal.UserID == "_anonymous" {
				handler.JSON(w, http.StatusUnauthorized, map[string]any{"error": "authenticated gateway principal is required"})
				return
			}
			if req.Method == http.MethodPut && req.URL.Path == "/mentorship/v1/me" {
				next.ServeHTTP(w, req)
				return
			}
			// M2M principals are authorization identities, not local human users.
			if principal.IsM2M() {
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
	r.Route("/mentorship/v1", func(r chi.Router) {
		r.Use(jwtAuth.GatewayMiddleware)
		r.Use(handler.IndexMetadata)
		routes(r)
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
		router:      r,
		pool:        pool,
		cfg:         cfg,
		logger:      logger,
		httpSrv:     httpSrv,
		natsConn:    natsConn,
		relayCancel: relayCancel,

		emailNotifier: emailNotifier,
	}, nil
}

// Start begins listening for HTTP requests.
func (s *Server) Start() error {
	s.logger.Info("starting mentorship API", "addr", s.httpSrv.Addr)
	return s.httpSrv.ListenAndServe()
}

// newObjectStore connects to one bucket, returning nil when the bucket is not configured.
func newObjectStore(ctx context.Context, region string, cfg BucketConfig, logger *slog.Logger) (*objectstore.Store, error) {
	if cfg.Bucket == "" {
		return nil, nil
	}
	store, err := objectstore.New(ctx, objectstore.Config{
		Bucket:              cfg.Bucket,
		Region:              region,
		EndpointURL:         cfg.EndpointURL,
		CreateMissingBucket: cfg.CreateMissingBucket,
	})
	if err != nil {
		return nil, err
	}
	if err := store.EnsureBucket(ctx, logger); err != nil {
		return nil, err
	}
	return store, nil
}

// Shutdown gracefully stops the server and closes the database pool.
func (s *Server) Shutdown(ctx context.Context) error {
	err := s.httpSrv.Shutdown(ctx)
	if s.relayCancel != nil {
		s.relayCancel()
	}
	if s.emailNotifier != nil {
		if waitErr := s.emailNotifier.Wait(ctx); waitErr != nil {
			s.logger.Warn("in-flight email notifications abandoned at shutdown", "error", waitErr)
		}
	}
	if s.natsConn != nil {
		s.natsConn.Close()
	}
	s.pool.Close()
	return err
}
