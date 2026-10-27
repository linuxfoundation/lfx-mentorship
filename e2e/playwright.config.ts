// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import path from 'node:path';
import { defineConfig, devices } from '@playwright/test';

try {
  process.loadEnvFile(path.resolve(__dirname, '.env'));
} catch (err) {
  if ((err as NodeJS.ErrnoException).code !== 'ENOENT') {
    throw err;
  }
}

export default defineConfig({
  testDir: './tests',
  // The P1 plan uses up state as it goes (a fresh account registers once, a mentee graduates
  // last), so specs run one at a time, in file order.
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env['CI'],
  retries: 0,
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    ...devices['Desktop Chrome'],
    baseURL: process.env['E2E_SELF_SERVE_URL'] ?? 'https://app.dev.lfx.dev',
    // The backend works in UTC, and test dates are UTC (support/enroll.ts), so the browser's "today" must be too.
    timezoneId: 'UTC',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    headless: true,
  },
  projects: [
    { name: 'setup', testMatch: /auth\.setup\.ts/ },
    { name: 'p1', testMatch: /.*\.spec\.ts/, dependencies: ['setup'] },
  ],
});
