// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

/** User-facing message from a `$fetch` / Nuxt error, with a fallback. */
export function getFetchErrorMessage(error: unknown, fallback: string): string {
  if (typeof error !== 'object' || error === null) return fallback;

  const err = error as {
    data?: { message?: string };
    statusMessage?: string;
    message?: string;
  };

  return err.data?.message || err.statusMessage || err.message || fallback;
}
