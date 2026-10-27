// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { expect, test } from '../support/fixtures';
import { PROGRAM_A } from '../support/test-data';

// Section 1 of docs/testing/p1-test-plan.md. Test titles start with the plan's case number and end
// with the workbook scenario IDs, so `--grep "1.7"` or `--grep ADM-04-01` runs one case.
test.describe('1. Admin: access, programs list, program page', () => {
  test('1.1 Open /mentorship with the flag on [ADM-01-01]', async ({ asUser }) => {
    const page = await asUser('adminA');
    await page.goto('/mentorship');

    await expect(page).toHaveURL(/\/mentorship\/admin$/);
    await expect(page.getByTestId('mentorship-admin-title')).toHaveText('Admin');
    await expect(page.getByTestId('mentorship-admin-enroll-button')).toBeVisible();
  });

  test('1.2 Programs list shows Program A [ADM-02-01]', async ({ asUser }) => {
    const page = await asUser('adminA');
    await page.goto('/mentorship/admin');

    const card = page.getByTestId(`mentorship-program-card-${PROGRAM_A.id}`);
    await expect(card).toBeVisible();
    await expect(card).toContainText(PROGRAM_A.name);
    await expect(card).toContainText(PROGRAM_A.project);
    await expect(card.getByTestId('mentorship-program-card-status')).toContainText('Open');
    await expect(card.getByTestId('mentorship-program-card-metrics')).toContainText(/Mentors/);
  });

  test('1.7 Program page [ADM-04-01]', async ({ asUser }) => {
    const page = await asUser('adminA');
    await page.goto(`/mentorship/admin/${PROGRAM_A.id}`);

    await expect(page.getByTestId('mentorship-program-detail-title')).toHaveText(PROGRAM_A.name);
    await expect(page.getByTestId('mentorship-program-detail-header')).toContainText(PROGRAM_A.project);
    await expect(page.getByTestId('mentorship-program-detail-status')).toContainText('Open');
    await expect(page.getByTestId('mentorship-program-detail-view-public')).toBeVisible();
    await expect(page.getByTestId('mentorship-program-detail-edit')).toBeVisible();

    await expect(page.getByTestId('mentorship-program-detail-tab-current-mentees')).toHaveText(/Current Mentees\s*\d+/);
    await expect(page.getByTestId('mentorship-program-detail-tab-past-mentees')).toHaveText(/Past Mentees\s*\d+/);
    await expect(page.getByTestId('mentorship-program-detail-tab-mentors')).toHaveText(/Mentors\s*\d+/);
    await expect(page.getByTestId('mentorship-program-detail-tab-terms')).toHaveText(/Terms\s*\d+/);
  });
});
