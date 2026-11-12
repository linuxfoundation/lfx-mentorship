// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { MENTEE_PAGE_SIZE } from '../../app/components/modules/mentees/config/mentees-header.config';
import { MENTOR_PAGE_SIZE } from '../../app/components/modules/mentors/config/mentors-header.config';
import { PROGRAM_PAGE_SIZE } from '../../app/components/modules/programs/config/programs-header.config';
import { env } from '../env';

// Signed-out checks of the public site. Case numbers refer to docs/testing/e2e-test-plan.md.
// The data is live, so each test picks its records from the public API and asserts the page's
// shape, never counts or names known in advance.

const UNKNOWN_ID = '00000000-0000-4000-8000-000000000e2e';

interface ListResponse<T> {
  data: T[];
  total: number;
}

interface Entry {
  id: string;
  slug?: string;
  name: string;
}

async function getJson<T>(request: APIRequestContext, path: string): Promise<T> {
  const response = await request.get(path);
  expect(response.status(), path).toBe(200);
  return (await response.json()) as T;
}

async function firstEntry(request: APIRequestContext, path: string): Promise<Entry> {
  const list = await getJson<ListResponse<Entry>>(request, `${path}?limit=1`);
  expect(list.data, `${path} returned no records`).not.toHaveLength(0);
  return list.data[0]!;
}

// The public API must never expose contact details; mentees and mentors enter their own names,
// so the check is on field names rather than on text that looks like an address.
function expectNoEmailFields(body: unknown, path: string): void {
  const keys = JSON.stringify(body).match(/"[^"]*email[^"]*":/gi) ?? [];
  expect(keys, `${path} exposes email fields`).toEqual([]);
}

// Each card on a directory page links to its detail page, e.g. /programs/{id}.
async function expectUniqueCards(page: Page, section: string, count: number): Promise<void> {
  const cards = page.locator(`main a[href^="/${section}/"]`);
  await expect(cards).toHaveCount(count);
  const hrefs = await cards.evaluateAll((links) => links.map((link) => link.getAttribute('href')));
  expect(new Set(hrefs).size, 'duplicate cards').toBe(count);
}

async function expectNotFound(page: Page, path: string): Promise<void> {
  const response = await page.goto(path);
  expect(response?.status(), path).toBe(404);
  await expect(page.getByRole('heading', { level: 1, name: 'Page not found' })).toBeVisible();
}

test('1.1 landing page shows platform stats and links to Self Serve', async ({ page, request }) => {
  const summary = await getJson<{ programCount: number; mentorCount: number }>(
    request,
    '/api/summary',
  );
  expect(summary.programCount).toBeGreaterThan(0);
  expect(summary.mentorCount).toBeGreaterThan(0);

  await page.goto('/');
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  for (const stat of ['Programs Accepting', 'Mentees Graduated', 'Active Mentors']) {
    await expect(page.getByRole('main').getByText(stat, { exact: true })).toBeVisible();
  }
  for (const name of ['Become a Mentor', 'My Mentorship']) {
    const href = await page.getByRole('link', { name }).first().getAttribute('href');
    expect(href?.startsWith(`${env.selfServeUrl}/mentorship`), `${name}: ${href}`).toBe(true);
  }
});

test('1.2 programs list pages through published programs', async ({ page, request }) => {
  const { total } = await getJson<ListResponse<Entry>>(request, '/api/programs?limit=1');

  await page.goto('/programs');
  const tabs = page.getByRole('main').getByRole('tab');
  await expect(tabs).toHaveText(['All', 'Accepting', 'In progress', 'Completed']);
  await expectUniqueCards(page, 'programs', Math.min(total, PROGRAM_PAGE_SIZE));

  test.skip(total <= PROGRAM_PAGE_SIZE, 'one page of programs; nothing to load');
  await page.getByRole('button', { name: 'Load More' }).click();
  await expectUniqueCards(page, 'programs', Math.min(total, 2 * PROGRAM_PAGE_SIZE));
});

test('1.2 programs search shows the empty state', async ({ page }) => {
  await page.goto('/programs');
  await page.getByPlaceholder('Search programs...').fill(`e2e no match ${UNKNOWN_ID}`);
  await expect(page.getByText('No programs found.')).toBeVisible();
});

test('1.3 program detail opens by slug and by ID', async ({ page, request }) => {
  const program = await firstEntry(request, '/api/programs');
  for (const path of [`/api/programs/${program.id}`, `/api/programs/${program.id}/mentees`]) {
    expectNoEmailFields(await getJson(request, path), path);
  }
  // F1: sponsors failed for programs without a Crowdfunding initiative.
  await getJson(request, `/api/programs/${program.id}/sponsors`);

  for (const key of [program.slug, program.id]) {
    await page.goto(`/programs/${key}`);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(program.name);
  }

  const sections = page.getByRole('tablist', { name: 'Program sections' }).getByRole('tab');
  await expect(sections).toHaveText(['Overview', 'Terms', 'Mentors', 'Mentees', 'Sponsors']);
  for (const tab of await sections.all()) {
    await tab.click();
    await expect(tab).toHaveAttribute('aria-selected', 'true');
  }
  // The Sponsors tab is last, so it is open now.
  await expect(page.getByText('Loading sponsors…')).toHaveCount(0);
  await expect(page.getByText('Unable to load sponsors')).toHaveCount(0);
});

for (const directory of [
  { section: 'mentors', heading: 'Mentors', pageSize: MENTOR_PAGE_SIZE, tabs: [] as string[] },
  {
    section: 'mentees',
    heading: 'Mentees',
    pageSize: MENTEE_PAGE_SIZE,
    tabs: ['All', 'Active', 'Graduated'],
  },
]) {
  test(`1.6 ${directory.section} directory lists profiles`, async ({ page, request }) => {
    const { total } = await getJson<ListResponse<Entry>>(
      request,
      `/api/${directory.section}?limit=1`,
    );

    await page.goto(`/${directory.section}`);
    await expect(page.getByRole('heading', { level: 1 })).toContainText(directory.heading);
    if (directory.tabs.length) {
      await expect(page.getByRole('main').getByRole('tab')).toHaveText(directory.tabs);
    }
    await expectUniqueCards(page, directory.section, Math.min(total, directory.pageSize));

    test.skip(total <= directory.pageSize, 'one page of profiles; nothing to load');
    await page.getByRole('button', { name: 'Load More' }).click();
    await expectUniqueCards(page, directory.section, Math.min(total, 2 * directory.pageSize));
  });

  test(`1.6 ${directory.section} profile page`, async ({ page, request }) => {
    const profile = await firstEntry(request, `/api/${directory.section}`);
    const path = `/api/${directory.section}/${profile.id}`;
    expectNoEmailFields(await getJson(request, path), path);

    await page.goto(`/${directory.section}/${profile.id}`);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(profile.name);
  });
}

test('1.6, 1.8 unknown program and profiles give 404', async ({ page }) => {
  await expectNotFound(page, '/programs/e2e-no-such-program');
  await expectNotFound(page, `/programs/${UNKNOWN_ID}`);
  await expectNotFound(page, `/mentors/${UNKNOWN_ID}`);
  await expectNotFound(page, `/mentees/${UNKNOWN_ID}`);
});
