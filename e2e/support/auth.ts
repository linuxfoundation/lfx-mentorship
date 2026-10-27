// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { Page } from '@playwright/test';
import { Credentials } from './users';

const AUTH0_URL = /auth0\.com/;

// The Transcend cookie banner covers the Sign In button on the Auth0 page.
export const HIDE_COOKIE_BANNER = '#transcend-consent-manager { display: none !important; pointer-events: none !important; }';

/**
 * Signs in on the LF Auth0 page. Same flow as lfx-self-serve's e2e AuthHelper.loginWithAuth0.
 * The page must already be on, or redirecting to, the Auth0 login page.
 */
export async function loginWithAuth0(page: Page, credentials: Credentials): Promise<void> {
  await page.waitForURL(AUTH0_URL, { timeout: 15_000 });
  await page.addStyleTag({ content: HIDE_COOKIE_BANNER });

  await page.getByRole('textbox', { name: 'Username or Email' }).fill(credentials.username);
  await page.getByRole('textbox', { name: 'Password' }).fill(credentials.password);

  const signIn = page.getByRole('button', { name: 'SIGN IN' });
  await signIn.waitFor({ state: 'visible' });
  // The banner can mount after the form, so hide it again just before clicking.
  await page.addStyleTag({ content: HIDE_COOKIE_BANNER });
  await signIn.click();

  // Auth0 may show a consent page after the first sign-in.
  try {
    await page.waitForURL(/\/u\/consent/, { timeout: 5_000 });
    await page.getByRole('button', { name: /accept|authorize|allow/i }).first().click();
  } catch {
    // No consent page.
  }

  await page.waitForURL((url) => !AUTH0_URL.test(url.href), { timeout: 30_000 });
}
