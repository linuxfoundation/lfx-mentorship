// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { Program, ProgramTerm } from '../types/program.types';

/**
 * Terms currently accepting applications: status is open, the start
 * date has been reached, and the close date has not arrived yet.
 */
export function getActiveTerms(terms: ProgramTerm[], now: Date = new Date()): ProgramTerm[] {
  const nowMs = now.getTime();

  return terms.filter((term) => {
    if (term.status !== 'open') return false;

    const startMs = parseTime(term.applicationsStartAt);
    const closeMs = parseTime(term.applicationsCloseAt);
    if (startMs === null || closeMs === null) return false;

    return startMs <= nowMs && nowMs < closeMs;
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

function parseTime(iso: string | undefined): number | null {
  if (!iso) return null;
  const ms = new Date(iso).getTime();
  return Number.isNaN(ms) ? null : ms;
}
