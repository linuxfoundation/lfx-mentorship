// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { Page } from '@playwright/test';
import { decideProgram, findAdminPrograms, hideProgram, ProgramListItem, ProgramListStatus } from '../support/api';
import { loginWithAuth0 } from '../support/auth';
import { e2eProgramName, enrollProgram } from '../support/enroll';
import { expect, test } from '../support/fixtures';
import { gotoHydrated } from '../support/page';
import { REVIEW_PROGRAMS, ReviewDecision, reviewPath } from '../support/test-data';
import { credentialsFor, storageStatePath, UserKey } from '../support/users';

const isDecisionPost = (url: string, method: string) => method === 'POST' && /\/api\/mentorship\/program-review\/[^/]+\/decision$/.test(new URL(url).pathname);

/** The latest pending program 4.7 enrolled under `prefix`, read as its admin (adminA). */
async function pendingProgram(adminPage: Page, prefix: string): Promise<ProgramListItem> {
  const [program] = await findAdminPrograms(adminPage.request, prefix, 'pending-review');
  expect(program, `a pending "${prefix} …" program: run 4.7 first`).toBeDefined();
  return program!;
}

async function openReviewLink(page: Page, programId: string, decision: ReviewDecision): Promise<void> {
  await gotoHydrated(page, reviewPath(programId, decision));
  await expect(page.getByTestId('mentorship-program-review-loading')).toHaveCount(0);
}

async function expectAdminCardStatus(adminPage: Page, program: ProgramListItem, label: string, status: ProgramListStatus): Promise<void> {
  const [listed] = await findAdminPrograms(adminPage.request, program.name);
  expect(listed?.status).toBe(status);

  await gotoHydrated(adminPage, '/mentorship/admin');
  await adminPage.locator('[data-test="mentorship-programs-search"]').fill(program.name);
  await expect(adminPage.getByTestId(`mentorship-program-card-${program.id}`).getByTestId('mentorship-program-card-status')).toContainText(label);
}

