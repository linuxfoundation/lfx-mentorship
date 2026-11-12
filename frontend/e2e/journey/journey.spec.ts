// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { existsSync } from 'node:fs';
import type { APIRequestContext, Page } from '@playwright/test';
import {
  decideApplication,
  declineAllApplications,
  inviteMentor,
  programTerms,
} from '../support/admin';
import { decideProgram, findAdminPrograms, hideProgram, PUBLISHED_STATUSES } from '../support/api';
import { enrollProgram, openTerm } from '../support/enroll';
import { expect, gotoHydrated, test } from '../support/fixtures';
import {
  apply,
  menteeApplication,
  registerFromApplyLink,
  setTaskStatus,
  uploadTaskFile,
} from '../support/mentee';
import { acceptInvitation, approveTask, createTask } from '../support/mentor';
import { env } from '../env';
import { credentials, storageStatePath, type User } from '../users';

// One program from enrollment to graduation on DEV, driven through Self Serve by several users,
// plus a second program with the admin roles swapped. Case numbers refer to
// docs/testing/e2e-test-plan.md.
//
// ADMIN_A  Program Admin of program A, approver of program B, then mentor of program B
// ADMIN_B  approver of program A, mentor of program A, then Program Admin of program B
// MENTEE_1 applies to program A, is accepted, works a task and graduates
// MENTEE_2 registers from program A's Apply link (on its first run), applies and is declined

const PREFIX = 'E2E Journey';
const RUN = new Date().toISOString().slice(0, 19).replace('T', ' ').replaceAll(':', '');
const PROGRAM_A = `${PREFIX} A ${RUN}`;
const PROGRAM_B = `${PREFIX} B ${RUN}`;
const TASK = `E2E Task ${RUN}`;

test.afterAll(async ({ playwright }, testInfo) => {
  // No sessions means the setup skipped, so the journey never ran.
  if (!existsSync(storageStatePath('ADMIN_A')) || !existsSync(storageStatePath('ADMIN_B'))) return;
  const as = (user: User): Promise<APIRequestContext> =>
    playwright.request.newContext({
      baseURL: testInfo.project.use.baseURL,
      storageState: storageStatePath(user),
    });
  const adminA = await as('ADMIN_A');
  const adminB = await as('ADMIN_B');
  try {
    // Every journey program still pending is rejected. Every published one has its applications
    // declined, so the mentees can apply again (a graduate never could), and is then hidden.
    // This includes programs an earlier run left behind when it failed.
    for (const [admin, approver] of [
      [adminA, adminB],
      [adminB, adminA],
    ] as [APIRequestContext, APIRequestContext][]) {
      for (const program of await findAdminPrograms(admin, PREFIX)) {
        if (program.status === 'pending-review') {
          await decideProgram(approver, program.id, 'reject');
        } else if (PUBLISHED_STATUSES.includes(program.status)) {
          await declineAllApplications(admin, program.id);
          await hideProgram(admin, program.id);
        }
      }
    }
  } finally {
    await adminA.dispose();
    await adminB.dispose();
  }
});

/** Approves a pending program from its review page, as an approver. */
async function approveProgram(
  approver: Page,
  admin: Page,
  programId: string,
  name: string,
): Promise<void> {
  await gotoHydrated(approver, `/mentorship/program-review/${programId}`);
  await expect(approver.getByTestId('mentorship-program-review-title')).toHaveText(name);
  const decision = approver.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === `/api/mentorship/program-review/${programId}/decision`,
  );
  await approver.getByTestId('mentorship-program-review-approve').click();
  expect((await decision).status(), `approve ${name}`).toBeLessThan(300);
  const [listed] = await findAdminPrograms(admin.request, name);
  expect(listed?.status, `${name} after approval`).toBe('accepting');
}

