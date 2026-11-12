// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { expect, type Page, type Request } from '@playwright/test';
import { LOGO_PNG } from './files';
import { gotoHydrated } from './fixtures';
import { addSkill, fillDate, fillRichText, isoDate, openSelect, pickOption } from './forms';

// The Self Serve enroll wizard (/mentorship/admin/enroll).

// A public LF project on DEV that the journey programs belong to.
const LF_PROJECT = 'Academy Software Foundation (ASWF)';
const SKILL = 'Go';
const MONTHS = [
  'January',
  'February',
  'March',
  'April',
  'May',
  'June',
  'July',
  'August',
  'September',
  'October',
  'November',
  'December',
];

const STANDARD_PREREQUISITES = [
  'Resume',
  'Cover Letter',
  'School Enrollment Verification',
  'Participation permission from school or employer',
  'Coding Challenge',
];

export interface EnrollTerm {
  name: string;
  startMonth: string;
  startYear: string;
  endMonth: string;
  endYear: string;
  applicationStartDate: string;
  applicationEndDate: string;
}

/**
 * A term that accepts applications from today: it starts next month and runs four months. The
 * wizard needs applications to open before the start month and close by the end month.
 */
export function openTerm(name: string): EnrollTerm {
  const today = new Date();
  const start = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth() + 1, 1));
  const end = new Date(Date.UTC(start.getUTCFullYear(), start.getUTCMonth() + 3, 1));
  const applicationEnd = new Date(Date.UTC(start.getUTCFullYear(), start.getUTCMonth() + 1, 0));
  return {
    name,
    startMonth: MONTHS[start.getUTCMonth()]!,
    startYear: String(start.getUTCFullYear()),
    endMonth: MONTHS[end.getUTCMonth()]!,
    endYear: String(end.getUTCFullYear()),
    applicationStartDate: isoDate(today),
    applicationEndDate: isoDate(applicationEnd),
  };
}

/** Types the name and waits for the availability check, so Next is not refused mid-check. */
async function fillProgramName(page: Page, name: string): Promise<void> {
  const lookup = page.waitForResponse((response) =>
    response.url().includes('/api/mentorship/programs/name-available'),
  );
  await page.locator('[data-test="mentorship-enroll-name"]').fill(name);
  expect((await lookup).ok(), 'name-available lookup').toBe(true);
}

/**
 * Searches the LF project picker and picks the project. The picker keeps reading pages after a
 * search and redraws its options as each lands, so a click can hit a replaced option. Each attempt
 * reopens it, searches, waits for the reads to settle, then clicks.
 */
async function pickLfProject(page: Page): Promise<void> {
  const select = page.locator('[data-test="mentorship-enroll-project"]');
  const isProjectsRead = (request: Request) =>
    request.url().includes('/api/mentorship/lf-projects');
  let inFlight = 0;
  const started = (request: Request) => isProjectsRead(request) && inFlight++;
  const settled = (request: Request) => isProjectsRead(request) && inFlight--;
  page.on('request', started);
  page.on('requestfinished', settled);
  page.on('requestfailed', settled);
  try {
    await expect(async () => {
      const overlay = await openSelect(page, select);
      const filter = overlay.locator('input.p-select-filter');
      if ((await filter.inputValue()) !== LF_PROJECT) await filter.fill(LF_PROJECT);
      const option = overlay.getByRole('option', { name: LF_PROJECT, exact: true });
      await expect(option).toBeVisible({ timeout: 5_000 });
      await expect.poll(() => inFlight, { timeout: 5_000 }).toBeLessThanOrEqual(0);
      await option.click({ timeout: 2_000 });
      await expect(select).toContainText(LF_PROJECT, { timeout: 2_000 });
    }).toPass({ timeout: 45_000 });
  } finally {
    page.off('request', started);
    page.off('requestfinished', settled);
    page.off('requestfailed', settled);
  }
}

