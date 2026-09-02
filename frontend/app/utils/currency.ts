// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

/** Formats cents as a whole-dollar USD string (e.g. "$24,000"). */
export function formatUsdFromCents(cents: number): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 0,
  }).format(cents / 100);
}

/** Formats dollars as compact USD (e.g. "$1.3K", "$6.1M"). */
export function formatCompactUsd(dollars: number): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    notation: 'compact',
    compactDisplay: 'short',
    maximumFractionDigits: 1,
  }).format(dollars);
}

export function formatCount(count: number): string {
  return new Intl.NumberFormat('en-US').format(count);
}