test('program journey: enroll to graduation', async ({ asUser }) => {
  const adminA = await asUser('ADMIN_A');
  const adminB = await asUser('ADMIN_B');
  const menteeOne = await asUser('MENTEE_1');
  const menteeTwo = await asUser('MENTEE_2');
  let programA = '';
  let termA = '';
  let menteeOneApp = '';

  await test.step('2.4 ADMIN_A enrolls program A', async () => {
    programA = await enrollProgram(adminA, PROGRAM_A, openTerm(`E2E Term A ${RUN}`));
    const [term] = await programTerms(adminA.request, programA);
    expect(term, 'program A term').toBeDefined();
    termA = term!.id;
  });

  await test.step('2.6 ADMIN_B approves program A', async () => {
    await approveProgram(adminB, adminA, programA, PROGRAM_A);
  });

  await test.step('3.3 ADMIN_A invites ADMIN_B as a mentor', async () => {
    await inviteMentor(adminA, programA, credentials('ADMIN_B')!.username);
  });

  await test.step('4.1 ADMIN_B accepts the invitation', async () => {
    await acceptInvitation(adminB, programA, PROGRAM_A);
  });

  await test.step('5.1, 5.2 MENTEE_2 opens the Apply link, registering first if new, and applies', async () => {
    const response = await menteeTwo.request.get('/api/mentorship/mentee/has-profile');
    expect(response.status(), 'GET mentee/has-profile').toBe(200);
    const { hasProfile } = (await response.json()) as { hasProfile: boolean };
    if (!hasProfile) await registerFromApplyLink(menteeTwo, programA, termA);
    await apply(menteeTwo, programA, termA, PROGRAM_A);
  });

  await test.step('5.3 MENTEE_1 applies', async () => {
    await apply(menteeOne, programA, termA, PROGRAM_A);
  });

  await test.step('5.4 MENTEE_1 uploads every prerequisite', async () => {
    const application = await menteeApplication(menteeOne, programA);
    menteeOneApp = application.id;
    expect(application.tasks.length, 'prerequisite tasks').toBeGreaterThan(0);
    for (const task of application.tasks) await uploadTaskFile(menteeOne, task.id);
  });

  await test.step('6.7 ADMIN_A declines MENTEE_2', async () => {
    const { id } = await menteeApplication(menteeTwo, programA);
    await decideApplication(adminA, programA, id, 'Decline');
  });

  await test.step('6.1 ADMIN_A accepts MENTEE_1', async () => {
    await decideApplication(adminA, programA, menteeOneApp, 'Accept');
  });

  await test.step('4.9 ADMIN_B, as mentor, creates a task for MENTEE_1', async () => {
    await createTask(adminB, programA, menteeOneApp, TASK);
  });

  let taskId = '';
  await test.step('5.8 MENTEE_1 starts the task and submits a file', async () => {
    await expect(async () => {
      const { tasks } = await menteeApplication(menteeOne, programA);
      taskId = tasks.find((task) => task.name === TASK)?.id ?? '';
      expect(taskId, TASK).not.toBe('');
    }).toPass({ timeout: 90_000 });
    await setTaskStatus(menteeOne, taskId, 'In Progress');
    await uploadTaskFile(menteeOne, taskId);
  });

  await test.step('4.10 ADMIN_B approves the task', async () => {
    await approveTask(adminB, programA, menteeOneApp, taskId);
  });

  await test.step('6.5 ADMIN_A graduates MENTEE_1', async () => {
    await decideApplication(adminA, programA, menteeOneApp, 'Graduate');
  });

  await test.step('5.11 the public site lists the graduate', async () => {
    await expect(async () => {
      const response = await adminA.request.get(
        `${env.publicUrl}/api/programs/${programA}/mentees`,
      );
      expect(response.status()).toBe(200);
      const mentees = (await response.json()) as { status: string }[];
      expect(mentees.map(({ status }) => status)).toContain('graduated');
    }).toPass({ timeout: 180_000 });
  });

  let programB = '';
  await test.step('2.4, 2.6 ADMIN_B enrolls program B and ADMIN_A approves it', async () => {
    programB = await enrollProgram(adminB, PROGRAM_B, openTerm(`E2E Term B ${RUN}`));
    await approveProgram(adminA, adminB, programB, PROGRAM_B);
  });

  await test.step('3.3, 4.1 ADMIN_B invites ADMIN_A as a mentor of program B, who accepts', async () => {
    await inviteMentor(adminB, programB, credentials('ADMIN_A')!.username);
    await acceptInvitation(adminA, programB, PROGRAM_B);
  });
});
