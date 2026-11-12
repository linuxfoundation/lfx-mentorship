// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { defineConfig, devices } from '@playwright/test';
import { env } from './e2e/env';

// The E2E suites run against a deployed environment (E2E_ENV=prod|dev, default prod), never a
// local server: they check what users get, across the public site, the backend and Self Serve.
// See e2e/README.md.
export default defineConfig({
  testDir: './e2e',
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: env.publicUrl,
    trace: 'retain-on-failure',
  },
  projects: [
    {
      // Signed out and read-only, so it is safe to run against PROD.
      name: 'smoke',
      testDir: './e2e/smoke',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
});
