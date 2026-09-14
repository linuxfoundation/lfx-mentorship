// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/clients"
)

type fundingStatsRepository interface {
	ListFundingSyncProgramIDs(ctx context.Context) ([]string, error)
	BulkUpsertFundingStats(ctx context.Context, rows []models.ProgramFundingStatsUpsert) (int, error)
}

type ledgerSource interface {
	GetTransactionsPage(ctx context.Context, projectID, txnType string, page int, perPage int) (*clients.LedgerTransactionsPage, error)
}

type syncResult struct {
	programs        int
	matchedPrograms int
	skippedPrograms int
	processedTxns   int
	unmappedTxns    int
	pagesFetched    int
	plannedUpserts  int
	upserted        int
}

type syncer struct {
	repo    fundingStatsRepository
	ledger  ledgerSource
	logger  *slog.Logger
	perPage int
	dryRun  bool
}

func newSyncer(repo fundingStatsRepository, ledger ledgerSource, logger *slog.Logger, perPage int) *syncer {
	if perPage <= 0 {
		perPage = 100
	}
	if perPage > 100 {
		perPage = 100
	}
	return &syncer{repo: repo, ledger: ledger, logger: logger, perPage: perPage}
}

func (s *syncer) Run(ctx context.Context) (syncResult, error) {
	programIDs, err := s.repo.ListFundingSyncProgramIDs(ctx)
	if err != nil {
		return syncResult{}, fmt.Errorf("list sync program IDs: %w", err)
	}

	result := syncResult{programs: len(programIDs)}
	if len(programIDs) == 0 {
		s.logger.InfoContext(ctx, "no active programs found; nothing to sync")
		return result, nil
	}

	raised, spent, pagesFetched, processedTxns, unmappedTxns, err := s.fetchTotals(ctx, programIDs)
	if err != nil {
		return syncResult{}, err
	}
	result.pagesFetched = pagesFetched
	result.processedTxns = processedTxns
	result.unmappedTxns = unmappedTxns

	rows := make([]models.ProgramFundingStatsUpsert, 0, len(programIDs))
	for _, id := range programIDs {
		raisedCents := raised[id]
		if raisedCents > 0 || spent[id] > 0 {
			result.matchedPrograms++
		} else {
			result.skippedPrograms++
		}
		rows = append(rows, models.ProgramFundingStatsUpsert{
			ProgramID:         id,
			AmountRaisedCents: raisedCents,
			AmountSpentCents:  spent[id],
		})
	}
	result.plannedUpserts = len(rows)

	if s.dryRun {
		s.logger.InfoContext(ctx, "dry-run mode enabled; skipping funding stats upsert", "planned_upserts", result.plannedUpserts)
		return result, nil
	}

	upserted, err := s.repo.BulkUpsertFundingStats(ctx, rows)
	if err != nil {
		return syncResult{}, fmt.Errorf("bulk upsert funding stats: %w", err)
	}
	result.upserted = upserted
	return result, nil
}

func (s *syncer) fetchTotals(ctx context.Context, programIDs []string) (map[string]int64, map[string]int64, int, int, int, error) {
	raised := make(map[string]int64, len(programIDs))
	spent := make(map[string]int64, len(programIDs))
	pagesFetched := 0
	processedTxns := 0
	unmappedTxns := 0

	for _, programID := range programIDs {
		for _, txnType := range []string{"credit", "debit"} {
			page := 1
			for {
				resp, err := s.ledger.GetTransactionsPage(ctx, programID, txnType, page, s.perPage)
				if err != nil {
					return nil, nil, pagesFetched, processedTxns, unmappedTxns, fmt.Errorf("fetch ledger %s page %d for project %s: %w", txnType, page, programID, err)
				}
				pagesFetched++

				for _, txn := range resp.Transactions {
					if txn.Amount == 0 {
						continue
					}
					if !txn.TxnCategory.IsValid() {
						continue
					}
					if txn.TxnCategory != models.MentorshipCategory {
						continue
					}
					if txn.ProjectID != "" && txn.ProjectID != programID {
						unmappedTxns++
						continue
					}
					if txnType == "credit" && txn.Amount > 0 {
						raised[programID] += txn.Amount
					} else if txnType == "debit" {
						spent[programID] -= txn.Amount
					} else {
						continue
					}
					processedTxns++
				}

				if !resp.HasNext {
					break
				}
				page++
			}
		}
	}
	for programID, amount := range spent {
		if amount < 0 {
			spent[programID] = 0
		}
	}

	return raised, spent, pagesFetched, processedTxns, unmappedTxns, nil
}
