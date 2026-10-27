// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { expect, Locator, Page, Request } from '@playwright/test';
import { gotoHydrated } from './page';
import { PROGRAM_A } from './test-data';

// A 1×1 PNG, well under the 2 MB logo limit.
export const SMALL_PNG = {
  name: 'e2e-logo.png',
  mimeType: 'image/png',
  buffer: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==', 'base64'),
};

export const STANDARD_PREREQUISITES = ['Resume', 'Cover Letter', 'School Enrollment Verification', 'Participation permission from school or employer', 'Coding Challenge'];

export interface EnrollTerm {
  name: string;
  startMonth: string;
  startYear: string;
  endMonth: string;
  endYear: string;
  applicationStartDate: string;
  applicationEndDate: string;
}

const MONTHS = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'];

// Dates are UTC, like the backend's, so a run just after midnight in UTC+2 or UTC+3 does not pick tomorrow. The browser runs in
// UTC too (`timezoneId` in playwright.config.ts), so its "today" matches.
function isoDate(date: Date): string {
  return date.toISOString().slice(0, 10);
}

/**
 * A term that accepts applications from today: it starts `monthsAhead` months from now and runs four months. The wizard needs
 * applications to open before the start month and to close by the end month.
 */
export function openTerm(name: string, monthsAhead: number): EnrollTerm {
  const today = new Date();
  const start = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth() + monthsAhead, 1));
  const end = new Date(Date.UTC(start.getUTCFullYear(), start.getUTCMonth() + 3, 1));
  const applicationEnd = new Date(Date.UTC(start.getUTCFullYear(), start.getUTCMonth() + 1, 0));
  return {
    name,
    startMonth: MONTHS[start.getUTCMonth()],
    startYear: String(start.getUTCFullYear()),
    endMonth: MONTHS[end.getUTCMonth()],
    endYear: String(end.getUTCFullYear()),
    applicationStartDate: isoDate(today),
    applicationEndDate: isoDate(applicationEnd),
  };
}

/** `E2E P1 <purpose> <yyyy-mm-dd hhmmss>` in UTC, unique per run so a rerun never hits "name taken". */
export function e2eProgramName(prefix: string): string {
  const now = new Date().toISOString();
  return `${prefix} ${now.slice(0, 10)} ${now.slice(11, 19).replaceAll(':', '')}`;
}

const SELECT_OVERLAY = '.p-select-overlay';

/** A select: its `lfx-select` locator, or the `data-test` attribute the wrapper puts on it. */
export type SelectRef = string | Locator;

function selectLocator(page: Page, select: SelectRef): Locator {
  return typeof select === 'string' ? page.locator(`[data-test="${select}"]`) : select;
}

/**
 * Opens a PrimeNG select, or returns its overlay if it is already open. Clicks only once no other overlay is left: a click while
 * one is still animating closed can land on whatever is behind it, such as the wizard's Next.
 */
export async function openSelect(page: Page, ref: SelectRef): Promise<Locator> {
  const select = selectLocator(page, ref);
  const combobox = select.getByRole('combobox').first();
  const overlay = page.locator(SELECT_OVERLAY);
  await expect(async () => {
    if ((await combobox.getAttribute('aria-expanded')) !== 'true') {
      await expect(overlay).toHaveCount(0, { timeout: 3_000 });
      await select.click();
    }
    await expect(combobox).toHaveAttribute('aria-expanded', 'true', { timeout: 2_000 });
  }).toPass({ timeout: 15_000 });
  return overlay;
}

/** Picks the option with that exact label from a PrimeNG select, typing `filter` into its search box first. */
export async function pickOption(page: Page, ref: SelectRef, label: string, filter?: string): Promise<void> {
  const overlay = await openSelect(page, ref);
  if (filter) {
    await overlay.locator('input.p-select-filter').fill(filter);
  }
  await overlay.getByRole('option', { name: label, exact: true }).click();
  const select = selectLocator(page, ref);
  await expect(select).toContainText(label);
  // The overlay closes by itself, but the import picker can open again while the import fills the form. Only then close it by
  // toggling the select: Escape would also close a dialog the select sits in.
  try {
    await expect(page.locator(SELECT_OVERLAY)).toHaveCount(0, { timeout: 5_000 });
  } catch {
    await select.click();
    await expect(page.locator(SELECT_OVERLAY)).toHaveCount(0);
  }
}

export async function openEnrollWizard(page: Page): Promise<void> {
  await gotoHydrated(page, '/mentorship/admin/enroll');
  await expect(page.getByTestId('mentorship-enroll-details')).toBeVisible();
}

export function nextButton(page: Page): Locator {
  return page.getByTestId('mentorship-enroll-next');
}

