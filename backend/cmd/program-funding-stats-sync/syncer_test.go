// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/clients"
)

type fakeFundingRepo struct {
	programIDs []string
	upserts    []models.ProgramFundingStatsUpsert
	upsertCall int
}

func (f *fakeFundingRepo) ListFundingSyncProgramIDs(_ context.Context) ([]string, error) {
	return f.programIDs, nil
}

func (f *fakeFundingRepo) BulkUpsertFundingStats(_ context.Context, rows []models.ProgramFundingStatsUpsert) (int, error) {
	f.upsertCall++
	f.upserts = append([]models.ProgramFundingStatsUpsert{}, rows...)
	return len(rows), nil
}

type fakeLedger struct {
	pages []*clients.LedgerTransactionsPage
}

func (f *fakeLedger) GetTransactionsPage(_ context.Context, page int, _ int) (*clients.LedgerTransactionsPage, error) {
	if page-1 >= len(f.pages) {
		return &clients.LedgerTransactionsPage{HasNext: false, Transactions: []clients.LedgerTransaction{}}, nil
	}
	return f.pages[page-1], nil
}

func TestSyncerAggregatesMentorshipCredits(t *testing.T) {
	t.Parallel()

	repo := &fakeFundingRepo{programIDs: []string{"p1", "p2"}}
	ledger := &fakeLedger{pages: []*clients.LedgerTransactionsPage{
		{
			HasNext: true,
			Transactions: []clients.LedgerTransaction{
				{ProjectID: "p1", TxnCategory: "mentorship", Amount: 100},
				{ProjectID: "p1", TxnCategory: "mentorship", Amount: -25},
				{ProjectID: "p2", TxnCategory: "mentee", Amount: 500},
				{ProjectID: "outside", TxnCategory: "mentorship", Amount: 1000},
				{ProjectID: "p2", TxnCategory: "mentorship", Amount: 250},
			},
		},
		{
			HasNext: false,
			Transactions: []clients.LedgerTransaction{
				{ProjectID: "p2", TxnCategory: "mentorship", Amount: 50},
			},
		},
	}}

	s := newSyncer(repo, ledger, slog.New(slog.NewTextHandler(io.Discard, nil)), 100)
	result, err := s.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if got, want := result.programs, 2; got != want {
		t.Fatalf("programs = %d, want %d", got, want)
	}
	if got, want := result.matchedPrograms, 2; got != want {
		t.Fatalf("matchedPrograms = %d, want %d", got, want)
	}
	if got, want := result.skippedPrograms, 0; got != want {
		t.Fatalf("skippedPrograms = %d, want %d", got, want)
	}
	if got, want := result.processedTxns, 3; got != want {
		t.Fatalf("processedTxns = %d, want %d", got, want)
	}
	if got, want := result.pagesFetched, 2; got != want {
		t.Fatalf("pagesFetched = %d, want %d", got, want)
	}
	if got, want := result.plannedUpserts, 2; got != want {
		t.Fatalf("plannedUpserts = %d, want %d", got, want)
	}
	if got, want := result.upserted, 2; got != want {
		t.Fatalf("upserted = %d, want %d", got, want)
	}

	if got, want := len(repo.upserts), 2; got != want {
		t.Fatalf("upsert count = %d, want %d", got, want)
	}
	if got, want := repo.upsertCall, 1; got != want {
		t.Fatalf("upsertCall = %d, want %d", got, want)
	}

	if repo.upserts[0].ProgramID != "p1" || repo.upserts[0].AmountRaisedCents != 100 {
		t.Fatalf("p1 upsert = %+v, want amount cents 100", repo.upserts[0])
	}
	if repo.upserts[1].ProgramID != "p2" || repo.upserts[1].AmountRaisedCents != 300 {
		t.Fatalf("p2 upsert = %+v, want amount cents 300", repo.upserts[1])
	}
}

func TestSyncerCreatesZeroRowsForProgramsWithoutTransactions(t *testing.T) {
	t.Parallel()

	repo := &fakeFundingRepo{programIDs: []string{"p1", "p2"}}
	ledger := &fakeLedger{pages: []*clients.LedgerTransactionsPage{{HasNext: false, Transactions: []clients.LedgerTransaction{}}}}

	s := newSyncer(repo, ledger, slog.New(slog.NewTextHandler(io.Discard, nil)), 100)
	result, err := s.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if got, want := result.matchedPrograms, 0; got != want {
		t.Fatalf("matchedPrograms = %d, want %d", got, want)
	}
	if got, want := result.skippedPrograms, 2; got != want {
		t.Fatalf("skippedPrograms = %d, want %d", got, want)
	}

	if got, want := len(repo.upserts), 2; got != want {
		t.Fatalf("upsert count = %d, want %d", got, want)
	}
	if got, want := repo.upsertCall, 1; got != want {
		t.Fatalf("upsertCall = %d, want %d", got, want)
	}
	for _, row := range repo.upserts {
		if row.AmountRaisedCents != 0 {
			t.Fatalf("row for %s has amount cents %d, want 0", row.ProgramID, row.AmountRaisedCents)
		}
	}
}

func TestSyncerDryRunSkipsUpsert(t *testing.T) {
	t.Parallel()

	repo := &fakeFundingRepo{programIDs: []string{"p1", "p2"}}
	ledger := &fakeLedger{pages: []*clients.LedgerTransactionsPage{{
		HasNext: false,
		Transactions: []clients.LedgerTransaction{
			{ProjectID: "p1", TxnCategory: "mentorship", Amount: 100},
			{ProjectID: "p2", TxnCategory: "mentorship", Amount: 50},
		},
	}}}

	s := newSyncer(repo, ledger, slog.New(slog.NewTextHandler(io.Discard, nil)), 100)
	s.dryRun = true

	result, err := s.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if got, want := result.plannedUpserts, 2; got != want {
		t.Fatalf("plannedUpserts = %d, want %d", got, want)
	}
	if got, want := result.upserted, 0; got != want {
		t.Fatalf("upserted = %d, want %d", got, want)
	}
	if got, want := repo.upsertCall, 0; got != want {
		t.Fatalf("upsertCall = %d, want %d", got, want)
	}
	if got, want := len(repo.upserts), 0; got != want {
		t.Fatalf("upsert rows = %d, want %d", got, want)
	}
}
