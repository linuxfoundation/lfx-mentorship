// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/clients"
)

type fundingStatsRepository interface {
	ListFundingSyncProgramIDs(ctx context.Context) ([]string, error)
	BulkUpsertFundingStats(ctx context.Context, rows []models.ProgramFundingStatsUpsert) (int, error)
}

type ledgerSource interface {
	GetTransactionsPage(ctx context.Context, page int, perPage int) (*clients.LedgerTransactionsPage, error)
}

type syncResult struct {
	programs        int
	matchedPrograms int
	skippedPrograms int
	processedTxns   int
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

	active := make(map[string]struct{}, len(programIDs))
	for _, id := range programIDs {
		active[id] = struct{}{}
	}

	totals, pagesFetched, processedTxns, err := s.fetchTotals(ctx, active)
	if err != nil {
		return syncResult{}, err
	}
	result.pagesFetched = pagesFetched
	result.processedTxns = processedTxns

	rows := make([]models.ProgramFundingStatsUpsert, 0, len(programIDs))
	for _, id := range programIDs {
		cents := totals[id]
		if cents > 0 {
			result.matchedPrograms++
		} else {
			result.skippedPrograms++
		}
		rows = append(rows, models.ProgramFundingStatsUpsert{
			ProgramID:         id,
			AmountRaisedCents: cents,
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

func (s *syncer) fetchTotals(ctx context.Context, active map[string]struct{}) (map[string]int64, int, int, error) {
	totals := make(map[string]int64, len(active))
	page := 1
	pagesFetched := 0
	processedTxns := 0

	for {
		resp, err := s.ledger.GetTransactionsPage(ctx, page, s.perPage)
		if err != nil {
			return nil, pagesFetched, processedTxns, fmt.Errorf("fetch ledger page %d: %w", page, err)
		}
		pagesFetched++

		for _, txn := range resp.Transactions {
			if txn.Amount <= 0 {
				continue
			}
			if !strings.EqualFold(txn.TxnCategory, string(models.MentorshipCategory)) {
				continue
			}
			if _, ok := active[txn.ProjectID]; !ok {
				continue
			}
			totals[txn.ProjectID] += txn.Amount
			processedTxns++
		}

		if !resp.HasNext {
			break
		}
		page++
	}

	return totals, pagesFetched, processedTxns, nil
}
