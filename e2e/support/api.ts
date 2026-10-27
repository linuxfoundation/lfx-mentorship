// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { APIRequestContext, expect } from '@playwright/test';

export type ProgramListStatus = 'open' | 'pending-review' | 'completed' | 'rejected' | 'hidden';

export interface ProgramListItem {
  id: string;
  name: string;
  status: ProgramListStatus;
}

/**
 * The signed-in user's admin programs whose name starts with `prefix` (all of them when it is empty), read from the BFF with
 * that user's session: a page's `page.request`, or a request context opened on the user's storage state. Newest first, so a test
 * picks the latest one a run created.
 */
export async function findAdminPrograms(request: APIRequestContext, prefix: string, status?: ProgramListStatus): Promise<ProgramListItem[]> {
  const params = new URLSearchParams({ limit: '50' });
  if (prefix) params.set('search', prefix);
  if (status) params.set('status', status);
  const response = await request.get(`/api/mentorship/admin/programs?${params}`);
  expect(response.status(), `GET admin/programs?${params}`).toBe(200);
  const { data } = (await response.json()) as { data: (ProgramListItem & { createdOn: string })[] };
  return data.filter((program) => program.name.startsWith(prefix)).sort((a, b) => b.createdOn.localeCompare(a.createdOn));
}

/** Hides a published program as its admin, so a test program stops showing publicly with open terms. */
export async function hideProgram(request: APIRequestContext, programId: string): Promise<void> {
  const response = await request.post(`/api/mentorship/admin/programs/${programId}/hide`);
  expect(response.status(), `hide ${programId}`).toBeLessThan(300);
}

/** Records an approver's decision on a pending program, as the review page's Confirm does. */
export async function decideProgram(request: APIRequestContext, programId: string, decision: 'approve' | 'reject'): Promise<void> {
  const response = await request.post(`/api/mentorship/program-review/${programId}/decision`, { data: { decision } });
  expect(response.status(), `${decision} ${programId}`).toBe(200);
}
