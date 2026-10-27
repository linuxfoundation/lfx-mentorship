// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { expect, Locator, Page } from '@playwright/test';
import { addSkill, openSelect, pickOption } from './enroll';

/** Types into the one rich editor inside `scope`. */
export async function fillRichText(scope: Locator, text: string): Promise<void> {
  const editor = scope.locator('[contenteditable="true"]');
  await editor.click();
  await editor.fill(text);
}

/** Fills the Become a Mentee form with valid answers; demographics are optional and left empty. */
export async function fillMenteeRegisterForm(page: Page): Promise<void> {
  await fillRichText(page.getByTestId('mentorship-mentee-register-fields'), 'E2E P1 mentee: registered by the Playwright run.');
  await addSkill(page, 'Go', 'mentorship-mentee-have');
  await addSkill(page, 'Kubernetes', 'mentorship-mentee-want');
  await pickOption(page, page.getByTestId('mentorship-mentee-country-select'), 'Egypt', 'Egypt');
  for (const checkbox of ['age-eligible', 'work-authorized', 'no-duplicate-profile', 'compliance', 'terms']) {
    await page.locator(`#mentorship-mentee-${checkbox}`).check();
  }
}

/** Fills the Become a Mentor form with valid answers, leaving the program picks to the caller. */
export async function fillMentorRegisterForm(page: Page): Promise<void> {
  await fillRichText(page.getByTestId('mentorship-mentor-register-fields'), 'E2E P1 mentor: registered by the Playwright run.');
  await addSkill(page, 'Go', 'mentorship-mentor');
  await page.locator('#mentorship-mentor-compliance').check();
  await page.locator('#mentorship-mentor-terms').check();
}

/** The program picker's section, on the Become a Mentor form or in the profile drawer (`scope`). */
export function mentorProgramsSection(scope: Page | Locator): Locator {
  return scope.getByTestId('mentorship-mentor-programs');
}

/** The picker's option for `programName`. Its accessible name also carries a note such as "Pending", so match the label alone. */
export function mentorProgramOption(page: Page, programName: string): Locator {
  return page.locator('.p-select-overlay').getByRole('option').filter({ has: page.getByText(programName, { exact: true }) });
}

/**
 * Searches the mentor program picker in `scope` and opens it on `programName`'s option. The search goes upstream and the list
 * redraws as pages land, so each attempt reopens the picker and searches again.
 */
export async function findMentorProgramOption(page: Page, scope: Page | Locator, programName: string): Promise<Locator> {
  const select = mentorProgramsSection(scope).locator('[data-test="mentorship-mentor-program"]');
  const option = mentorProgramOption(page, programName);
  await expect(async () => {
    const overlay = await openSelect(page, select);
    const filter = overlay.locator('input.p-select-filter');
    if ((await filter.inputValue()) !== programName) await filter.fill(programName);
    await expect(option).toBeVisible({ timeout: 5_000 });
  }).toPass({ timeout: 30_000 });
  return option;
}

/** Picks `programName` in the mentor program picker in `scope`, and waits for its row. */
export async function pickMentorProgram(page: Page, scope: Page | Locator, programName: string): Promise<void> {
  const option = await findMentorProgramOption(page, scope, programName);
  await option.click();
  await expect(mentorProgramRow(scope, programName)).toBeVisible();
}

/** A request or invitation row in the picker's table. */
export function mentorProgramRow(scope: Page | Locator, programName: string): Locator {
  return mentorProgramsSection(scope).getByRole('row').filter({ hasText: programName });
}
