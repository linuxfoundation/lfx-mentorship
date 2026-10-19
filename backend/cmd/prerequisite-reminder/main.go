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

const (
	// sendAttempts and sendRetryDelay retry a run that fails before queuing any reminder, such as
	// one hit by a brief database outage. The Job itself never retries: there is no sent marker, so
	// rerunning after a crash part-way through sending would email those mentees again.
	sendAttempts   = 3
	sendRetryDelay = 30 * time.Second
	// shutdownGrace is how long queued reminders get to finish once the job is told to stop; it
	// stays under the pod's default 30-second termination grace period.
	shutdownGrace = 20 * time.Second
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
	reminderSvc := service.NewPrerequisiteReminderService(applicationRepo, notifier)
	reminded, err := sendWithRetry(ctx, logger, sendRetryDelay, func(ctx context.Context) (int, error) {
		return reminderSvc.SendReminders(ctx, start)
	})
	if err != nil {
		return err
	}

	err = notifier.Wait(ctx)
	if ctx.Err() != nil {
		// Stopped while sending. The sends are detached from ctx and run on, so give them until
		// shortly before the pod is killed rather than closing NATS under them.
		graceCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		err = notifier.Wait(graceCtx)
	}
	if err != nil {
		return fmt.Errorf("stopped before every reminder was sent: %w", err)
	}

	logger.Info("prerequisite-reminder complete",
		"duration", time.Since(start).String(),
		"reminded", reminded,
	)
	return nil
}

// sendWithRetry calls send up to sendAttempts times, delay apart, until it succeeds. send must queue
// nothing when it fails, so a retry cannot remind anyone twice.
func sendWithRetry(ctx context.Context, logger *slog.Logger, delay time.Duration, send func(context.Context) (int, error)) (int, error) {
	for attempt := 1; ; attempt++ {
		reminded, err := send(ctx)
		if err == nil || attempt == sendAttempts {
			return reminded, err
		}
		logger.Warn("prerequisite-reminder failed before sending; retrying", "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return 0, err
		case <-time.After(delay):
		}
	}
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
