// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import fs from 'node:fs';
import { expect, test as setup } from '@playwright/test';
import { loginWithAuth0 } from '../support/auth';
import { credentialsFor, storageStatePath, USER_KEYS, USERS } from '../support/users';

for (const key of USER_KEYS) {
  setup(`sign in as ${key}`, async ({ browser, baseURL }) => {
    const statePath = storageStatePath(key);
    const credentials = credentialsFor(key);
    if (!credentials) {
      // Remove a session left by an earlier run, so specs skip instead of using stale cookies.
      fs.rmSync(statePath, { force: true });
      setup.skip(true, `${USERS[key].envPrefix}_USERNAME and _PASSWORD are not set`);
      return;
    }

    const context = await browser.newContext({ baseURL });
    const page = await context.newPage();

    // Sign in from the dashboard, not a Mentorship page, so a brand-new user (mentee2) makes no
    // Mentorship call before case 12.6 checks its first one.
    await page.goto('/');
    await loginWithAuth0(page, credentials);
    await expect(page).not.toHaveURL(/auth0\.com/);

    await context.storageState({ path: statePath });
    await context.close();
  });
}