/** Types the name and waits for the availability check, so Next is not refused while it is still running. */
export async function fillProgramName(page: Page, name: string): Promise<void> {
  const nameInput = page.locator('[data-test="mentorship-enroll-name"]');
  const lookup = page.waitForResponse((response) => response.url().includes('/api/mentorship/programs/name-available'));
  await nameInput.fill(name);
  expect((await lookup).ok(), 'name-available lookup').toBe(true);
}

/**
 * Searches the LF project picker and picks `projectName`. The picker keeps reading pages after a search and its virtual list
 * redraws the options as each lands, so a click can hit an option that is replaced mid-click, and a missed click can close the
 * picker. Each attempt reopens it, searches, waits for the reads to finish, then clicks.
 */
export async function pickLfProject(page: Page, projectName: string): Promise<void> {
  const select = page.locator('[data-test="mentorship-enroll-project"]');
  const isProjectsRead = (url: string) => url.includes('/api/mentorship/lf-projects');
  let inFlight = 0;
  const started = (request: Request) => isProjectsRead(request.url()) && inFlight++;
  const settled = (request: Request) => isProjectsRead(request.url()) && inFlight--;
  page.on('request', started);
  page.on('requestfinished', settled);
  page.on('requestfailed', settled);
  try {
    await expect(async () => {
      const overlay = await openSelect(page, 'mentorship-enroll-project');
      const filter = overlay.locator('input.p-select-filter');
      if ((await filter.inputValue()) !== projectName) await filter.fill(projectName);
      const option = overlay.getByRole('option', { name: projectName, exact: true });
      await expect(option).toBeVisible({ timeout: 5_000 });
      await expect.poll(() => inFlight, { timeout: 5_000 }).toBeLessThanOrEqual(0);
      await option.click({ timeout: 2_000 });
      await expect(select).toContainText(projectName, { timeout: 2_000 });
    }).toPass({ timeout: 45_000 });
  } finally {
    page.off('request', started);
    page.off('requestfinished', settled);
    page.off('requestfailed', settled);
  }
}

export async function addTechnology(page: Page, technology: string): Promise<void> {
  await page.locator('#mentorship-enroll-technology-input').fill(technology);
  await page.keyboard.press('Escape');
  await page.getByTestId('mentorship-enroll-add-technology').click();
}

export async function fillDescription(page: Page, text: string): Promise<void> {
  const editor = page.getByTestId('mentorship-enroll-description').locator('[contenteditable="true"]');
  await editor.click();
  await editor.fill(text);
}

/** Fills Step 1 with valid answers. */
export async function fillDetailsStep(page: Page, name: string, options: { ciiId?: string } = {}): Promise<void> {
  await fillProgramName(page, name);
  await pickLfProject(page, PROGRAM_A.project);
  await addTechnology(page, 'Go');
  await fillDescription(page, `${name}: a program created by the P1 Playwright run. Do not apply.`);
  await page.locator('[data-test="mentorship-enroll-repo"]').fill('https://github.com/linuxfoundation/lfx-mentorship');
  if (options.ciiId) {
    await page.locator('[data-test="mentorship-enroll-cii"]').fill(options.ciiId);
    await expect(page.getByTestId('mentorship-enroll-cii-badge')).toBeVisible();
  }
  await page.getByTestId('mentorship-enroll-details').locator('input[type="file"]').setInputFiles(SMALL_PNG);
}

/** Adds a skill with a skills picker; `idPrefix` is the picker's, `mentorship-enroll` on the wizard. */
export async function addSkill(page: Page, skill: string, idPrefix = 'mentorship-enroll'): Promise<void> {
  await page.locator(`#${idPrefix}-skill-input`).fill(skill);
  await page.keyboard.press('Escape');
  await page.getByTestId(`${idPrefix}-add-skill`).click();
  await expect(page.getByTestId(`${idPrefix}-skill-list`)).toContainText(skill);
}

/**
 * Types a YYYY-MM-DD date into a PrimeNG date picker and leaves it with Tab, which closes its overlay: Escape would also close
 * an enclosing dialog, and the overlay covers the fields below, so a click elsewhere may land on it. Tab may open the next
 * date field's picker, so only the value is checked.
 */
async function fillDate(input: Locator, value: string): Promise<void> {
  // PrimeNG's date picker ignores input that has no keydown, so a bare `fill` would show the text but never set the form value.
  // Typing it key by key does not work either where the form writes each parsed date back into the field (the custom
  // prerequisite's due date), so a key press satisfies the check and the whole value then goes in as one input.
  await input.click();
  await input.press('End');
  await input.fill(value);
  await input.press('Tab');
  await expect(input).toHaveValue(value);
}

/** The Add Term / Edit Term dialog. */
export function termDialog(page: Page): Locator {
  return page.getByRole('dialog', { name: /^(Add|Edit) Term$/ });
}

