// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { describe, it, expect } from 'vitest';
import type { Program, ProgramTerm } from '../types/program.types';
import {
  formatTermLabel,
  getActiveTerms,
  getProgramTermDisplayStatus,
  sortTermsNewestFirst,
  toProgramCardStatus,
  withActiveTerms,
} from './program-terms';

const term = (overrides: Partial<ProgramTerm> = {}): ProgramTerm => ({
  id: 't1',
  name: 'Fall',
  status: 'open',
  startsAt: '2026-09-01',
  endsAt: '2026-11-30',
  applicationsStartAt: '2026-06-01T15:00:00Z',
  applicationsCloseAt: '2026-08-01T00:00:00Z',
  ...overrides,
});

describe('application window', () => {
  it.each([
    ['before the start day', '2026-05-31T23:59:59Z', 'opens-soon'],
    ['on the start day before its stored time', '2026-06-01T00:00:00Z', 'accepting'],
    ['through the last second of the end day', '2026-08-01T23:59:59Z', 'accepting'],
    ['after the end day, while the term is still open', '2026-08-02T00:00:00Z', 'in-progress'],
  ])('%s', (_, now, want) => {
    expect(getProgramTermDisplayStatus(term(), new Date(now))).toBe(want);
    expect(getActiveTerms([term()], new Date(now))).toHaveLength(want === 'accepting' ? 1 : 0);
  });

  it('treats an open term missing either window date as in progress, not accepting', () => {
    const now = new Date('2026-07-01T00:00:00Z');
    for (const t of [
      term({ applicationsStartAt: undefined }),
      term({ applicationsCloseAt: undefined }),
    ]) {
      expect(getActiveTerms([t], now)).toHaveLength(0);
      expect(getProgramTermDisplayStatus(t, now)).toBe('in-progress');
    }
  });

  it('never accepts on a closed term', () => {
    const now = new Date('2026-07-01T00:00:00Z');
    expect(getActiveTerms([term({ status: 'closed' })], now)).toHaveLength(0);
    expect(getProgramTermDisplayStatus(term({ status: 'closed' }), now)).toBe('completed');
  });

  it('treats a closed or deleted term as completed once its window has passed', () => {
    const now = new Date('2026-08-02T00:00:00Z');
    expect(getProgramTermDisplayStatus(term({ status: 'closed' }), now)).toBe('completed');
    expect(getProgramTermDisplayStatus(term({ status: 'deleted' }), now)).toBe('completed');
  });

  it('treats an open term with an unparseable window date as in progress', () => {
    const now = new Date('2026-07-01T00:00:00Z');
    expect(getProgramTermDisplayStatus(term({ applicationsCloseAt: 'not-a-date' }), now)).toBe(
      'in-progress',
    );
  });
});

describe('toProgramCardStatus', () => {
  // It reads the real clock, so windows are relative to now.
  const day = 24 * 60 * 60 * 1000;
  const iso = (offsetDays: number) => new Date(Date.now() + offsetDays * day).toISOString();
  const accepting = term({ applicationsStartAt: iso(-2), applicationsCloseAt: iso(2) });
  const opensSoon = term({ applicationsStartAt: iso(5), applicationsCloseAt: iso(10) });
  const pastWindow = term({ applicationsStartAt: iso(-10), applicationsCloseAt: iso(-5) });
  const finished = term({ status: 'closed' });

  it.each([
    ['any term accepting', [finished, opensSoon, accepting], 'acceptance'],
    ['a term opening soon', [finished, opensSoon], 'open-soon'],
    ['an open term past its window', [finished, pastWindow], 'in-progress'],
    ['only closed terms', [finished], 'completed'],
    ['no terms', [], 'completed'],
  ])('%s', (_, terms, want) => {
    expect(toProgramCardStatus(terms)).toBe(want);
  });
});

describe('term helpers', () => {
  it('sorts newest start first, undated last, without mutating the input', () => {
    const input = [
      term({ id: 'older', startsAt: '2025-01-01' }),
      term({ id: 'undated', startsAt: '' }),
      term({ id: 'newer', startsAt: '2026-01-01' }),
    ];
    expect(sortTermsNewestFirst(input).map((t) => t.id)).toEqual(['newer', 'older', 'undated']);
    expect(input.map((t) => t.id)).toEqual(['older', 'undated', 'newer']);
  });

  it('appends the date range to the label when present', () => {
    expect(formatTermLabel(term({ dateRangeLabel: 'Sep-Nov 2026' }))).toBe('Fall · Sep-Nov 2026');
    expect(formatTermLabel(term())).toBe('Fall');
  });

  it('derives activeTerms from the program terms', () => {
    const program = { terms: [term({ status: 'closed' })] } as unknown as Program;
    expect(withActiveTerms(program).activeTerms).toEqual([]);
  });
});
