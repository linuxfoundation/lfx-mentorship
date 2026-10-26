// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { vi, describe, it, expect, beforeEach } from 'vitest';
import * as h3 from 'h3';
import type { H3Event } from 'h3';
import legacyRedirects from './legacy-redirects';

vi.mock('h3', async (importOriginal) => ({
  ...(await importOriginal<typeof import('h3')>()),
  defineEventHandler: (fn: unknown) => fn,
  getRequestURL: vi.fn(),
  sendRedirect: vi.fn(),
}));

const handler = legacyRedirects as unknown as (event: H3Event) => unknown;
const mockGetRequestURL = vi.mocked(h3.getRequestURL);
const mockSendRedirect = vi.mocked(h3.sendRedirect);

const ID = '2f1c9a7e-5b3d-4c8e-9f0a-1b2c3d4e5f60';
const PROJECT = '7a6b5c4d-3e2f-4a1b-9c8d-7e6f5a4b3c2d';
const SELF_SERVE = 'https://app.dev.lfx.dev';

vi.stubGlobal('useRuntimeConfig', () => ({ public: { selfServeUrl: `${SELF_SERVE}/` } }));

// Simulates a request for the given path on the new host.
const request = (path: string, method = 'GET'): H3Event => {
  mockGetRequestURL.mockReturnValue(new URL(path, 'https://mentorship.linuxfoundation.org'));
  return { method } as H3Event;
};

beforeEach(() => {
  vi.clearAllMocks();
});

describe('legacy-redirects middleware', () => {
  it.each([
    [`/project/${ID}`, `/programs/${ID}`],
    [`/project/${ID}/`, `/programs/${ID}`],
    [`/project/${ID}/tasks`, `/programs/${ID}`],
    ['/project/kubernetes-lfx', '/programs/kubernetes-lfx'],
    [`/mentee/${ID}`, `/mentees/${ID}`],
    [`/mentee/${ID},${PROJECT}`, `/mentees/${ID}`],
    [`/mentee/${ID}%2C${PROJECT}`, `/mentees/${ID}`],
    [`/mentor/${ID}`, `/mentors/${ID}`],
  ])('redirects the legacy page %s to %s', (from, to) => {
    handler(request(from));
    expect(mockSendRedirect).toHaveBeenCalledWith(expect.anything(), to, 301);
  });

  it('keeps the query string when redirecting a legacy page', () => {
    handler(request(`/project/${ID}?utm_source=cncf`));
    expect(mockSendRedirect).toHaveBeenCalledWith(
      expect.anything(),
      `/programs/${ID}?utm_source=cncf`,
      301,
    );
  });

  it('redirects a HEAD request', () => {
    handler(request(`/mentor/${ID}`, 'HEAD'));
    expect(mockSendRedirect).toHaveBeenCalledWith(expect.anything(), `/mentors/${ID}`, 301);
  });

  it.each([
    [`/email/process-approval?id=${ID}&action=approve`, `/mentorship/program-review/${ID}`],
    ['/email/process-approval?id=a%20b%2Fc', '/mentorship/program-review/a%20b%2Fc'],
    [`/project/${ID}/mentees`, `/mentorship/admin/${ID}`],
    [`/project/${ID}/mentees/`, `/mentorship/admin/${ID}`],
    ['/participate/mentor', '/mentorship/mentor'],
    [`/participate/mentor/join/${ID}?token=abc`, '/mentorship/mentor'],
    [`/participate/mentor/decline/${ID}?token=abc`, '/mentorship/mentor'],
    ['/profile/mentor/edit', '/mentorship/mentor/profile'],
    ['/mentee/tasks', '/mentorship/mentee/tasks'],
    ['/mentee/applications', '/mentorship/mentee'],
    ['/project/applied', '/mentorship/mentee'],
  ])('sends the legacy signed-in link %s to Self Serve %s without its query', (from, to) => {
    handler(request(from));
    expect(mockSendRedirect).toHaveBeenCalledWith(expect.anything(), `${SELF_SERVE}${to}`, 302);
  });

  it.each([
    '/participate',
    `/participate/mentee/${ID}`,
    '/profile',
    '/profile/edit',
    '/profile/mentor',
    '/email/verify?token=abc',
    '/email/process-approval',
    '/email/process-approval?id=&action=approve',
  ])('sends the legacy flow page %s to the home page without its query', (from) => {
    handler(request(from));
    expect(mockSendRedirect).toHaveBeenCalledWith(expect.anything(), '/', 301);
  });

  it.each([
    '/',
    `/programs/${ID}`,
    `/mentees/${ID}`,
    `/mentors/${ID}`,
    '/programs/enroll',
    `/api/programs/${ID}`,
    '/project',
    '/project/',
    `/mentor/${ID}/extra`,
    `/mentee/${ID}/extra`,
  ])('leaves %s alone', (path) => {
    handler(request(path));
    expect(mockSendRedirect).not.toHaveBeenCalled();
  });

  it('leaves a non-GET request alone', () => {
    handler(request(`/project/${ID}`, 'POST'));
    expect(mockSendRedirect).not.toHaveBeenCalled();
  });
});
