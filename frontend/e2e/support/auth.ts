// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { Locator, Page } from '@playwright/test';
import type { Credentials } from '../users';

const AUTH0_URL = /auth0\.com/;

// The cookie consent banner covers the sign-in button, and the bottom of Self Serve pages.
export const HIDE_COOKIE_BANNER =
  '#transcend-consent-manager { display: none !important; pointer-events: none !important; }';

// Enters a credential without Playwright recording it: fill() and type() title their report
// steps with the text (`Fill "<value>"`), and reports are uploaded from CI. Evaluate steps are
// titled without their arguments; the setup project also turns traces off.
async function enterCredential(field: Locator, value: string): Promise<void> {
  await field.evaluate((input: HTMLInputElement, text: string) => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, text);
    input.dispatchEvent(new Event('input', { bubbles: true }));
    input.dispatchEvent(new Event('change', { bubbles: true }));
  }, value);
}

/** Signs in on the LFX sign-in page; the page must be on it, or redirecting to it. */
export async function signIn(page: Page, { username, password }: Credentials): Promise<void> {
  await page.waitForURL(AUTH0_URL);
  await page.addStyleTag({ content: HIDE_COOKIE_BANNER });
  await enterCredential(page.getByRole('textbox', { name: 'Username or Email' }), username);
  await enterCredential(page.getByRole('textbox', { name: 'Password' }), password);
  // The banner can mount after the form, so hide it again just before clicking.
  await page.addStyleTag({ content: HIDE_COOKIE_BANNER });
  await page.getByRole('button', { name: 'SIGN IN' }).click();
  // Auth0 may ask for consent after an account's first sign-in.
  const consent = page.getByRole('button', { name: /accept|authorize|allow/i }).first();
  await page.waitForURL(
    (url) => !AUTH0_URL.test(url.href) || url.pathname.startsWith('/u/consent'),
  );
  if (new URL(page.url()).pathname.startsWith('/u/consent')) await consent.click();
  await page.waitForURL((url) => !AUTH0_URL.test(url.href));
}
