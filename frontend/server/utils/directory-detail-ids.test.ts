// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { createError } from 'h3';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fetchMentee } from './mentee';
import { fetchMentor } from './mentor';

const fetchMock = vi.fn();
vi.stubGlobal('$fetch', fetchMock);
vi.stubGlobal('useRuntimeConfig', () => ({ apiBaseUrl: 'http://api' }));
vi.stubGlobal('createError', createError);

beforeEach(() => {
  fetchMock.mockReset();
  fetchMock.mockResolvedValue({ user_id: 'u1', joined_at: '' });
});

describe.each([
  ['fetchMentee', fetchMentee, 'mentees'],
  ['fetchMentor', fetchMentor, 'mentors'],
])('%s', (_name, fetcher, segment) => {
  it.each(['a b/../c?x=1#f', '..', '..\\programs\\slug\\catalog', '%2e%2e', ''])(
    'refuses the non-UUID id %j without calling the backend',
    async (id) => {
      await expect(fetcher(id)).rejects.toMatchObject({ statusCode: 404 });
      expect(fetchMock).not.toHaveBeenCalled();
    },
  );

  it('passes a user UUID through as the backend path segment', async () => {
    await fetcher('2f1c9a7e-5b3d-4c8e-9f0a-1b2c3d4e5f60');
    expect(fetchMock).toHaveBeenCalledWith(
      `http://api/v1/${segment}/2f1c9a7e-5b3d-4c8e-9f0a-1b2c3d4e5f60`,
    );
  });
});
