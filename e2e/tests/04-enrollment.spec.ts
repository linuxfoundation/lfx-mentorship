// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { findAdminPrograms } from '../support/api';
import {
  addSkill,
  addTerm,
  e2eProgramName,
  enrollProgram,
  fillDetailsStep,
  fillPrerequisitesStep,
  fillProgramName,
  fillSetupStep,
  fillTermDialog,
  goNext,
  nextButton,
  openEnrollWizard,
  openSelect,
  openTerm,
  pickOption,
  prerequisiteToggle,
  SMALL_PNG,
  STANDARD_PREREQUISITES,
  termRow,
} from '../support/enroll';
import { expect, test } from '../support/fixtures';
import { gotoHydrated } from '../support/page';
import { PROGRAM_A, REVIEW_PROGRAMS } from '../support/test-data';

// Section 4 of docs/testing/p1-test-plan.md. adminA (sameh100) enrolls; 4.7 leaves one pending program per
// REVIEW_PROGRAMS entry for section 5 to review.
test.describe('4. Enrollment and edit', () => {
  // Each case walks the wizard against DEV, which takes longer than the 30 s default.
  test.describe.configure({ timeout: 2 * 60_000 });

  test('4.1 Step 1 [ADM-19-01]', async ({ asUser }) => {
    const page = await asUser('adminA');
    await openEnrollWizard(page);
    await expect(page.getByTestId('mentorship-enroll-title')).toHaveText('Enroll a Program');

    await fillDetailsStep(page, e2eProgramName('E2E P1 Step1'));
    await goNext(page, 'setup');
  });

  test('4.2 CII ID shows the badge [ADM-19-09]', async ({ asUser }) => {
    const page = await asUser('adminA');
    await openEnrollWizard(page);

    await page.locator('[data-test="mentorship-enroll-cii"]').fill('1');
    const badge = page.getByTestId('mentorship-enroll-cii-badge');
    await expect(badge).toBeVisible();
    await expect(badge.locator('img')).toHaveAttribute('alt', /project 1$/);
  });

  test('4.3 LF project picker: search, scroll, Retry [ADM-19-10]', async ({ asUser }) => {
    const page = await asUser('adminA');
    await openEnrollWizard(page);
    const isProjectsRead = (url: string) => url.includes('/api/mentorship/lf-projects');
    const requests: string[] = [];
    page.on('request', (request) => isProjectsRead(request.url()) && requests.push(request.url()));

    const overlay = await openSelect(page, 'mentorship-enroll-project');
    await expect(overlay.getByRole('option').first()).toBeVisible();

    // Scrolling to the end of what is loaded reads the next page. The picker may still be following its first cursor, which
    // blocks a scroll read, so scroll until one is sent.
    const pageReads = () => requests.filter((url) => url.includes('page_token=')).length;
    const before = pageReads();
    await expect(async () => {
      await overlay.locator('.p-virtualscroller').evaluate((element) => element.scrollTo(0, element.scrollHeight));
      expect(pageReads()).toBeGreaterThan(before);
    }).toPass({ timeout: 15_000 });

    const filter = overlay.locator('input.p-select-filter');
    await filter.fill('Academy Software');
    await expect(overlay.getByRole('option', { name: PROGRAM_A.project, exact: true })).toBeVisible();

    // A failed read shows Try again, which recovers once the BFF answers again.
    // unroute only removes a route given the same matcher, so keep it in one place.
    const projectsReadUrl = (url: URL) => isProjectsRead(url.href);
    await page.route(projectsReadUrl, (route) => route.abort());
    await filter.fill('Cloud Native');
    await expect(page.getByTestId('mentorship-enroll-project-failed')).toBeVisible();
    await page.unroute(projectsReadUrl);

    const retried = page.waitForResponse((response) => isProjectsRead(response.url()) && response.ok());
    await page.getByTestId('mentorship-enroll-project-retry').click();
    await retried;
    await expect(page.getByTestId('mentorship-enroll-project-failed')).toBeHidden();
    await openSelect(page, 'mentorship-enroll-project');
    await expect(overlay.getByRole('option').first()).toBeVisible();
  });

  test('4.4 Import from an existing program [ADM-19-12]', async ({ asUser }) => {
    const page = await asUser('adminA');
    await openEnrollWizard(page);

    const template = page.waitForResponse((response) => response.url().includes(PROGRAM_A.id) && response.ok());
    await pickOption(page, 'mentorship-enroll-import', PROGRAM_A.name, PROGRAM_A.name);
    await template;

    // Details and technologies are copied; the logo is not.
    const details = page.getByTestId('mentorship-enroll-details');
    await expect(page.locator('[data-test="mentorship-enroll-name"]')).toHaveValue(PROGRAM_A.name);
    await expect(page.locator('[data-test="mentorship-enroll-project"]')).toContainText(PROGRAM_A.project);
    await expect(page.locator('[data-test="mentorship-enroll-repo"]')).not.toHaveValue('');
    await expect(page.locator('[data-test="mentorship-enroll-cii"]')).toHaveValue('1');
    await expect(page.getByTestId('mentorship-enroll-description').locator('[contenteditable="true"]')).not.toBeEmpty();
    await expect(details.getByText('Choose file')).toBeVisible();

    // The name is taken by Program A, so give it a new one and a logo to move on.
    await fillProgramName(page, e2eProgramName('E2E P1 Import'));
    await details.locator('input[type="file"]').setInputFiles(SMALL_PNG);
    await goNext(page, 'setup');

    // Skills are copied; terms are not.
    const skills = page.getByTestId('mentorship-enroll-skill-list');
    await expect(skills).toContainText(/\bgo\b/i);
    await expect(skills).toContainText(/kubernetes/i);
    await expect(page.getByTestId('mentorship-enroll-setup').locator('tbody tr')).toHaveCount(0);

    await addTerm(page, openTerm('E2E Import T1', 1));
    await goNext(page, 'prerequisites');

    // Prerequisites are copied; the T&C acknowledgement is not.
    for (const prerequisite of STANDARD_PREREQUISITES) {
      await expect(prerequisiteToggle(page, prerequisite)).toHaveAttribute('aria-pressed', 'true');
    }
    await expect(page.locator('[data-test="mentorship-enroll-custom-prereq-name-1"]')).toHaveValue('E2E Intro Video');
    await expect(page.locator('#mentorship-enroll-terms')).not.toBeChecked();
  });

  test('4.5 Step 2: skills and 1–4 terms [ADM-20-01]', async ({ asUser }) => {
    const page = await asUser('adminA');
    const name = e2eProgramName('E2E P1 Step2');
    await openEnrollWizard(page);
    await fillDetailsStep(page, name);
    await goNext(page, 'setup');

    // Nothing filled: Next stays on Setup.
    await nextButton(page).click();
    await expect(page.getByTestId('mentorship-enroll-skill-error')).toHaveText('Add at least one skill.');
    await expect(page.getByTestId('mentorship-enroll-setup')).toBeVisible();

    await addSkill(page, 'Go');

    // Add, then edit.
    await addTerm(page, openTerm('E2E Step2 T1', 1));
    await termRow(page, 'E2E Step2 T1').getByRole('button', { name: 'Edit' }).click();
    await fillTermDialog(page, openTerm('E2E Step2 T1 edited', 1));
    await expect(termRow(page, 'E2E Step2 T1 edited')).toBeVisible();

    // Up to 4 terms; Add Term is disabled at the limit.
    for (const index of [2, 3, 4]) {
      await addTerm(page, openTerm(`E2E Step2 T${index}`, index));
    }
    await expect(page.getByTestId('mentorship-enroll-setup').getByRole('button', { name: 'Add Term' })).toBeDisabled();

    // Delete asks first.
    await termRow(page, 'E2E Step2 T4').getByRole('button', { name: 'Delete' }).click();
    const confirm = page.getByRole('alertdialog', { name: 'Delete Term' });
    await expect(confirm).toContainText('Are you sure you want to delete this term?');
    await confirm.getByRole('button', { name: 'Delete Term' }).click();
    await expect(termRow(page, 'E2E Step2 T4')).toHaveCount(0);
    await expect(page.getByTestId('mentorship-enroll-setup').getByRole('button', { name: 'Add Term' })).toBeEnabled();

    await goNext(page, 'prerequisites');
  });

  test('4.6 Step 3 prerequisites [ADM-20-03]', async ({ asUser }) => {
    const page = await asUser('adminA');
    const name = e2eProgramName('E2E P1 Step3');
    await openEnrollWizard(page);
    await fillDetailsStep(page, name);
    await goNext(page, 'setup');
    await fillSetupStep(page, name);
    await goNext(page, 'prerequisites');

    await fillPrerequisitesStep(page);

    // Back to Setup and forward again: every answer is still there.
    await page.getByTestId('mentorship-enroll-back').click();
    await expect(page.getByTestId('mentorship-enroll-setup')).toBeVisible();
    await goNext(page, 'prerequisites');
    for (const prerequisite of STANDARD_PREREQUISITES) {
      await expect(prerequisiteToggle(page, prerequisite)).toHaveAttribute('aria-pressed', 'true');
    }
    await expect(page.locator('[data-test="mentorship-enroll-custom-prereq-name-1"]')).toHaveValue('E2E Intro Video');
    await expect(page.locator('#custom-prereq-require-file-1')).toBeChecked();
    await expect(page.locator('#mentorship-enroll-terms')).toBeChecked();
  });

  // Creates the pending programs section 5 reviews. The review email is not checked here.
  test('4.7 Submit [ADM-21-01]', async ({ asUser }) => {
    test.setTimeout(8 * 60_000);
    const page = await asUser('adminA');

    for (const prefix of Object.values(REVIEW_PROGRAMS)) {
      const name = e2eProgramName(prefix);
      const id = await enrollProgram(page, name);

      await page.locator('[data-test="mentorship-programs-search"]').fill(name);
      const card = page.getByTestId(`mentorship-program-card-${id}`);
      await expect(card).toContainText(name);
      await expect(card.getByTestId('mentorship-program-card-status')).toContainText('Pending Review');

      const [listed] = await findAdminPrograms(page.request, name);
      expect(listed?.status).toBe('pending-review');
    }
  });

  test('4.8 Leave the wizard with no changes [ADM-22-01]', async ({ asUser }) => {
    const page = await asUser('adminA');
    await openEnrollWizard(page);

    await page.getByTestId('mentorship-enroll-back').click();
    await expect(page).toHaveURL(/\/mentorship\/admin$/);
    await expect(page.getByRole('alertdialog', { name: 'Cancel enrollment' })).toHaveCount(0);
  });

  test('4.9 Edit program [ADM-23-01]', async ({ asUser }) => {
    const page = await asUser('adminA');
    await gotoHydrated(page, `/mentorship/admin/${PROGRAM_A.id}`);
    await page.getByTestId('mentorship-program-detail-edit').click();

    await expect(page).toHaveURL(new RegExp(`/mentorship/admin/enroll\\?programId=${PROGRAM_A.id}`));
    await expect(page.getByTestId('mentorship-enroll-title')).toHaveText('Edit program');
    await expect(page.locator('[data-test="mentorship-enroll-name"]')).toHaveValue(PROGRAM_A.name);
    await expect(page.locator('[data-test="mentorship-enroll-project"]')).toContainText(PROGRAM_A.project);
    await expect(page.locator('[data-test="mentorship-enroll-import"]')).toHaveCount(0);

    // The logo is optional: Next goes on with the current one.
    await expect(page.getByText('Checking program name...')).toBeHidden();
    await goNext(page, 'setup');

    // Open terms can be edited; closed ones are read-only.
    for (const term of Object.values(PROGRAM_A.terms)) {
      await expect(termRow(page, term.name).getByRole('button', { name: 'Edit' })).toBeVisible();
    }
    const closed = page.getByTestId('mentorship-enroll-closed-term');
    for (const row of await closed.all()) {
      await expect(row).toContainText('Managed in the Terms tab');
      await expect(row.getByRole('button')).toHaveCount(0);
    }

    await goNext(page, 'prerequisites');
    await expect(page.locator('#mentorship-enroll-terms')).toHaveCount(0);
  });

  test('4.10 Update [ADM-23-02]', async ({ asUser }) => {
    const page = await asUser('adminA');
    await gotoHydrated(page, `/mentorship/admin/enroll?programId=${PROGRAM_A.id}`);
    await expect(page.locator('[data-test="mentorship-enroll-name"]')).toHaveValue(PROGRAM_A.name);

    await page.locator('[data-test="mentorship-enroll-website"]').fill(`https://example.org/e2e-${Date.now()}`);
    await expect(page.getByText('Checking program name...')).toBeHidden();
    await goNext(page, 'setup');
    await goNext(page, 'prerequisites');
    await expect(nextButton(page)).toHaveText('Update');
    await nextButton(page).click();

    await expect(page.getByText('Program updated.')).toBeVisible();
    await expect(page).toHaveURL(new RegExp(`/mentorship/admin/${PROGRAM_A.id}$`));
  });
});