// Section 5 of docs/testing/p1-test-plan.md. adminB (samehlol16) reviews the programs 4.7 enrolled as adminA (sameh100);
// both are approvers. 5.1, 5.3 and 5.4 each use up one program; the others only read the "Pending" one.
test.describe('5. Program review', () => {
  // Programs this run approved (5.1, 5.4). Hidden afterwards, so test programs do not stay public with open terms.
  const approved: string[] = [];

  test.afterAll(async ({ playwright }, testInfo) => {
    const as = (key: UserKey) =>
      playwright.request.newContext({ baseURL: testInfo.project.use.baseURL, storageState: storageStatePath(key) });

    const adminA = await as('adminA');
    const adminB = await as('adminB');
    try {
      for (const programId of approved.splice(0)) {
        await hideProgram(adminA, programId);
      }
      // The run reads the newest "Pending" program; reject the older ones earlier runs left behind.
      const [, ...stale] = await findAdminPrograms(adminA, REVIEW_PROGRAMS.pending, 'pending-review');
      for (const program of stale) {
        await decideProgram(adminB, program.id, 'reject');
      }
    } finally {
      await adminA.dispose();
      await adminB.dispose();
    }
  });

  test('5.1 Approve [PR-01-01, PR-01-05]', async ({ asUser }) => {
    const admin = await asUser('adminA');
    const program = await pendingProgram(admin, REVIEW_PROGRAMS.approve);
    const approver = await asUser('adminB');
    const posts: string[] = [];
    approver.on('request', (request) => isDecisionPost(request.url(), request.method()) && posts.push(request.url()));

    await openReviewLink(approver, program.id, 'approve');
    await expect(approver.getByTestId('mentorship-program-review-title')).toHaveText('Approve this program?');
    await expect(approver.getByTestId('mentorship-program-review-program-name')).toHaveText(program.name);

    await approver.getByTestId('mentorship-program-review-confirm-button').click();
    const success = approver.getByTestId('mentorship-program-review-success');
    await expect(success).toContainText('Program approved');
    await expect(success).toContainText(`${program.name} has been approved.`);
    expect(posts, 'decision POSTs').toHaveLength(1);
    approved.push(program.id);

    await expectAdminCardStatus(admin, program, 'Open', 'open');
  });

  test('5.2 Open the link only [PR-01-02]', async ({ asUser }) => {
    const admin = await asUser('adminA');
    const program = await pendingProgram(admin, REVIEW_PROGRAMS.pending);
    const approver = await asUser('adminB');
    // The page reads the program during the server render, so the browser may send no GET at all; the confirm card shows the
    // read happened. What matters is that opening the link sends no write.
    const writes: string[] = [];
    approver.on('request', (request) => request.url().includes('/api/mentorship/program-review/') && request.method() !== 'GET' && writes.push(request.method()));

    await openReviewLink(approver, program.id, 'approve');
    await expect(approver.getByTestId('mentorship-program-review-confirm')).toBeVisible();
    // Give a late request time to show up before counting.
    await approver.waitForTimeout(2_000);

    expect(writes, 'program-review writes').toEqual([]);
    const review = await admin.request.get(`/api/mentorship/program-review/${program.id}`);
    expect(((await review.json()) as { status: string }).status).toBe('pending');
  });

  test('5.3 Reject [PR-02-01]', async ({ asUser }) => {
    const admin = await asUser('adminA');
    const program = await pendingProgram(admin, REVIEW_PROGRAMS.reject);
    const approver = await asUser('adminB');

    await openReviewLink(approver, program.id, 'reject');
    await expect(approver.getByTestId('mentorship-program-review-title')).toHaveText('Reject this program?');
    const confirm = approver.getByTestId('mentorship-program-review-confirm-button').getByRole('button');
    await expect(confirm).toHaveText('Reject');
    await expect(confirm).toHaveClass(/p-button-danger/);

    await confirm.click();
    await expect(approver.getByTestId('mentorship-program-review-success')).toContainText('Program rejected');

    await expectAdminCardStatus(admin, program, 'Rejected', 'rejected');
  });

  test('5.4 Two approvers at once [PR-04-02]', async ({ asUser }) => {
    const first = await asUser('adminA');
    const program = await pendingProgram(first, REVIEW_PROGRAMS.race);
    const second = await asUser('adminB');

    await openReviewLink(first, program.id, 'approve');
    await openReviewLink(second, program.id, 'approve');
    await expect(first.getByTestId('mentorship-program-review-confirm')).toBeVisible();
    await expect(second.getByTestId('mentorship-program-review-confirm')).toBeVisible();

    await second.getByTestId('mentorship-program-review-confirm-button').click();
    await expect(second.getByTestId('mentorship-program-review-success')).toContainText('Program approved');
    approved.push(program.id);

    const late = first.waitForResponse((response) => isDecisionPost(response.url(), response.request().method()));
    await first.getByTestId('mentorship-program-review-confirm-button').click();
    expect((await late).status()).toBe(409);
    await expect(first.getByTestId('mentorship-program-review-already-decided')).toContainText(
      'Another reviewer decided this program before you confirmed.'
    );
  });

  test('5.5 Non-approver opens the link [PR-05-01]', async ({ asUser }) => {
    const admin = await asUser('adminA');
    const program = await pendingProgram(admin, REVIEW_PROGRAMS.pending);
    const mentee = await asUser('mentee1');

    const read = mentee.waitForResponse((response) => response.url().includes(`/api/mentorship/program-review/${program.id}`));
    await openReviewLink(mentee, program.id, 'approve');
    expect((await read).status()).toBe(403);
    await expect(mentee.getByTestId('mentorship-program-review-forbidden')).toContainText("You don't have permission to review programs");
    await expect(mentee.getByTestId('mentorship-program-review-confirm')).toHaveCount(0);
  });

  // Makes sameh001 a Program Admin, so it runs only at the very end of the plan: set E2E_RUN_LAST=1.
  test('5.6 Program\'s own admin, not an approver, confirms [PR-05-02]', async ({ asUser }) => {
    test.skip(!process.env['E2E_RUN_LAST'], 'Runs last in the plan (sameh001 becomes a Program Admin): set E2E_RUN_LAST=1');
    test.setTimeout(3 * 60_000);
    const owner = await asUser('mentee1');
    const name = e2eProgramName('E2E P1 Own Admin');
    const programId = await enrollProgram(owner, name);

    await openReviewLink(owner, programId, 'approve');
    const confirmCard = owner.getByTestId('mentorship-program-review-confirm');
    // The confirm card may or may not show; either way nothing is recorded.
    if (await confirmCard.isVisible()) {
      const decision = owner.waitForResponse((response) => isDecisionPost(response.url(), response.request().method()));
      await owner.getByTestId('mentorship-program-review-confirm-button').click();
      expect((await decision).status()).toBe(403);
    }
    await expect(owner.getByTestId('mentorship-program-review-forbidden')).toBeVisible();
    const [listed] = await findAdminPrograms(owner.request, name);
    expect(listed?.status).toBe('pending-review');

    // Clean up: reject it as an approver.
    const approver = await asUser('adminB');
    await decideProgram(approver.request, programId, 'reject');
  });

  test('5.7 Timeout, then retry [PR-07-03]', async () => {
    test.fixme(true, 'Fault case: needs a stubbed backend that times out on the decision POST');
  });

  test('5.8 Logged out [PR-08-02]', async ({ asUser, browser, baseURL }) => {
    const credentials = credentialsFor('adminB');
    test.skip(!credentials, 'No credentials for adminB');
    const admin = await asUser('adminA');
    const program = await pendingProgram(admin, REVIEW_PROGRAMS.pending);

    const context = await browser.newContext({ baseURL });
    try {
      const page = await context.newPage();
      await page.goto(reviewPath(program.id, 'approve'));
      await loginWithAuth0(page, credentials!);

      // Back on the link, decision kept; nothing is confirmed here.
      await expect(page).toHaveURL(new RegExp(`/mentorship/program-review/${program.id}\\?decision=approve$`));
      await expect(page.getByTestId('mentorship-program-review-title')).toHaveText('Approve this program?');
    } finally {
      await context.close();
    }
  });

  test('5.9 First-time approver [PR-08-04]', async () => {
    test.fixme(true, 'No approver account that has never opened Mentorship');
  });

  test('5.10 Cross-site form POST to /program-review/:id/decision [PR-08-06]', async ({ asUser, baseURL }) => {
    const admin = await asUser('adminA');
    const program = await pendingProgram(admin, REVIEW_PROGRAMS.pending);
    const approver = await asUser('adminB');

    // A page on another site that auto-submits a form to the decision endpoint, as an attacker's page would.
    const attacker = 'https://e2e-cross-site.example';
    const decisionUrl = `${baseURL}/api/mentorship/program-review/${program.id}/decision`;
    await approver.route(`${attacker}/**`, (route) =>
      route.fulfill({
        contentType: 'text/html',
        body: `<form method="POST" action="${decisionUrl}"><input name="decision" value="approve"></form><script>document.forms[0].submit()</script>`,
      })
    );

    const decision = approver.waitForResponse((response) => response.url() === decisionUrl);
    await approver.goto(`${attacker}/`);
    expect((await decision).status()).toBe(401);

    const review = await admin.request.get(`/api/mentorship/program-review/${program.id}`);
    expect(((await review.json()) as { status: string }).status).toBe('pending');
  });
});
