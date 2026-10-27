// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import fs from 'node:fs';
import { expect, test } from '@playwright/test';
import { PROGRAM_A } from '../support/test-data';
import { storageStatePath } from '../support/users';

const PROTECTED_ENDPOINTS = [
  '/api/mentorship/admin/programs',
  `/api/mentorship/admin/programs/${PROGRAM_A.id}`,
  '/api/mentorship/mentor/programs',
  '/api/mentorship/mentee/has-profile',
];

// Section 12 of docs/testing/p1-test-plan.md: BFF authentication, called directly.
test.describe('12. API and hardening', () => {
  test('12.1 No session on /api/mentorship/* gives 401 JSON [BFF-01-01, ADM-01-06]', async ({ playwright, baseURL }) => {
    const api = await playwright.request.newContext({ baseURL });

    for (const endpoint of PROTECTED_ENDPOINTS) {
      const response = await api.get(endpoint, { maxRedirects: 0 });
      expect.soft(response.status(), endpoint).toBe(401);
      expect.soft(response.headers()['content-type'] ?? '', endpoint).toContain('application/json');
    }

    await api.dispose();
  });

  test('12.4 Tampered appSession cookie gives 401 [BFF-01-04]', async ({ playwright, baseURL }) => {
    const statePath = storageStatePath('adminA');
    test.skip(!fs.existsSync(statePath), 'No session for adminA');

    const state = JSON.parse(fs.readFileSync(statePath, 'utf8'));
    const sessionCookies = state.cookies.filter((cookie: { name: string }) => cookie.name.startsWith('appSession'));
    expect(sessionCookies.length, 'adminA session has an appSession cookie').toBeGreaterThan(0);
    for (const cookie of sessionCookies) {
      cookie.value = `${cookie.value.slice(0, -4)}AAAA`;
    }

    const api = await playwright.request.newContext({ baseURL, storageState: state });
    const response = await api.get('/api/mentorship/admin/programs', { maxRedirects: 0 });
    expect(response.status()).toBe(401);
    await api.dispose();
  });
});
