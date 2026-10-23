// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Command user-backfill completes users rows created before sign-in learned
// the caller's identity (issue #360): rows with an LFID but no email, name,
// given name, family name or avatar. For each, the LF directory supplies what
// is missing and only the columns still NULL at write time are set, so a value
// stored since the row was read is kept and no other row is touched.
//
// It reports without writing unless --apply is given. Rows are walked oldest
// first in pages; when --limit stops a run early, the command prints the
// --cursor that continues from where it left off, so rows the directory could
// not complete do not block the ones after them. Run it in the cluster, where
// FGA_NATS_URL reaches lfx-v2-auth-service, as a one-off Job or from the
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
	ListIncompleteIdentities(ctx context.Context, after *db.UserCursor, limit int) ([]*models.User, error)
	FillMissingIdentity(ctx context.Context, id string, input models.UserUpdateInput) (*models.User, error)
}

// identityResolver fills a row's missing fields from the LF directory without
// writing; a lookup failure comes back as an error.
type identityResolver interface {
	ResolveIdentity(ctx context.Context, lfid string, input models.UserUpdateInput) (models.UserUpdateInput, error)
}

// backfill fills incomplete rows; its dependencies are interfaces so the
// decisions are testable without a database or NATS.
type backfill struct {
	users    userStore
	identity identityResolver
	apply    bool
	pageSize int
	out      io.Writer
}

// summary counts what one run did, or would do in a dry run. Next is set when
// --limit stopped the run with rows still to visit.
type summary struct {
	Seen, Completed, Partial, Unchanged, Failed int
	Next                                        *db.UserCursor
}

const (
	runTimeout      = 30 * time.Minute
	defaultPageSize = 100
)

func main() {
	apply := flag.Bool("apply", false, "write the filled fields; without it the command only reports")
	limit := flag.Int("limit", 1000, "maximum rows to visit in this run; the run prints a --cursor to continue")
	cursor := flag.String("cursor", "", "continue after this position, as printed by an earlier run")
	flag.Parse()
	if *limit <= 0 {
		fmt.Fprintln(os.Stderr, "user-backfill: --limit must be positive")
		os.Exit(2)
	}
	var after *db.UserCursor
	if *cursor != "" {
		parsed, err := db.ParseUserCursor(*cursor)
		if err != nil {
			fmt.Fprintf(os.Stderr, "user-backfill: --cursor: %v\n", err)
			os.Exit(2)
		}
		after = &parsed
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
	b := &backfill{users: users, identity: svc, apply: *apply, pageSize: defaultPageSize, out: os.Stdout}
	result, err := b.run(ctx, after, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "user-backfill: %v\n", err)
		os.Exit(1)
	}
	if result.Failed > 0 {
		os.Exit(1)
	}
}

// run visits up to limit incomplete rows after the cursor, a page at a time.
// A dry run still asks the directory, so the report shows exactly what --apply
// would write. It stops with an error when the context ends or a page cannot
// be read; a row that fails is counted and the run goes on.
func (b *backfill) run(ctx context.Context, after *db.UserCursor, limit int) (summary, error) {
	var s summary
	pageSize := b.pageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	for s.Seen < limit {
		if err := ctx.Err(); err != nil {
			b.finish(s)
			return s, fmt.Errorf("stopped after %d rows: %w", s.Seen, err)
		}
		want := min(pageSize, limit-s.Seen)
		page, err := b.users.ListIncompleteIdentities(ctx, after, want)
		if err != nil {
			b.finish(s)
			return s, fmt.Errorf("list incomplete users: %w", err)
		}
		if len(page) == 0 {
			break
		}
		for _, row := range page {
			if err := ctx.Err(); err != nil {
				b.finish(s)
				return s, fmt.Errorf("stopped after %d rows: %w", s.Seen, err)
			}
			b.visit(ctx, row, &s)
		}
		last := page[len(page)-1]
		after = &db.UserCursor{CreatedOn: last.CreatedOn, ID: last.ID}
		if len(page) < want {
			break // a short page: nothing after it
		}
	}
	if s.Seen >= limit {
		// There may be more; let the operator continue from here.
		more, err := b.users.ListIncompleteIdentities(ctx, after, 1)
		if err != nil {
			b.finish(s)
			return s, fmt.Errorf("check for remaining users: %w", err)
		}
		if len(more) > 0 {
			s.Next = after
		}
	}
	b.finish(s)
	return s, nil
}

// visit resolves and, with --apply, writes one row, counting the outcome.
func (b *backfill) visit(ctx context.Context, row *models.User, s *summary) {
	if row.LFID == nil || *row.LFID == "" {
		return
	}
	s.Seen++
	lfid := *row.LFID
	resolved, err := b.identity.ResolveIdentity(ctx, lfid, identityOf(row))
	if err != nil {
		s.Failed++
		b.printf("%s: FAILED directory lookup: %v\n", lfid, err)
		return
	}
	filled := filledFields(row, resolved)
	if len(filled) == 0 {
		s.Unchanged++
		b.printf("%s: unchanged (directory has nothing more)\n", lfid)
		return
	}
	if b.apply {
		// Only the directory's additions are sent; the UPDATE keeps any column
		// filled since the row was read.
		if _, err := b.users.FillMissingIdentity(ctx, row.ID, additions(row, resolved)); err != nil {
			s.Failed++
			b.printf("%s: FAILED %v\n", lfid, err)
			return
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

// finish prints the summary and, when the run stopped early, how to continue.
func (b *backfill) finish(s summary) {
	b.printf("seen %d: completed %d, partial %d, unchanged %d, failed %d", s.Seen, s.Completed, s.Partial, s.Unchanged, s.Failed)
	if !b.apply {
		b.printf(" (dry run; nothing written)")
	}
	b.printf("\n")
	if s.Next != nil {
		b.printf("more rows remain; continue with --cursor %s\n", s.Next)
	}
}

// identityOf is the row's current identity fields, so the directory is asked
// only for what the row lacks.
func identityOf(u *models.User) models.UserUpdateInput {
	return models.UserUpdateInput{Email: u.Email, Name: u.Name, GivenName: u.GivenName, FamilyName: u.FamilyName, AvatarURL: u.AvatarURL}
}

// additions is what the directory supplied: the resolved fields the row lacked.
func additions(row *models.User, resolved models.UserUpdateInput) models.UserUpdateInput {
	added := func(before, after *string) *string {
		if before == nil && after != nil && *after != "" {
			return after
		}
		return nil
	}
	return models.UserUpdateInput{
		Email:      added(row.Email, resolved.Email),
		Name:       added(row.Name, resolved.Name),
		GivenName:  added(row.GivenName, resolved.GivenName),
		FamilyName: added(row.FamilyName, resolved.FamilyName),
		AvatarURL:  added(row.AvatarURL, resolved.AvatarURL),
	}
}

// filledFields names the identity fields the directory supplied.
func filledFields(row *models.User, resolved models.UserUpdateInput) []string {
	a := additions(row, resolved)
	var filled []string
	for _, f := range []struct {
		name  string
		value *string
	}{{"email", a.Email}, {"name", a.Name}, {"given_name", a.GivenName}, {"family_name", a.FamilyName}, {"avatar_url", a.AvatarURL}} {
		if f.value != nil {
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

// printf reports progress; a failed write to stdout is not actionable here.
func (b *backfill) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(b.out, format, args...)
}
