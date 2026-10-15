// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { createError } from 'h3';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fetchPlatformSummary } from './platform-summary';

const fetchMock = vi.fn();
vi.stubGlobal('$fetch', fetchMock);
vi.stubGlobal('useRuntimeConfig', () => ({ apiBaseUrl: 'http://api' }));
vi.stubGlobal('createError', createError);

beforeEach(() => {
  fetchMock.mockReset();
});

describe('fetchPlatformSummary', () => {
  it('maps the backend summary', async () => {
    fetchMock.mockResolvedValue({
      program_count: 10,
      project_count: 4,
      accepting_program_count: 3,
      mentor_count: 20,
      stipends_paid: 12345,
      graduated_mentee_count: 7,
      graduated_mentee_users: [
        { name: 'Ada', avatar_url: 'a.png' },
        { name: null, avatar_url: null },
      ],
    });
    await expect(fetchPlatformSummary()).resolves.toEqual({
      programCount: 10,
      acceptingProgramCount: 3,
      mentorCount: 20,
      graduatedMenteeCount: 7,
      foundationCount: 4,
      stipendsPaid: 12345,
      graduatedMentees: [
        { name: 'Ada', avatarUrl: 'a.png' },
        { name: undefined, avatarUrl: undefined },
      ],
    });
    expect(fetchMock).toHaveBeenCalledWith('http://api/v1/summary');
  });

  it('defaults to no graduated mentees', async () => {
    fetchMock.mockResolvedValue({ program_count: 0 });
    await expect(fetchPlatformSummary()).resolves.toMatchObject({ graduatedMentees: [] });
  });

  it.each([
    [{ statusCode: 504 }, 504],
    [new Error('down'), 502],
  ])('maps an upstream failure to a stable error', async (upstream, statusCode) => {
    fetchMock.mockRejectedValue(upstream);
    await expect(fetchPlatformSummary()).rejects.toMatchObject({
      statusCode,
      message: 'Failed to load landing summary',
    });
  });
});
