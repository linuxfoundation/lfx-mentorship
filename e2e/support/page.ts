// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { Page } from '@playwright/test';

/**
 * Opens a Self Serve page and waits for Angular to hydrate the server render: anything typed before then is wiped.
 * Hydration removes the `ngh` attributes the server render carries.
 */
export async function gotoHydrated(page: Page, path: string): Promise<void> {
  await page.goto(path);
  await page.waitForFunction(() => !document.querySelector('[ngh]'));
}
