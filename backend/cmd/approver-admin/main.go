// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Command approver-admin manages the global mentorship approver team from
// inside the cluster. The HTTP routes for the roster stay off the gateway until
// a platform staff relation exists (docs/rewrite/blockers.md §3), so this is
// how approvers are added today: as a one-off Job, or as the chart's post-sync
// hook that reconciles the roster with the LFIDs in the Helm values.
//
// Every write goes through the roster repository, so the roster row and its
// FGA membership marker land in one transaction exactly as the API's would.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/db"
)

const usage = `usage: approver-admin <command> [lfid ...]

  list                 print the current approver team
  add <lfid> ...       add each LFID to the approver team
  remove <lfid> ...    remove each LFID from the approver team
  sync <lfid> ...      make the approver team exactly this set of LFIDs

An LFID must already have a Mentorship user: each approver signs in to Self
Serve once before they can be added.
`

// userLookup is the part of the user repository the command needs.
type userLookup interface {
	GetByLFID(ctx context.Context, lfid string) (userRef, error)
}

type userRef struct{ ID string }

// admin applies roster changes; its dependencies are interfaces so the
// decisions are testable without a database.
type admin struct {
	roster domain.RosterRepository
	users  userLookup
	out    io.Writer
}

// errUnknownLFIDs reports LFIDs with no Mentorship user. Changes for the other
// LFIDs are still applied before it is returned.
type errUnknownLFIDs []string

func (e errUnknownLFIDs) Error() string {
	return fmt.Sprintf("no Mentorship user for %s (each approver must sign in to Self Serve once first)", strings.Join(e, ", "))
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	command, lfids := os.Args[1], os.Args[2:]
	if err := validateArgs(command, lfids); err != nil {
		fmt.Fprintf(os.Stderr, "approver-admin: %v\n\n%s", err, usage)
		os.Exit(2)
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, db.PoolConfig{MaxConns: 2})
	if err != nil {
		fmt.Fprintf(os.Stderr, "approver-admin: database pool: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	a := &admin{roster: db.NewRosterRepository(pool), users: pgUsers{db.NewUserRepository(pool)}, out: os.Stdout}
	if err := a.run(ctx, command, lfids); err != nil {
		fmt.Fprintf(os.Stderr, "approver-admin: %v\n", err)
		os.Exit(1)
	}
}

// pgUsers adapts the user repository to the narrow lookup the command uses.
type pgUsers struct{ repo domain.UserRepository }

func (u pgUsers) GetByLFID(ctx context.Context, lfid string) (userRef, error) {
	user, err := u.repo.GetByLFID(ctx, lfid)
	if err != nil {
		return userRef{}, err
	}
	return userRef{ID: user.ID}, nil
}

// validateArgs rejects malformed invocations before a database connection is
// opened.
func validateArgs(command string, lfids []string) error {
	switch command {
	case "list":
		if len(lfids) > 0 {
			return errors.New("list takes no arguments")
		}
	case "add", "remove", "sync":
		if len(lfids) == 0 {
			return fmt.Errorf("%s needs at least one LFID", command)
		}
		seen := map[string]bool{}
		for _, lfid := range lfids {
			if strings.TrimSpace(lfid) == "" {
				return fmt.Errorf("%s: empty LFID", command)
			}
			if seen[lfid] {
				return fmt.Errorf("%s: %q given more than once", command, lfid)
			}
			seen[lfid] = true
		}
	default:
		return fmt.Errorf("unknown command %q", command)
	}
	return nil
}

func (a *admin) run(ctx context.Context, command string, lfids []string) error {
	switch command {
	case "list":
		return a.list(ctx)
	case "add":
		return a.add(ctx, lfids)
	case "remove":
		return a.remove(ctx, lfids)
	case "sync":
		return a.sync(ctx, lfids)
	}
	return fmt.Errorf("unknown command %q", command)
}

func (a *admin) list(ctx context.Context) error {
	members, err := a.roster.ListApprovers(ctx)
	if err != nil {
		return err
	}
	if len(members) == 0 {
		a.printf("approver team is empty\n")
		return nil
	}
	for _, m := range members {
		a.printf("%s\t%s\t%s\n", m.LFID, m.UserID, m.CreatedOn.UTC().Format("2006-01-02"))
	}
	return nil
}

func (a *admin) add(ctx context.Context, lfids []string) error {
	var unknown errUnknownLFIDs
	for _, lfid := range lfids {
		user, err := a.users.GetByLFID(ctx, lfid)
		if errors.Is(err, domain.ErrUserNotFound) {
			unknown = append(unknown, lfid)
			continue
		}
		if err != nil {
			return fmt.Errorf("look up %q: %w", lfid, err)
		}
		if _, err := a.roster.AddApprover(ctx, user.ID); err != nil {
			return fmt.Errorf("add %q: %w", lfid, err)
		}
		a.printf("added %s\n", lfid)
	}
	if len(unknown) > 0 {
		return unknown
	}
	return nil
}

func (a *admin) remove(ctx context.Context, lfids []string) error {
	current, err := a.rosterByLFID(ctx)
	if err != nil {
		return err
	}
	for _, lfid := range lfids {
		userID, ok := current[lfid]
		if !ok {
			a.printf("%s is not an approver\n", lfid)
			continue
		}
		if err := a.roster.RemoveApprover(ctx, userID); err != nil {
			return fmt.Errorf("remove %q: %w", lfid, err)
		}
		a.printf("removed %s\n", lfid)
	}
	return nil
}

// sync adds the LFIDs missing from the roster and removes members not in the
// list, so rerunning it with the same list changes nothing. LFIDs without a
// user are reported after every other change has been applied.
func (a *admin) sync(ctx context.Context, lfids []string) error {
	current, err := a.rosterByLFID(ctx)
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, lfid := range lfids {
		want[lfid] = true
	}

	var toAdd, toRemove []string
	for _, lfid := range lfids {
		if _, ok := current[lfid]; !ok {
			toAdd = append(toAdd, lfid)
		}
	}
	for lfid := range current {
		if !want[lfid] {
			toRemove = append(toRemove, lfid)
		}
	}
	sort.Strings(toRemove)

	if len(toAdd) == 0 && len(toRemove) == 0 {
		a.printf("approver team already matches (%d members)\n", len(current))
		return nil
	}
	if err := a.remove(ctx, toRemove); err != nil {
		return err
	}
	return a.add(ctx, toAdd)
}

// printf reports progress; a failed write to stdout is not actionable here.
func (a *admin) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(a.out, format, args...)
}

func (a *admin) rosterByLFID(ctx context.Context) (map[string]string, error) {
	members, err := a.roster.ListApprovers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list approvers: %w", err)
	}
	byLFID := make(map[string]string, len(members))
	for _, m := range members {
		byLFID[m.LFID] = m.UserID
	}
	return byLFID, nil
}
