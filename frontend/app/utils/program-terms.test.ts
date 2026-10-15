// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { describe, it, expect } from 'vitest';
import type { ProgramTerm } from '../types/program.types';
import { getActiveTerms, getProgramTermDisplayStatus } from './program-terms';

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
    ['after the end day', '2026-08-02T00:00:00Z', 'completed'],
  ])('%s', (_, now, want) => {
    expect(getProgramTermDisplayStatus(term(), new Date(now))).toBe(want);
    expect(getActiveTerms([term()], new Date(now))).toHaveLength(want === 'accepting' ? 1 : 0);
  });

  it('treats a term missing either window date as not accepting', () => {
    const now = new Date('2026-07-01T00:00:00Z');
    for (const t of [
      term({ applicationsStartAt: undefined }),
      term({ applicationsCloseAt: undefined }),
    ]) {
      expect(getActiveTerms([t], now)).toHaveLength(0);
      expect(getProgramTermDisplayStatus(t, now)).toBe('completed');
    }
  });

  it('never accepts on a closed term', () => {
    const now = new Date('2026-07-01T00:00:00Z');
    expect(getActiveTerms([term({ status: 'closed' })], now)).toHaveLength(0);
    expect(getProgramTermDisplayStatus(term({ status: 'closed' }), now)).toBe('completed');
  });
});