export async function fillTermDialog(page: Page, term: EnrollTerm): Promise<void> {
  const dialog = termDialog(page);
  await expect(dialog).toBeVisible();
  await dialog.locator('#dialog-term-name').fill(term.name);
  await pickOption(page, 'mentorship-enroll-term-start-month', term.startMonth);
  await pickOption(page, 'mentorship-enroll-term-start-year', term.startYear);
  await pickOption(page, 'mentorship-enroll-term-end-month', term.endMonth);
  await pickOption(page, 'mentorship-enroll-term-end-year', term.endYear);
  for (const [id, value] of [
    ['#dialog-term-application-start', term.applicationStartDate],
    ['#dialog-term-application-end', term.applicationEndDate],
  ] as const) {
    await fillDate(dialog.locator(id), value);
  }
  await dialog.getByRole('button', { name: /^(Add|Save) Term$/ }).click();
  await expect(dialog).toBeHidden();
}

export async function addTerm(page: Page, term: EnrollTerm): Promise<void> {
  await page.getByTestId('mentorship-enroll-setup').getByRole('button', { name: 'Add Term' }).click();
  await fillTermDialog(page, term);
  await expect(termRow(page, term.name)).toBeVisible();
}

export function termRow(page: Page, termName: string): Locator {
  return page.getByTestId('mentorship-enroll-setup').getByRole('row').filter({ hasText: termName });
}

/** Fills Step 2: two skills and two open terms. */
export async function fillSetupStep(page: Page, name: string): Promise<void> {
  await addSkill(page, 'Go');
  await addSkill(page, 'Kubernetes');
  await addTerm(page, openTerm(`E2E ${name.slice(-17)} T1`, 1));
  await addTerm(page, openTerm(`E2E ${name.slice(-17)} T2`, 2));
}

export function prerequisiteToggle(page: Page, prerequisite: string): Locator {
  return page.getByRole('button', { name: `Toggle required for ${prerequisite}`, exact: true });
}

export async function addCustomPrerequisite(page: Page, index: number, name: string, dueDate: string): Promise<void> {
  await page.getByTestId('mentorship-enroll-add-prereq').click();
  const card = page.getByTestId(`mentorship-enroll-custom-prereq-${index}`);
  await card.locator(`[data-test="mentorship-enroll-custom-prereq-name-${index}"]`).fill(name);
  await fillDate(card.locator(`#custom-prereq-due-date-${index}`), dueDate);
  await card.locator(`#custom-prereq-description-${index}`).fill(`${name}: upload a short intro video.`);
  await card.locator(`#custom-prereq-require-file-${index}`).check();
}

/** Fills Step 3: every standard prerequisite required, one custom one, and the T&C ticked. */
export async function fillPrerequisitesStep(page: Page): Promise<void> {
  for (const prerequisite of STANDARD_PREREQUISITES) {
    await prerequisiteToggle(page, prerequisite).click();
    await expect(prerequisiteToggle(page, prerequisite)).toHaveAttribute('aria-pressed', 'true');
  }
  await page.locator('#challenge-url-prereq-coding').fill('https://github.com/linuxfoundation/lfx-mentorship');
  const due = new Date();
  due.setUTCDate(due.getUTCDate() + 30);
  await addCustomPrerequisite(page, 1, 'E2E Intro Video', isoDate(due));
  await page.locator('#mentorship-enroll-terms').check();
}

/**
 * Clicks Next until the wizard shows `expectedStep`. Step 1 refuses Next with a "Checking…" toast while its name and CII
 * lookups run; a Next it accepts changes the step at once, so a retry only happens on the old step.
 */
export async function goNext(page: Page, expectedStep: 'setup' | 'prerequisites'): Promise<void> {
  const target = page.getByTestId(`mentorship-enroll-${expectedStep}`);
  await expect(async () => {
    if (!(await target.isVisible())) await nextButton(page).click();
    await expect(target).toBeVisible({ timeout: 3_000 });
  }).toPass({ timeout: 20_000 });
}

/** Runs the whole wizard and submits. Returns the new program's id. */
export async function enrollProgram(page: Page, name: string): Promise<string> {
  await openEnrollWizard(page);
  await fillDetailsStep(page, name);
  await goNext(page, 'setup');
  await fillSetupStep(page, name);
  await goNext(page, 'prerequisites');
  await fillPrerequisitesStep(page);

  const created = page.waitForResponse((response) => response.request().method() === 'POST' && /\/api\/mentorship\/admin\/programs$/.test(new URL(response.url()).pathname));
  const logo = page.waitForResponse((response) => response.request().method() === 'POST' && /\/api\/mentorship\/admin\/programs\/[^/]+\/logo/.test(response.url()));
  await nextButton(page).click();

  const createResponse = await created;
  expect(createResponse.status(), 'create program').toBeLessThan(300);
  const { id } = (await createResponse.json()) as { id: string };
  expect((await logo).status(), 'logo upload').toBeLessThan(300);

  await expect(page.getByText('Program submitted for review.')).toBeVisible();
  await expect(page).toHaveURL(/\/mentorship\/admin$/);
  return id;
}
