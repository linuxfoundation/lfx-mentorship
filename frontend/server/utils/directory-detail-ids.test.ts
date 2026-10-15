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
  it('encodes the id in the backend path', async () => {
    await fetcher('a b/../c?x=1#f');
    expect(fetchMock).toHaveBeenCalledWith(`http://api/v1/${segment}/a%20b%2F..%2Fc%3Fx%3D1%23f`);
  });

  it('leaves a plain id unchanged', async () => {
    await fetcher('2f1c9a7e-5b3d-4c8e-9f0a-1b2c3d4e5f60');
    expect(fetchMock).toHaveBeenCalledWith(
      `http://api/v1/${segment}/2f1c9a7e-5b3d-4c8e-9f0a-1b2c3d4e5f60`,
    );
  });
});
