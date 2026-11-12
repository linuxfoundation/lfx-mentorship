// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { existsSync } from 'node:fs';
import { test as base, type BrowserContext, type Page } from '@playwright/test';
import { storageStatePath, type User } from '../users';
import { HIDE_COOKIE_BANNER } from './auth';

type AsUser = (user: User) => Promise<Page>;

/**
 * `asUser('ADMIN_A')` opens a page signed in as that user, in its own browser context, so one test
 * drives several users side by side. Skips the test when the setup project saved no session.
 */
export const test = base.extend<{ asUser: AsUser }>({
  asUser: async ({ browser, baseURL }, use, testInfo) => {
    const contexts: BrowserContext[] = [];
    await use(async (user) => {
      const storageState = storageStatePath(user);
      testInfo.skip(!existsSync(storageState), `no signed-in session for ${user}`);
      const context = await browser.newContext({ baseURL, storageState });
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

/** Opens a Self Serve page once Angular has hydrated its server render: earlier input is wiped. */
export async function gotoHydrated(page: Page, path: string): Promise<void> {
  await page.goto(path);
  await page.waitForFunction(() => !document.querySelector('[ngh]'));
}
