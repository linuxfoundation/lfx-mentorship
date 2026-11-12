// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { expect, type APIRequestContext, type Page } from '@playwright/test';
import { gotoHydrated } from './fixtures';

// The Program Admin's pages in Self Serve (/mentorship/admin/<programId>).

export interface AdminProgramTerm {
  id: string;
  name: string;
  status: string;
}

/** The program's terms, as its admin reads them. */
export async function programTerms(
  request: APIRequestContext,
  programId: string,
): Promise<AdminProgramTerm[]> {
  const response = await request.get(`/api/mentorship/admin/programs/${programId}`);
  expect(response.status(), `GET admin/programs/${programId}`).toBe(200);
  return ((await response.json()) as { terms: AdminProgramTerm[] }).terms;
}

async function openAdminProgram(page: Page, programId: string): Promise<void> {
  await gotoHydrated(page, `/mentorship/admin/${programId}`);
  await expect(page.getByTestId('mentorship-current-mentees-tab')).toBeVisible();
}

/** Invites a user as a mentor from the program's Mentors tab, by LFID. */
export async function inviteMentor(page: Page, programId: string, lfid: string): Promise<void> {
  await openAdminProgram(page, programId);
  await page.getByTestId('mentorship-program-detail-tab-mentors').click();
  await expect(page.getByTestId('mentorship-mentors-tab')).toBeVisible();
  await page.locator('#mentorship-mentors-invitee-input').fill(lfid);
  await page.getByTestId(`mentorship-mentors-candidate-${lfid}`).click();
  await page.getByTestId('mentorship-mentors-invite').locator('button').click();
  await expect(page.getByText('Invitation sent.')).toBeVisible();
}

export type ApplicationAction = 'Accept' | 'Decline' | 'Graduate';

const ACTION_DONE: Record<ApplicationAction, string> = {
  Accept: 'Application accepted',
  Decline: 'Application declined',
  Graduate: 'Mentee graduated',
};

/**
 * Accepts, declines or graduates an application from its row on the Current Mentees tab. A new
 * application can take a few seconds to reach the list, so the page is reloaded until its row shows.
 */
export async function decideApplication(
  page: Page,
  programId: string,
  applicationId: string,
  action: ApplicationAction,
): Promise<void> {
  const actions = page.getByTestId(`mentorship-current-mentee-actions-${applicationId}`);
  await expect(async () => {
    await openAdminProgram(page, programId);
    await expect(actions).toBeVisible({ timeout: 10_000 });
  }).toPass({ timeout: 90_000 });
  await actions.click();
  await page.getByRole('menuitem', { name: action, exact: true }).click();
  if (action === 'Accept') {
    await expect(page.getByTestId('mentorship-admin-accept-dialog')).toBeVisible();
    await page.locator('[data-test="mentorship-admin-accept-attendance"]').click();
    await page.getByRole('option', { name: 'Full time', exact: true }).click();
    await page.getByTestId('mentorship-admin-accept-confirm').getByRole('button').click();
  } else {
    await page
      .locator('.p-confirmdialog')
      .getByRole('button', { name: action, exact: true })
      .click();
  }
  await expect(page.getByText(ACTION_DONE[action])).toBeVisible();
}

async function setApplicationStatus(
  request: APIRequestContext,
  applicationId: string,
  status: 'accepted' | 'declined',
): Promise<void> {
  const data = status === 'accepted' ? { status, attendanceType: 'full_time' } : { status };
  const response = await request.patch(
    `/api/mentorship/admin/applications/${applicationId}/status`,
    { data },
  );
  expect(response.status(), `set ${applicationId} ${status}`).toBeLessThan(300);
}

const PAGE_SIZE = 10;
const SETTLED = ['declined', 'withdrawn'];

/**
 * Declines every application in the program, as its admin, so the program can be hidden and the
 * mentees can apply again. A graduation is walked back first: graduated can only go to accepted.
 */
export async function declineAllApplications(
  request: APIRequestContext,
  programId: string,
): Promise<void> {
  for (const type of ['current', 'past']) {
    const applications: { id: string; status: string }[] = [];
    for (let offset = 0; ; offset += PAGE_SIZE) {
      const path = `/api/mentorship/admin/programs/${programId}/mentees?type=${type}&offset=${offset}&limit=${PAGE_SIZE}`;
      const response = await request.get(path);
      expect(response.status(), `GET ${path}`).toBe(200);
      const { data, total } = (await response.json()) as {
        data: { id: string; status: string }[];
        total: number;
      };
      applications.push(...data);
      if (offset + PAGE_SIZE >= total) break;
    }
    for (const { id, status } of applications) {
      if (SETTLED.includes(status)) continue;
      if (status === 'graduated') await setApplicationStatus(request, id, 'accepted');
      await setApplicationStatus(request, id, 'declined');
    }
  }
}
