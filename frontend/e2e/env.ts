// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// The deployed environments the E2E suites run against, selected with E2E_ENV (default prod).
// Matches the URL table in docs/testing/e2e-test-plan.md.

interface E2eEnvironment {
  publicUrl: string;
  selfServeUrl: string;
  // The legacy site's host, which now redirects here; only PROD kept one.
  legacyUrl?: string;
}

const ENVIRONMENTS: Record<string, E2eEnvironment> = {
  prod: {
    publicUrl: 'https://mentorship.linuxfoundation.org',
    selfServeUrl: 'https://app.lfx.dev',
    legacyUrl: 'https://mentorship.lfx.linuxfoundation.org',
  },
  dev: {
    publicUrl: 'https://mentorship.dev.lfx.dev',
    selfServeUrl: 'https://app.dev.lfx.dev',
  },
};

const name = process.env.E2E_ENV ?? 'prod';
const selected = ENVIRONMENTS[name];
if (!selected) {
  throw new Error(`E2E_ENV must be one of ${Object.keys(ENVIRONMENTS).join(', ')}; got "${name}"`);
}

export const env = { name, ...selected };
