// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

/** Formats an ISO date as "Jul 15, 2026". */
export function formatProgramDate(isoDate: string): string {
  const date = parseUtcDate(isoDate);
  if (!date) return isoDate;

  return new Intl.DateTimeFormat('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    timeZone: 'UTC',
  }).format(date);
}

/**
 * Formats an inclusive date range.
 * Same year: "Aug 3 – Oct 15, 2026"
 * Different years: "Nov 3, 2025 – Jan 15, 2026"
 */
export function formatProgramDateRange(startIso: string, endIso: string): string {
  const start = parseUtcDate(startIso);
  const end = parseUtcDate(endIso);
  if (!start || !end) return `${startIso} – ${endIso}`;

  const sameYear = start.getUTCFullYear() === end.getUTCFullYear();
  const startLabel = new Intl.DateTimeFormat('en-US', {
    month: 'short',
    day: 'numeric',
    ...(sameYear ? {} : { year: 'numeric' }),
    timeZone: 'UTC',
  }).format(start);
  const endLabel = formatProgramDate(endIso);

  return `${startLabel} – ${endLabel}`;
}

function parseUtcDate(isoDate: string): Date | null {
  const date = new Date(isoDate);
  return Number.isNaN(date.getTime()) ? null : date;
}
