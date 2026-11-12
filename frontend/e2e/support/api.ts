// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { expect, type APIRequestContext } from '@playwright/test';

// Self Serve BFF calls, made with a signed-in user's session (a page's `page.request`).

export type ProgramListStatus =
  'accepting' | 'in-progress' | 'upcoming' | 'pending-review' | 'completed' | 'rejected' | 'hidden';

/** The statuses of a program that is public. */
export const PUBLISHED_STATUSES: readonly ProgramListStatus[] = [
  'accepting',
  'in-progress',
  'upcoming',
  'completed',
];

export interface AdminProgram {
  id: string;
  name: string;
  status: ProgramListStatus;
  createdOn: string;
}

/** The user's admin programs whose name starts with `prefix`, newest first. */
export async function findAdminPrograms(
  request: APIRequestContext,
  prefix: string,
  status?: ProgramListStatus,
): Promise<AdminProgram[]> {
  const params = new URLSearchParams({ limit: '50', search: prefix });
  if (status) params.set('status', status);
  const response = await request.get(`/api/mentorship/admin/programs?${params}`);
  expect(response.status(), `GET admin/programs?${params}`).toBe(200);
  const { data } = (await response.json()) as { data: AdminProgram[] };
  return data
    .filter((program) => program.name.startsWith(prefix))
    .sort((a, b) => b.createdOn.localeCompare(a.createdOn));
}

/** Records an approver's decision on a pending program, as the review page does. */
export async function decideProgram(
  request: APIRequestContext,
  programId: string,
  decision: 'approve' | 'reject',
): Promise<void> {
  const response = await request.post(`/api/mentorship/program-review/${programId}/decision`, {
    data: { decision },
  });
  expect(response.status(), `${decision} ${programId}`).toBeLessThan(300);
}

/** Takes a published program out of public view, as its admin. */
export async function hideProgram(request: APIRequestContext, programId: string): Promise<void> {
  const response = await request.post(`/api/mentorship/admin/programs/${programId}/hide`);
  expect(response.status(), `hide ${programId}`).toBeLessThan(300);
}
