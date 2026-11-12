// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { expect, type Page } from '@playwright/test';
import { gotoHydrated } from './fixtures';

// A mentor's pages in Self Serve (/mentorship/mentor/...).

/** Accepts the invitation to mentor `programName` in the Edit Mentor Profile drawer. */
export async function acceptInvitation(
  page: Page,
  programId: string,
  programName: string,
): Promise<void> {
  let invitationId = '';
  await expect(async () => {
    const response = await page.request.get('/api/mentorship/mentor/requests');
    expect(response.status(), 'GET mentor/requests').toBe(200);
    const { invitations } = (await response.json()) as {
      invitations: { id: string; programId: string }[];
    };
    invitationId = invitations.find((invitation) => invitation.programId === programId)?.id ?? '';
    expect(invitationId, `invitation to ${programName}`).not.toBe('');
  }).toPass({ timeout: 60_000 });

  await gotoHydrated(page, '/mentorship/mentor/profile');
  await page.getByTestId('mentorship-mentor-profile-details-edit').getByRole('button').click();
  await expect(page.getByTestId('mentor-profile-edit-drawer-body')).toBeVisible();
  await expect(page.getByTestId(`mentorship-mentor-invitation-row-${invitationId}`)).toContainText(
    programName,
  );
  await page
    .getByTestId(`mentorship-mentor-invitation-accept-${invitationId}`)
    .getByRole('button')
    .click();
  await expect(page.getByText('You are now a mentor of this program.')).toBeVisible();
}

async function openMentorProgram(page: Page, programId: string): Promise<void> {
  await gotoHydrated(page, `/mentorship/mentor/programs/${programId}`);
  await expect(page.getByTestId('mentorship-mentor-program-detail-tab-mentees')).toBeVisible();
}

/** Creates a task for an accepted mentee from the program's Mentees tab. */
export async function createTask(
  page: Page,
  programId: string,
  applicationId: string,
  name: string,
): Promise<void> {
  const create = page.getByTestId(`mentorship-mentor-mentee-create-task-${applicationId}`);
  await expect(async () => {
    await openMentorProgram(page, programId);
    await page.getByTestId('mentorship-mentor-program-detail-tab-mentees').click();
    await expect(create).toBeVisible({ timeout: 10_000 });
  }).toPass({ timeout: 90_000 });
  await create.click();
  const dialog = page.getByTestId('mentorship-task-form-dialog');
  await expect(dialog).toBeVisible();
  await dialog.locator('#mentorship-task-name').fill(name);
  await dialog
    .locator('#mentorship-task-description')
    .fill(`${name}: created by the E2E journey suite.`);
  await page.getByTestId('mentorship-task-form-submit').click();
  await expect(page.getByText('Task created')).toBeVisible();
}

/** Approves a submitted task from the program's Tasks tab. */
export async function approveTask(
  page: Page,
  programId: string,
  applicationId: string,
  taskId: string,
): Promise<void> {
  const approve = page.getByTestId(`mentorship-mentor-task-approve-${applicationId}__${taskId}`);
  await expect(async () => {
    await openMentorProgram(page, programId);
    await expect(approve).toBeVisible({ timeout: 10_000 });
  }).toPass({ timeout: 90_000 });
  await approve.click();
  await expect(page.getByText('Task approved')).toBeVisible();
}
