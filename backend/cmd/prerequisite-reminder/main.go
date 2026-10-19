// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// prerequisite-reminder emails pending mentees who still have prerequisite tasks to submit, on
// their reminder days. It runs once a day as a CronJob.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/mail"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/db"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/email"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
	"github.com/nats-io/nats.go"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("prerequisite-reminder failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	start := time.Now()

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	pool, err := db.NewPool(ctx, db.PoolConfig{
		MaxConns:        5,
		MinConns:        1,
		ConnMaxLifetime: 5 * time.Minute,
	})
	if err != nil {
		return fmt.Errorf("database pool: %w", err)
	}
	defer pool.Close()

	natsConn, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		return fmt.Errorf("NATS connection: %w", err)
	}
	defer natsConn.Close()

	applicationRepo := db.NewApplicationRepository(pool)
	notifier := email.NewNotifier(email.NewClient(natsConn, email.Config{}), email.Repositories{
		Users:        db.NewUserRepository(pool),
		Profiles:     db.NewUserProfileRepository(pool),
		Programs:     db.NewProgramRepository(pool),
		Terms:        db.NewProgramTermRepository(pool),
		Members:      db.NewProgramMemberRepository(pool),
		Applications: applicationRepo,
		Tasks:        db.NewTaskRepository(pool),
	}, email.NotifierConfig{
		SelfServeURL:      cfg.SelfServeURL,
		AllowedRecipients: cfg.AllowedRecipients,
	}, logger)
	if len(cfg.AllowedRecipients) > 0 {
		logger.Warn("email restricted to EMAIL_ALLOWED_RECIPIENTS; all other recipients are suppressed", "allowed", len(cfg.AllowedRecipients))
	}

	logger.Info("prerequisite-reminder starting")
	reminded, err := service.NewPrerequisiteReminderService(applicationRepo, notifier).SendReminders(ctx, start)
	if err != nil {
		return err
	}
	// Reminders are queued from here on, and there is no sent marker: failing the run now would
	// retry it and email every mentee again, so a send that does not finish is logged instead.
	if err := notifier.Wait(ctx); err != nil {
		logger.Error("prerequisite-reminder stopped before every reminder was sent", "error", err)
	}

	logger.Info("prerequisite-reminder complete",
		"duration", time.Since(start).String(),
		"reminded", reminded,
	)
	return nil
}

type config struct {
	NATSURL           string
	SelfServeURL      string
	AllowedRecipients []string
}

// loadConfig reads the settings the reminder email needs; the API's other email settings are unused here.
func loadConfig() (*config, error) {
	cfg := &config{
		NATSURL:      os.Getenv("FGA_NATS_URL"),
		SelfServeURL: strings.TrimRight(os.Getenv("SELF_SERVE_URL"), "/"),
	}
	if cfg.NATSURL == "" {
		return nil, fmt.Errorf("FGA_NATS_URL is required to send email")
	}
	// The reminder's button links into Self Serve.
	if u, err := url.Parse(cfg.SelfServeURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, fmt.Errorf("SELF_SERVE_URL must be an absolute http(s) URL with no query or fragment")
	}
	for r := range strings.SplitSeq(os.Getenv("EMAIL_ALLOWED_RECIPIENTS"), ",") {
		if r = strings.TrimSpace(r); r == "" {
			continue
		}
		if addr, err := mail.ParseAddress(r); err != nil || addr.Address != r {
			return nil, fmt.Errorf("EMAIL_ALLOWED_RECIPIENTS must be a comma-separated list of bare email addresses")
		}
		cfg.AllowedRecipients = append(cfg.AllowedRecipients, r)
	}
	// A set value with no addresses would mean "send to everyone"; fail closed instead.
	if os.Getenv("EMAIL_ALLOWED_RECIPIENTS") != "" && len(cfg.AllowedRecipients) == 0 {
		return nil, fmt.Errorf("EMAIL_ALLOWED_RECIPIENTS is set but lists no addresses")
	}
	return cfg, nil
}
