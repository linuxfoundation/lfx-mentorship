// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { Program, ProgramStatus, ProgramTerm } from '../types/program.types';

/** Public-facing term lifecycle shown on the program Terms tab. */
export const PROGRAM_TERM_DISPLAY_STATUSES = [
  'opens-soon',
  'accepting',
  'in-progress',
  'completed',
] as const;

export type ProgramTermDisplayStatus = (typeof PROGRAM_TERM_DISPLAY_STATUSES)[number];

/**
 * Where `now` falls against a term's application window. As in the backend
 * (`ProgramTerm.ApplicationWindow`), the window is compared by UTC calendar
 * day with both days inclusive, and a term missing either date has no window.
 */
type ApplicationWindow = 'unset' | 'upcoming' | 'open' | 'closed';

function getApplicationWindow(term: ProgramTerm, now: Date): ApplicationWindow {
  const startMs = parseTime(term.applicationsStartAt);
  const closeMs = parseTime(term.applicationsCloseAt);
  if (startMs === null || closeMs === null) return 'unset';

  const today = utcDay(now.getTime());
  if (today < utcDay(startMs)) return 'upcoming';
  if (today > utcDay(closeMs)) return 'closed';
  return 'open';
}

/** Terms currently accepting applications: status is open and today is inside the application window. */
export function getActiveTerms(terms: ProgramTerm[], now: Date = new Date()): ProgramTerm[] {
  return terms.filter(
    (term) => term.status === 'open' && getApplicationWindow(term, now) === 'open',
  );
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

  switch (getApplicationWindow(term, now)) {
    case 'upcoming':
      return 'opens-soon';
    case 'open':
      return 'accepting';
    default:
      // Application window passed or unset, but the term is still open: it is running.
      return 'in-progress';
  }
}

/**
 * Catalog / program-card badge: derived from term application windows
 * the same way `GET /v1/programs/catalog` is mapped for the programs list.
 * Never `pending`, which the caller sets for a pending program; the narrower
 * type keeps the result assignable to both `ProgramStatus` and `ProfileProgramStatus`.
 */
export function toProgramCardStatus(terms: ProgramTerm[]): Exclude<ProgramStatus, 'pending'> {
  const displays = terms.map((term) => getProgramTermDisplayStatus(term));
  if (displays.some((status) => status === 'accepting')) return 'acceptance';
  if (displays.some((status) => status === 'opens-soon')) return 'open-soon';
  if (displays.some((status) => status === 'in-progress')) return 'in-progress';
  return 'completed';
}

/** Newest term first for the Terms tab table. */
export function sortTermsNewestFirst(terms: ProgramTerm[]): ProgramTerm[] {
  return [...terms].sort((a, b) => (parseTime(b.startsAt) ?? 0) - (parseTime(a.startsAt) ?? 0));
}

/** Midnight UTC of the instant's UTC calendar day, in ms. */
function utcDay(ms: number): number {
  const day = new Date(ms);
  return Date.UTC(day.getUTCFullYear(), day.getUTCMonth(), day.getUTCDate());
}

function parseTime(iso: string | undefined): number | null {
  if (!iso) return null;
  const ms = new Date(iso).getTime();
  return Number.isNaN(ms) ? null : ms;
}
