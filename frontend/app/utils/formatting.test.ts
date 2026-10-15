// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { describe, expect, it } from 'vitest';
import { formatCompactUsd, formatCount, formatUsdFromCents } from './currency';
import { formatProgramDate, formatProgramDateRange } from './date';
import { getFetchErrorMessage } from './fetch-error';
import { plainTextFromHtml, plainTextLength } from './html-text';

describe('plainTextFromHtml', () => {
  it('separates blocks, decodes entities once, and collapses whitespace', () => {
    expect(plainTextFromHtml('<p>a&nbsp;&lt;b&gt;</p><p>&quot;c&quot;\n\n d</p>')).toBe(
      'a <b> "c" d',
    );
    expect(plainTextFromHtml('&amp;lt;')).toBe('&lt;');
    expect(plainTextLength('<p> hi </p>')).toBe(2);
  });
});

describe('currency', () => {
  it('formats amounts', () => {
    expect(formatUsdFromCents(2400000)).toBe('$24,000');
    expect(formatCompactUsd(1300)).toBe('$1.3K');
    expect(formatCompactUsd(6100000)).toBe('$6.1M');
    expect(formatCount(12345)).toBe('12,345');
  });
});

describe('dates', () => {
  it('formats a date in UTC and passes through unparseable input', () => {
    expect(formatProgramDate('2026-07-15')).toBe('Jul 15, 2026');
    expect(formatProgramDate('2026-07-15T23:30:00-05:00')).toBe('Jul 16, 2026');
    expect(formatProgramDate('soon')).toBe('soon');
  });

  it('formats a range, naming the start year only when it differs', () => {
    expect(formatProgramDateRange('2026-08-03', '2026-10-15')).toBe('Aug 3 – Oct 15, 2026');
    expect(formatProgramDateRange('2025-11-03', '2026-01-15')).toBe('Nov 3, 2025 – Jan 15, 2026');
    expect(formatProgramDateRange('2026-08-03', 'tbd')).toBe('2026-08-03 – tbd');
  });
});

describe('getFetchErrorMessage', () => {
  it.each([
    ['a non-object', 'boom', 'fallback'],
    ['null', null, 'fallback'],
    [
      'a server message',
      { data: { message: 'Program not found' }, statusMessage: 'Not Found' },
      'Program not found',
    ],
    ['a status message', { statusMessage: 'Bad Gateway', message: 'x' }, 'Bad Gateway'],
    ['an error message', new Error('network'), 'network'],
    ['an empty error', {}, 'fallback'],
  ])('uses %s', (_label, error, want) => {
    expect(getFetchErrorMessage(error, 'fallback')).toBe(want);
  });
});
