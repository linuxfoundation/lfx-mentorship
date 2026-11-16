// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { existsSync } from 'node:fs';
import { defineConfig, devices } from '@playwright/test';

// Locally the DEV test accounts come from .env (gitignored); in CI from secrets. Loaded before
// e2e/env, which reads E2E_ENV.
const dotenv = new URL('.env', import.meta.url);
if (existsSync(dotenv)) process.loadEnvFile(dotenv);
const { env } = await import('./e2e/env');

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
    {
      // Signs the journey users in on DEV; skipped when their credentials are not set. No traces
      // or screenshots: they would record the credentials.
      name: 'setup',
      testMatch: /auth\.setup\.ts/,
      use: { ...devices['Desktop Chrome'], trace: 'off', screenshot: 'off' },
    },
    {
      // One program from enrollment to graduation on DEV, with several signed-in users. Its steps
      // build on each other and create data, so it runs serially and is never retried.
      name: 'journey',
      testDir: './e2e/journey',
      dependencies: ['setup'],
      retries: 0,
      timeout: 20 * 60_000,
      expect: { timeout: 30_000 },
      use: {
        ...devices['Desktop Chrome'],
        baseURL: env.selfServeUrl,
        // The backend works in UTC and the journey's dates are UTC, so the browser's "today" must be too.
        timezoneId: 'UTC',
        // Self Serve pages show the test accounts' LFX profiles (email, address, phone), so CI,
        // whose reports are downloadable, keeps no screenshots or traces of them.
        screenshot: process.env.CI ? 'off' : 'only-on-failure',
        trace: process.env.CI ? 'off' : 'retain-on-failure',
        // Without these a click on a missing element waits for the whole test timeout.
        actionTimeout: 30_000,
        navigationTimeout: 60_000,
      },
    },
  ],
});
