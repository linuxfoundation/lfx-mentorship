// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Command user-backfill completes users rows created before sign-in learned
// the caller's identity (issue #360): rows with an LFID but no email, name,
// given name, family name or avatar. For each, the LF directory supplies what
// is missing and the row is upserted through the same COALESCE a sign-in uses,
// so every stored value is left alone.
//
// It reports without writing unless --apply is given. Run it in the cluster,
// where FGA_NATS_URL reaches lfx-v2-auth-service, as a one-off Job or from the
// runner pod.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/authservice"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/db"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
	"github.com/nats-io/nats.go"
)

// userStore is the part of the user repository the command uses.
type userStore interface {
	ListIncompleteIdentities(ctx context.Context, limit int) ([]*models.User, error)
	BootstrapByLFID(ctx context.Context, input models.UserCreateInput) (*models.User, error)
}

// identityResolver fills a row's missing fields from the LF directory without writing.
type identityResolver interface {
	ResolveIdentity(ctx context.Context, lfid string, input models.UserUpdateInput) models.UserUpdateInput
}

// backfill fills incomplete rows; its dependencies are interfaces so the
// decisions are testable without a database or NATS.
type backfill struct {
	users    userStore
	identity identityResolver
	apply    bool
	out      io.Writer
}

// summary counts what one run did, or would do in a dry run.
type summary struct {
	Seen, Completed, Partial, Unchanged, Failed int
}

const runTimeout = 30 * time.Minute

func main() {
	apply := flag.Bool("apply", false, "write the filled fields; without it the command only reports")
	limit := flag.Int("limit", 1000, "maximum rows to process in this run")
	flag.Parse()
	if *limit <= 0 {
		fmt.Fprintln(os.Stderr, "user-backfill: --limit must be positive")
		os.Exit(2)
	}
	natsURL := os.Getenv("FGA_NATS_URL")
	if natsURL == "" {
		fmt.Fprintln(os.Stderr, "user-backfill: FGA_NATS_URL is required to reach lfx-v2-auth-service")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	pool, err := db.NewPool(ctx, db.PoolConfig{MaxConns: 2})
	if err != nil {
		fmt.Fprintf(os.Stderr, "user-backfill: database pool: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()
	conn, err := nats.Connect(natsURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "user-backfill: NATS connection: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	users := db.NewUserRepository(pool)
	svc := service.NewUserService(users)
	svc.SetAccountDirectory(authservice.NewClient(conn, 0))
	b := &backfill{users: users, identity: svc, apply: *apply, out: os.Stdout}
	result, err := b.run(ctx, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "user-backfill: %v\n", err)
		os.Exit(1)
	}
	if result.Failed > 0 {
		os.Exit(1)
	}
}

// run processes up to limit incomplete rows. A dry run still asks the
// directory, so the report shows exactly what --apply would write.
func (b *backfill) run(ctx context.Context, limit int) (summary, error) {
	var s summary
	users, err := b.users.ListIncompleteIdentities(ctx, limit)
	if err != nil {
		return s, fmt.Errorf("list incomplete users: %w", err)
	}
	for _, before := range users {
		if before.LFID == nil || *before.LFID == "" {
			continue
		}
		s.Seen++
		lfid := *before.LFID
		resolved := b.identity.ResolveIdentity(ctx, lfid, identityOf(before))
		filled := filledFields(before, resolved)
		if len(filled) == 0 {
			s.Unchanged++
			b.printf("%s: unchanged (directory has nothing more)\n", lfid)
			continue
		}
		if b.apply {
			if _, err := b.users.BootstrapByLFID(ctx, models.UserCreateInput{
				LFID: &lfid, Email: resolved.Email, Name: resolved.Name, GivenName: resolved.GivenName, FamilyName: resolved.FamilyName, AvatarURL: resolved.AvatarURL,
			}); err != nil {
				s.Failed++
				b.printf("%s: FAILED %v\n", lfid, err)
				continue
			}
		}
		if missing := missingFields(resolved); missing == 0 {
			s.Completed++
			b.printf("%s: completed %v\n", lfid, filled)
		} else {
			s.Partial++
			b.printf("%s: filled %v, still missing %d\n", lfid, filled, missing)
		}
	}
	b.printf("seen %d: completed %d, partial %d, unchanged %d, failed %d", s.Seen, s.Completed, s.Partial, s.Unchanged, s.Failed)
	if !b.apply {
		b.printf(" (dry run; nothing written)")
	}
	b.printf("\n")
	return s, nil
}

// identityOf is the row's current identity fields, so the directory is asked
// only for what the row lacks.
func identityOf(u *models.User) models.UserUpdateInput {
	return models.UserUpdateInput{Email: u.Email, Name: u.Name, GivenName: u.GivenName, FamilyName: u.FamilyName, AvatarURL: u.AvatarURL}
}

// filledFields names the identity fields the directory supplied.
func filledFields(before *models.User, after models.UserUpdateInput) []string {
	var filled []string
	for _, f := range []struct {
		name          string
		before, after *string
	}{
		{"email", before.Email, after.Email},
		{"name", before.Name, after.Name},
		{"given_name", before.GivenName, after.GivenName},
		{"family_name", before.FamilyName, after.FamilyName},
		{"avatar_url", before.AvatarURL, after.AvatarURL},
	} {
		if f.before == nil && f.after != nil && *f.after != "" {
			filled = append(filled, f.name)
		}
	}
	return filled
}

// missingFields counts identity fields still empty after resolution.
func missingFields(u models.UserUpdateInput) int {
	n := 0
	for _, v := range []*string{u.Email, u.Name, u.GivenName, u.FamilyName, u.AvatarURL} {
		if v == nil || *v == "" {
			n++
		}
	}
	return n
}

// printf writes one report line; the report is best-effort output.
func (b *backfill) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(b.out, format, args...)
}
