// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { expect, type Page } from '@playwright/test';
import { taskPdf } from './files';
import { gotoHydrated } from './fixtures';
import { addSkill } from './forms';

// A mentee's pages in Self Serve (/mentorship/mentee/...).

interface MenteeTask {
  id: string;
  name: string;
  status: string;
}

export interface MenteeApplication {
  id: string;
  programId: string;
  term: { id: string; name: string };
  tasks: MenteeTask[];
}

function applyPath(programId: string, termId: string): string {
  return `/mentorship/mentee/apply?programId=${programId}&programTermId=${termId}`;
}

/**
 * The mentee's application to `programId`. Access to a new application reaches the permission
 * store a few seconds after it is created, so the read is retried until it shows.
 */
export async function menteeApplication(page: Page, programId: string): Promise<MenteeApplication> {
  let application: MenteeApplication | undefined;
  await expect(async () => {
    const response = await page.request.get('/api/mentorship/mentee/applications?withTasks=true');
    expect(response.status(), 'GET mentee/applications').toBe(200);
    const { data } = (await response.json()) as { data: MenteeApplication[] };
    application = data.find((candidate) => candidate.programId === programId);
    expect(application, `application to ${programId}`).toBeDefined();
  }).toPass({ timeout: 90_000 });
  return application!;
}

/**
 * Opens an Apply link as a user with no mentee profile: Self Serve sends them to Become a Mentee
 * first, and back to the application once the profile is saved.
 */
export async function registerFromApplyLink(
  page: Page,
  programId: string,
  termId: string,
): Promise<void> {
  await gotoHydrated(page, applyPath(programId, termId));
  await expect(page.getByTestId('mentorship-mentee-register-title')).toBeVisible();
  const editor = page.locator('.lfx-rich-editor__host [contenteditable="true"]');
  await editor.click();
  await editor.fill('E2E journey mentee: registered by the Playwright suite.');
  await addSkill(page, 'Go', 'mentorship-mentee-have');
  await addSkill(page, 'Kubernetes', 'mentorship-mentee-want');
  await page.getByTestId('mentorship-mentee-country-select').locator('.p-select').first().click();
  await page.locator('.p-select-overlay .p-select-filter').fill('Canada');
  await page.getByRole('option', { name: 'Canada', exact: true }).click();
  for (const checkbox of [
    'age-eligible',
    'work-authorized',
    'no-duplicate-profile',
    'compliance',
    'terms',
  ]) {
    await page.locator(`#mentorship-mentee-${checkbox}`).check();
  }
  await page.getByTestId('mentorship-mentee-submit').getByRole('button').click();
  await expect(page.getByText('Profile created')).toBeVisible();
  await expect(page).toHaveURL(/\/mentorship\/mentee\/apply\?/);
}

/** Submits the application on the apply page. */
export async function apply(
  page: Page,
  programId: string,
  termId: string,
  programName: string,
): Promise<void> {
  if (!new URL(page.url()).pathname.endsWith('/mentorship/mentee/apply')) {
    await gotoHydrated(page, applyPath(programId, termId));
  }
  await expect(page.getByTestId('mentorship-mentee-apply-title')).toContainText(programName);
  const submit = page.getByTestId('mentorship-mentee-apply-submit').getByRole('button');
  await expect(submit).toBeVisible();
  // A mentee who registered on the way here already made these confirmations, so the page
  // leaves out its Before You Apply section.
  const confirmations = page.getByTestId('mentorship-mentee-apply-before');
  if (await confirmations.isVisible()) {
    for (const checkbox of [
      'apply-age-eligible',
      'apply-work-authorized',
      'apply-no-duplicate-profile',
      'apply-compliance',
      'terms',
    ]) {
      await confirmations.locator(`#mentorship-mentee-${checkbox}`).check();
    }
  }
  await expect(submit).toBeEnabled();
  await submit.click();
  await expect(page.getByText('Application submitted')).toBeVisible();
}

/** Opens the mentee's tasks page on `taskId`'s row; a new task can take a moment to show. */
async function openTaskRow(page: Page, taskId: string): Promise<void> {
  const status = page.locator(`[data-test="mentee-tasks-status-dropdown-${taskId}"]`);
  await expect(async () => {
    await gotoHydrated(page, '/mentorship/mentee/tasks');
    await expect(status).toBeVisible({ timeout: 10_000 });
  }).toPass({ timeout: 90_000 });
}

/** Uploads a PDF to a task, which also marks it submitted. */
export async function uploadTaskFile(page: Page, taskId: string): Promise<void> {
  await openTaskRow(page, taskId);
  await page
    .getByTestId(`mentee-tasks-file-input-${taskId}`)
    .setInputFiles(taskPdf(`e2e-${taskId}.pdf`));
  await expect(page.getByText('File uploaded').first()).toBeVisible();
  await expect(page.getByTestId(`mentee-tasks-download-file-${taskId}`)).toBeVisible();
}

/** Moves a task to a new status with its status dropdown. */
export async function setTaskStatus(
  page: Page,
  taskId: string,
  label: 'In Progress' | 'Submitted',
): Promise<void> {
  await openTaskRow(page, taskId);
  const status = page.locator(`[data-test="mentee-tasks-status-dropdown-${taskId}"]`);
  await status.click();
  await page.getByRole('option', { name: label, exact: true }).click();
  await expect(status).toContainText(label);
}
