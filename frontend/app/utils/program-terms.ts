// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { Program, ProgramStatus, ProgramTerm } from '../types/program.types';

/** Public-facing term lifecycle shown on the program Terms tab. */
export const PROGRAM_TERM_DISPLAY_STATUSES = ['opens-soon', 'accepting', 'completed'] as const;

export type ProgramTermDisplayStatus = (typeof PROGRAM_TERM_DISPLAY_STATUSES)[number];

/**
 * Terms currently accepting applications: status is open and now is
 * inside the application window, inclusive on both ends (same as
 * Postgres `NOW() BETWEEN application_start_date AND application_end_date`).
 */
export function getActiveTerms(terms: ProgramTerm[], now: Date = new Date()): ProgramTerm[] {
  const nowMs = now.getTime();

  return terms.filter((term) => {
    if (term.status !== 'open') return false;

    const startMs = parseTime(term.applicationsStartAt);
    const closeMs = parseTime(term.applicationsCloseAt);
    if (startMs === null || closeMs === null) return false;

    return startMs <= nowMs && nowMs <= closeMs;
  });
}

export function withActiveTerms(program: Omit<Program, 'activeTerms'> | Program): Program {
  return {
    ...program,
    activeTerms: getActiveTerms(program.terms),
  };
}

export function formatTermLabel(term: ProgramTerm): string {
  return term.dateRangeLabel ? `${term.name} · ${term.dateRangeLabel}` : term.name;
}

/**
 * Maps a term to the badge state shown on the public Terms tab.
 * Prefers application-window timing over the raw open/closed status.
 */
export function getProgramTermDisplayStatus(
  term: ProgramTerm,
  now: Date = new Date(),
): ProgramTermDisplayStatus {
  if (term.status === 'closed' || term.status === 'deleted') {
    return 'completed';
  }

  const nowMs = now.getTime();
  const applicationsStartMs = parseTime(term.applicationsStartAt);
  const applicationsCloseMs = parseTime(term.applicationsCloseAt);
  const termEndMs = parseTime(term.endsAt);

  if (applicationsStartMs !== null && nowMs < applicationsStartMs) {
    return 'opens-soon';
  }

  if (
    applicationsStartMs !== null &&
    applicationsCloseMs !== null &&
    applicationsStartMs <= nowMs &&
    nowMs <= applicationsCloseMs
  ) {
    return 'accepting';
  }

  if (termEndMs !== null && nowMs > termEndMs) {
    return 'completed';
  }

  // Application window passed but term not finished — treat as completed for listing.
  return 'completed';
}

/**
 * Catalog / program-card badge: derived from term application windows
 * the same way `GET /v1/programs/catalog` is mapped for the programs list.
 */
export function toProgramCardStatus(terms: ProgramTerm[]): ProgramStatus {
  const displays = terms.map((term) => getProgramTermDisplayStatus(term));
  if (displays.some((status) => status === 'accepting')) return 'acceptance';
  if (displays.some((status) => status === 'opens-soon')) return 'open-soon';
  if (terms.some((term) => term.status === 'open')) return 'in-progress';
  return 'completed';
}

/** Newest term first for the Terms tab table. */
export function sortTermsNewestFirst(terms: ProgramTerm[]): ProgramTerm[] {
  return [...terms].sort((a, b) => (parseTime(b.startsAt) ?? 0) - (parseTime(a.startsAt) ?? 0));
}

function parseTime(iso: string | undefined): number | null {
  if (!iso) return null;
  const ms = new Date(iso).getTime();
  return Number.isNaN(ms) ? null : ms;
}
