// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { expect, test, type APIRequestContext } from '@playwright/test';
import { env } from '../env';

// Legacy URL redirects (section 11 and F16 of docs/testing/e2e-test-plan.md) and the signed-out
// Self Serve redirect (7.1). The legacy routes do not look the IDs up, so synthetic ones do.

const PROGRAM_ID = '00000000-0000-4000-8000-0000000e2e01';
const USER_ID = '00000000-0000-4000-8000-0000000e2e02';

async function expectRedirect(
  request: APIRequestContext,
  url: string,
  status: number,
  location: string,
): Promise<void> {
  const response = await request.get(url, { maxRedirects: 0 });
  expect(response.status(), url).toBe(status);
  // Locations may be relative; compare them resolved.
  expect(new URL(response.headers().location ?? '', url).href, url).toBe(location);
}

const PUBLIC_REDIRECTS: ReadonlyArray<readonly [string, string]> = [
  [`/project/${PROGRAM_ID}`, `/programs/${PROGRAM_ID}`],
  [`/project/${PROGRAM_ID}/detail`, `/programs/${PROGRAM_ID}`],
  [`/mentee/${USER_ID}`, `/mentees/${USER_ID}`],
  [`/mentee/${USER_ID},${PROGRAM_ID}`, `/mentees/${USER_ID}`],
  [`/mentor/${USER_ID}`, `/mentors/${USER_ID}`],
  ['/participate/project', '/'],
  ['/profile', '/'],
];

for (const [from, to] of PUBLIC_REDIRECTS) {
  test(`11 legacy ${from} redirects to ${to}`, async ({ request }) => {
    await expectRedirect(request, `${env.publicUrl}${from}`, 301, `${env.publicUrl}${to}`);
  });
}

// Signed-in legacy flows now served by Self Serve; 302 so browsers don't cache them.
const SELF_SERVE_REDIRECTS: ReadonlyArray<readonly [string, string]> = [
  [
    `/email/process-approval?id=${PROGRAM_ID}&action=approve`,
    `/mentorship/program-review/${PROGRAM_ID}?decision=approve`,
  ],
  [`/project/${PROGRAM_ID}/mentees`, `/mentorship/admin/${PROGRAM_ID}`],
  [`/participate/mentor/join/${PROGRAM_ID}`, '/mentorship/mentor'],
  ['/mentee/tasks', '/mentorship/mentee/tasks'],
];

for (const [from, to] of SELF_SERVE_REDIRECTS) {
  test(`F16 legacy ${from} redirects to Self Serve`, async ({ request }) => {
    await expectRedirect(request, `${env.publicUrl}${from}`, 302, `${env.selfServeUrl}${to}`);
  });
}

// F13: legacy /mentee and /mentor with no ID still return 404.
for (const path of ['/mentee', '/mentor']) {
  test.fixme(`11 legacy ${path} with no ID redirects`, async ({ request }) => {
    const response = await request.get(`${env.publicUrl}${path}`, { maxRedirects: 0 });
    expect(response.status()).toBe(301);
  });
}

test('11 legacy host redirects here, keeping path and query', async ({ request }) => {
  test.skip(!env.legacyUrl, 'this environment has no legacy host');
  const path = `/project/${PROGRAM_ID}?ref=e2e`;
  await expectRedirect(request, `${env.legacyUrl}${path}`, 301, `${env.publicUrl}${path}`);
});

test('7.1 signed out, Self Serve Mentorship redirects to sign-in', async ({ request }) => {
  const url = `${env.selfServeUrl}/mentorship`;
  const response = await request.get(url, { maxRedirects: 0 });
  expect(response.status(), url).toBe(302);
  const location = new URL(response.headers().location ?? '', url);
  expect(location.pathname).toBe('/authorize');
  expect(location.searchParams.get('redirect_uri')).toBe(`${env.selfServeUrl}/callback`);
});
