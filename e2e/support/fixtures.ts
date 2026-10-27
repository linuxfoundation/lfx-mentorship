// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import fs from 'node:fs';
import { BrowserContext, Page, test as base } from '@playwright/test';
import { HIDE_COOKIE_BANNER } from './auth';
import { storageStatePath, USERS, UserKey } from './users';

export type AsUser = (key: UserKey) => Promise<Page>;

/**
 * `asUser('adminA')` opens a page signed in as that user, in its own browser context, so one test
 * can drive several users side by side (for example an admin accepting a mentee's application).
 * Skips the test when the setup project has no session for the user.
 */
export const test = base.extend<{ asUser: AsUser }>({
  asUser: async ({ browser, baseURL }, use, testInfo) => {
    const contexts: BrowserContext[] = [];

    await use(async (key) => {
      const statePath = storageStatePath(key);
      if (!fs.existsSync(statePath)) {
        testInfo.skip(true, `No session for ${key}: set ${USERS[key].envPrefix}_USERNAME and _PASSWORD in e2e/.env`);
      }
      const context = await browser.newContext({ baseURL, storageState: statePath });
      // The cookie banner covers the bottom of every page, including dialog buttons.
      await context.addInitScript((css) => {
        document.addEventListener('DOMContentLoaded', () => {
          const style = document.createElement('style');
          style.textContent = css;
          document.head.append(style);
        });
      }, HIDE_COOKIE_BANNER);
      contexts.push(context);
      return context.newPage();
    });

    await Promise.all(contexts.map((context) => context.close()));
  },
});

export { expect } from '@playwright/test';