async function fillDetailsStep(page: Page, name: string): Promise<void> {
  await fillProgramName(page, name);
  await pickLfProject(page);
  await page.locator('#mentorship-enroll-technology-input').fill(SKILL);
  await page.keyboard.press('Escape');
  await page.getByTestId('mentorship-enroll-add-technology').click();
  await fillRichText(
    page.getByTestId('mentorship-enroll-description'),
    `${name}: created by the E2E journey suite. Do not apply.`,
  );
  await page
    .locator('[data-test="mentorship-enroll-repo"]')
    .fill('https://github.com/linuxfoundation/lfx-mentorship');
  await page
    .getByTestId('mentorship-enroll-details')
    .locator('input[type="file"]')
    .setInputFiles(LOGO_PNG);
}

async function addTerm(page: Page, term: EnrollTerm): Promise<void> {
  await page
    .getByTestId('mentorship-enroll-setup')
    .getByRole('button', { name: 'Add Term' })
    .click();
  const dialog = page.getByRole('dialog', { name: 'Add Term' });
  await expect(dialog).toBeVisible();
  await dialog.locator('#dialog-term-name').fill(term.name);
  await pickOption(page, 'mentorship-enroll-term-start-month', term.startMonth);
  await pickOption(page, 'mentorship-enroll-term-start-year', term.startYear);
  await pickOption(page, 'mentorship-enroll-term-end-month', term.endMonth);
  await pickOption(page, 'mentorship-enroll-term-end-year', term.endYear);
  await fillDate(dialog.locator('#dialog-term-application-start'), term.applicationStartDate);
  await fillDate(dialog.locator('#dialog-term-application-end'), term.applicationEndDate);
  await dialog.getByRole('button', { name: 'Add Term' }).click();
  await expect(dialog).toBeHidden();
  await expect(
    page.getByTestId('mentorship-enroll-setup').getByRole('row').filter({ hasText: term.name }),
  ).toBeVisible();
}

/** Every standard prerequisite required, and the terms accepted. */
async function fillPrerequisitesStep(page: Page): Promise<void> {
  for (const prerequisite of STANDARD_PREREQUISITES) {
    const toggle = page.getByRole('button', {
      name: `Toggle required for ${prerequisite}`,
      exact: true,
    });
    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-pressed', 'true');
  }
  await page
    .locator('#challenge-url-prereq-coding')
    .fill('https://github.com/linuxfoundation/lfx-mentorship');
  await page.locator('#mentorship-enroll-terms').check();
}

/**
 * Clicks Next until the wizard shows `step`. Step 1 refuses Next with a "Checking…" toast while
 * its lookups run; an accepted Next changes the step at once, so a retry only happens on the old step.
 */
async function goNext(page: Page, step: 'setup' | 'prerequisites'): Promise<void> {
  const target = page.getByTestId(`mentorship-enroll-${step}`);
  await expect(async () => {
    if (!(await target.isVisible())) await page.getByTestId('mentorship-enroll-next').click();
    await expect(target).toBeVisible({ timeout: 3_000 });
  }).toPass({ timeout: 20_000 });
}

/** Runs the whole wizard with one open term and submits. Returns the new program's ID. */
export async function enrollProgram(page: Page, name: string, term: EnrollTerm): Promise<string> {
  await gotoHydrated(page, '/mentorship/admin/enroll');
  await expect(page.getByTestId('mentorship-enroll-details')).toBeVisible();
  await fillDetailsStep(page, name);
  await goNext(page, 'setup');
  await addSkill(page, SKILL, 'mentorship-enroll');
  await addTerm(page, term);
  await goNext(page, 'prerequisites');
  await fillPrerequisitesStep(page);

  const isPost = (pattern: RegExp) => (response: { request(): Request; url(): string }) =>
    response.request().method() === 'POST' && pattern.test(new URL(response.url()).pathname);
  const created = page.waitForResponse(isPost(/\/api\/mentorship\/admin\/programs$/));
  const logo = page.waitForResponse(isPost(/\/api\/mentorship\/admin\/programs\/[^/]+\/logo/));
  await page.getByTestId('mentorship-enroll-next').click();

  const createResponse = await created;
  expect(createResponse.status(), 'create program').toBeLessThan(300);
  const { id } = (await createResponse.json()) as { id: string };
  expect((await logo).status(), 'logo upload').toBeLessThan(300);
  await expect(page.getByText('Program submitted for review.')).toBeVisible();
  return id;
}
