// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { Page } from '@playwright/test';
import { expect, test } from '../support/fixtures';
import { gotoHydrated } from '../support/page';
import { fillMenteeRegisterForm } from '../support/profiles';
import { applyPath, PROGRAM_A } from '../support/test-data';

/** Fails, rather than passes, when mentee2 is no longer brand-new: these cases only mean something on its first visit. */
async function expectNoMenteeProfile(page: Page): Promise<void> {
  const response = await page.request.get('/api/mentorship/mentee/has-profile');
  expect(response.status(), 'GET mentee/has-profile').toBe(200);
  const { hasProfile } = (await response.json()) as { hasProfile: boolean };
  expect(hasProfile, 'mentee2 already has a mentee profile, so this is not its first Mentorship visit: 12.6 and 8.3 need a brand-new account').toBe(false);
}

// Cases 12.6 and 8.3 of docs/testing/p1-test-plan.md: mentee2 (samehhotmail) is a brand-new user, and these use up that state.
// The file is numbered 00 so it runs before any other spec signs in as mentee2.
test.describe('First Mentorship visit (mentee2)', () => {
  test.describe.configure({ mode: 'serial' });

  test('12.6 First Mentorship call by samehhotmail [BFF-08-01]', async ({ asUser }, testInfo) => {
    const page = await asUser('mentee2');
    const calls: string[] = [];
    page.on('response', (response) => {
      const { pathname } = new URL(response.url());
      if (pathname.startsWith('/api/mentorship/')) calls.push(`${response.request().method()} ${pathname} ${response.status()}`);
    });

    // The backend answers a first call 401 "not provisioned"; the BFF then provisions the user (PUT /me) and retries, so the
    // browser should only ever see the retry's answer.
    // This visit can only happen once, so the calls are kept in the report whatever fails below.
    try {
      const firstRequest = page.waitForRequest((request) => new URL(request.url()).pathname.startsWith('/api/mentorship/'));
      await gotoHydrated(page, '/mentorship/mentee');
      const first = await firstRequest;
      const firstResponse = await first.response();

      await expectNoMenteeProfile(page);
      expect(firstResponse?.status(), `first call: ${first.method()} ${new URL(first.url()).pathname}`).toBe(200);

      await expect(page.getByTestId('mentorship-mentee-register-title')).toBeVisible();
      await expect(page.locator('.p-toast-message-error')).toHaveCount(0);
      expect(calls.filter((call) => / [45]\d\d$/.test(call)), 'failed Mentorship calls').toEqual([]);
    } finally {
      await testInfo.attach('mentorship-calls', { body: calls.join('\n'), contentType: 'text/plain' });
    }
  });

  test('8.3 Apply link with no profile [MTE-04-01]', async ({ asUser }) => {
    const page = await asUser('mentee2');
    await expectNoMenteeProfile(page);
    const ids = `programId=${PROGRAM_A.id}&programTermId=${PROGRAM_A.terms.a.id}`;

    await gotoHydrated(page, applyPath(PROGRAM_A.id, PROGRAM_A.terms.a.id));
    await expect(page).toHaveURL(new RegExp(`/mentorship/mentee\\?${ids}$`));
    await expect(page.getByTestId('mentorship-mentee-register-title')).toBeVisible();

    await fillMenteeRegisterForm(page);
    const register = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/api/mentorship/mentee/profile'));
    await page.getByTestId('mentorship-mentee-submit').click();
    expect((await register).status(), 'POST mentee/profile').toBeLessThan(300);
    await expect(page.getByText('Your mentee profile has been saved.')).toBeVisible();

    // e2e F14 (lfx-self-serve#4232): a first-time applicant from an Apply link stays on the register form after saving.
    const backOnApply = await page.waitForURL(/\/mentorship\/mentee\/apply\?/, { timeout: 15_000 }).then(
      () => true,
      () => false
    );
    test.fail(!backOnApply && (await page.getByTestId('mentorship-mentee-register').isVisible()), 'e2e F14 (lfx-self-serve#4232): stays on the register form');
    await expect(page).toHaveURL(new RegExp(`/mentorship/mentee/apply\\?${ids}$`));
    await expect(page.getByTestId('mentorship-mentee-apply-title')).toContainText(PROGRAM_A.name);
    await expect(page.getByTestId('mentorship-mentee-apply-before')).toHaveCount(0);
    await expect(page.getByTestId('mentorship-mentee-apply-submit').getByRole('button')).toBeEnabled();
    // Not submitted here: section 8 applies.
  });
});
