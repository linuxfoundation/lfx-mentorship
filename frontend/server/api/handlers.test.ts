// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { createError, type H3Event } from 'h3';
import { beforeEach, describe, expect, it, vi } from 'vitest';

type Handler = (event: H3Event) => unknown;
type FakeEvent = { params?: Record<string, string>; query?: Record<string, unknown> };

const USER_ID = '2f1c9a7e-5b3d-4c8e-9f0a-1b2c3d4e5f60';
const fetchMock = vi.fn();
vi.stubGlobal('$fetch', fetchMock);
vi.stubGlobal('useRuntimeConfig', () => ({ apiBaseUrl: 'http://api' }));
vi.stubGlobal('createError', createError);
vi.stubGlobal('defineEventHandler', (fn: Handler) => fn);
vi.stubGlobal('defineSitemapEventHandler', (fn: Handler) => fn);
vi.stubGlobal('getRouterParam', (event: FakeEvent, name: string) => event.params?.[name]);
vi.stubGlobal('getQuery', (event: FakeEvent) => event.query ?? {});

// Handlers call the auto-imported helpers at module load, so import them after stubbing.
const load = async (path: string) => (await import(path)).default as Handler;
const call = (handler: Handler, event: FakeEvent = {}) => handler(event as unknown as H3Event);

beforeEach(() => {
  fetchMock.mockReset();
});

describe('BFF route handlers', () => {
  it('reports health', async () => {
    expect(call(await load('./health.get'))).toEqual({ status: 'ok', service: 'mentorship' });
  });

  it.each([
    ['./summary.get', 'http://api/v1/summary'],
    ['./mentees/summary.get', 'http://api/v1/mentees/summary'],
    ['./mentors/summary.get', 'http://api/v1/mentors/summary'],
  ])('%s reads %s', async (path, url) => {
    fetchMock.mockResolvedValue({});
    await call(await load(path));
    expect(fetchMock).toHaveBeenCalledWith(url);
  });

  it.each([
    ['./mentees/[id].get', 'Missing mentee id'],
    ['./mentors/[id].get', 'Missing mentor id'],
    ['./programs/[id].get', 'Missing program id'],
    ['./programs/[id]/mentees.get', 'Missing program id'],
    ['./programs/[id]/sponsors.get', 'Missing program id'],
  ])('%s rejects a missing id without calling the API', async (path, message) => {
    await expect(call(await load(path), { params: {} })).rejects.toMatchObject({
      statusCode: 400,
      message,
    });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('maps a mentee and a mentor by id', async () => {
    fetchMock.mockResolvedValue({
      user_id: USER_ID,
      name: 'Ada',
      joined_at: '2026-03-15T12:00:00Z',
    });
    await expect(
      call(await load('./mentees/[id].get'), { params: { id: USER_ID } }),
    ).resolves.toMatchObject({
      id: USER_ID,
      name: 'Ada',
      programs: [],
    });
    await expect(
      call(await load('./mentors/[id].get'), { params: { id: USER_ID } }),
    ).resolves.toMatchObject({
      id: USER_ID,
      name: 'Ada',
      currentMentees: [],
    });
    expect(fetchMock.mock.calls.map((c) => c[0])).toEqual([
      `http://api/v1/mentees/${USER_ID}`,
      `http://api/v1/mentors/${USER_ID}`,
    ]);
  });

  it('maps a program and its mentees and sponsors', async () => {
    fetchMock.mockImplementation(async (url: string) => {
      if (url.includes('/resolve/')) return { id: 'uid-1' };
      if (url.endsWith('/catalog'))
        return { id: 'uid-1', name: 'P', slug: 'p', status: 'published', updated_on: '' };
      if (url.endsWith('/mentees'))
        return { data: [{ user_id: 'e1', status: 'graduated', term_id: 't', term_name: 'T' }] };
      return { data: [{ id: 's1', name: 'ACME', amount_cents: 100 }] };
    });
    const event = { params: { id: 'p' } };
    await expect(call(await load('./programs/[id].get'), event)).resolves.toMatchObject({
      id: 'uid-1',
      slug: 'p',
    });
    await expect(call(await load('./programs/[id]/mentees.get'), event)).resolves.toEqual([
      expect.objectContaining({ id: 'e1', status: 'graduated' }),
    ]);
    await expect(call(await load('./programs/[id]/sponsors.get'), event)).resolves.toEqual([
      expect.objectContaining({ id: 's1', name: 'ACME', amountCents: 100 }),
    ]);
  });

  it('treats a missing mentees or sponsors list as empty', async () => {
    fetchMock.mockImplementation(async (url: string) =>
      url.includes('/resolve/') ? { id: 'uid-1' } : {},
    );
    const event = { params: { id: 'p' } };
    await expect(call(await load('./programs/[id]/mentees.get'), event)).resolves.toEqual([]);
    await expect(call(await load('./programs/[id]/sponsors.get'), event)).resolves.toEqual([]);
  });

  it.each([
    ['./programs/index.get', 'http://api/v1/programs/catalog', { sortBy: 'name_asc' }],
    ['./mentees/index.get', 'http://api/v1/mentees', { status: 'graduated' }],
    ['./mentors/index.get', 'http://api/v1/mentors', {}],
  ])('%s trims the search and parses paging', async (path, url, extra) => {
    fetchMock.mockResolvedValue({ data: [], meta: { total: 9, limit: 5, offset: 0 } });
    const result = await call(await load(path), {
      query: { search: '  ada  ', skill: 'Go', limit: '5', offset: 'later', ...extra },
    });
    expect(result).toMatchObject({ data: [], total: 9 });
    const [calledUrl, { query }] = fetchMock.mock.calls[0]!;
    expect(calledUrl).toBe(url);
    expect(query).toMatchObject({ search: 'ada', skill: 'Go', limit: 5, ...extra });
    // A non-numeric offset falls back to the default.
    expect(query.offset).toBe(0);
  });

  it('lists programs with no query parameters', async () => {
    fetchMock.mockResolvedValue({ data: [], meta: { total: 0, limit: 15, offset: 0 } });
    await expect(call(await load('./programs/index.get'))).resolves.toEqual({
      data: [],
      total: 0,
      foundationCount: 0,
    });
  });
});

describe('sitemap sources', () => {
  it('lists the static pages', async () => {
    expect(call(await load('./seo/sitemap/static'))).toEqual(
      ['/', '/programs', '/programs/enroll', '/mentees', '/mentors'].map((loc) => ({ loc })),
    );
  });

  it('pages through the catalog and skips programs without a slug', async () => {
    const page = (slugs: string[], total: number) => ({
      data: slugs.map((slug, i) => ({
        id: `p${i}`,
        name: slug,
        slug,
        status: 'published',
        updated_on: slug ? '2026-01-01' : '',
      })),
      meta: { total, limit: 100, offset: 0 },
    });
    fetchMock.mockResolvedValueOnce(page(['a', ''], 3)).mockResolvedValueOnce(page(['c'], 3));
    await expect(call(await load('./seo/sitemap/programs'))).resolves.toEqual([
      { loc: '/programs/a', lastmod: '2026-01-01' },
      { loc: '/programs/c', lastmod: '2026-01-01' },
    ]);
    expect(fetchMock.mock.calls.map((c) => c[1].query.offset)).toEqual([0, 2]);
  });

  it('stops on an empty page even if the total says otherwise', async () => {
    fetchMock.mockResolvedValue({ data: [], meta: { total: 50, limit: 100, offset: 0 } });
    await expect(call(await load('./seo/sitemap/programs'))).resolves.toEqual([]);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
