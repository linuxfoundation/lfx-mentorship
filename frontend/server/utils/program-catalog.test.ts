// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { createError } from 'h3';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  fetchProgramCatalog,
  fetchProgramCatalogItem,
  fetchProgramMentees,
  fetchProgramSponsors,
  mapCatalogItemToProgram,
  mapCatalogMentee,
  mapCatalogSponsor,
  type ProgramCatalogItem,
} from './program-catalog';

const fetchMock = vi.fn();
vi.stubGlobal('$fetch', fetchMock);
vi.stubGlobal('useRuntimeConfig', () => ({ apiBaseUrl: 'http://api' }));
vi.stubGlobal('createError', createError);

beforeEach(() => {
  fetchMock.mockReset();
});

const item = (overrides: Partial<ProgramCatalogItem> = {}): ProgramCatalogItem => ({
  id: 'p1',
  name: 'Program',
  slug: 'program',
  status: 'published',
  updated_on: '2026-01-01T00:00:00Z',
  ...overrides,
});

describe('mapCatalogItemToProgram', () => {
  it('fills defaults for absent optional fields', () => {
    const program = mapCatalogItemToProgram(item({ cii_project_id: '', code_of_conduct: '' }));
    expect(program).toMatchObject({
      id: 'p1',
      slug: 'program',
      description: '',
      skills: [],
      terms: [],
      mentors: [],
      mentees: [],
      sponsors: [],
      activeTerms: [],
      status: 'completed',
      ciiProjectId: undefined,
      codeOfConduct: undefined,
    });
  });

  it('keeps a pending program pending whatever its terms say', () => {
    expect(mapCatalogItemToProgram(item({ status: 'pending' })).status).toBe('pending');
  });

  it('maps terms, trimming dates and normalising unknown statuses to open', () => {
    const program = mapCatalogItemToProgram(
      item({
        terms: [
          {
            id: 't1',
            program_id: 'p1',
            name: 'Fall',
            status: 'draft',
            start_date_time: '2026-09-01T00:00:00Z',
            end_date_time: '2026-11-30T00:00:00Z',
            application_start_date: '2026-06-01T00:00:00Z',
          },
          { id: 't2', program_id: 'p1', name: 'Old', status: 'deleted' },
        ],
      }),
    );
    expect(program.terms).toEqual([
      {
        id: 't1',
        name: 'Fall',
        status: 'open',
        startsAt: '2026-09-01',
        endsAt: '2026-11-30',
        applicationsStartAt: '2026-06-01T00:00:00Z',
        applicationsCloseAt: undefined,
      },
      expect.objectContaining({ id: 't2', status: 'deleted', startsAt: '', endsAt: '' }),
    ]);
    // An open term with no application window is still running.
    expect(program.status).toBe('in-progress');
  });

  it('maps mentors with fallbacks for blank names and intros', () => {
    const program = mapCatalogItemToProgram(
      item({
        mentors: [
          { id: 'm1', user_id: 'u1', name: '  Ada  ', introduction: ' Hi ', avatar_url: 'a.png' },
          { id: 'm2', user_id: 'u2', name: ' ', introduction: ' ' },
        ],
      }),
    );
    expect(program.mentors).toEqual([
      { id: 'u1', name: 'Ada', intro: 'Hi', avatarUrl: 'a.png' },
      { id: 'u2', name: 'Mentor', intro: undefined, avatarUrl: undefined },
    ]);
  });
});

describe('mapCatalogMentee and mapCatalogSponsor', () => {
  it('maps a mentee, treating anything but graduated as active', () => {
    const base = { user_id: 'u1', term_id: 't1', term_name: 'Fall' };
    expect(mapCatalogMentee({ ...base, status: 'graduated', name: ' Bo ' })).toEqual({
      id: 'u1',
      name: 'Bo',
      avatarUrl: undefined,
      intro: undefined,
      status: 'graduated',
      termId: 't1',
      termLabel: 'Fall',
    });
    expect(mapCatalogMentee({ ...base, status: 'accepted' })).toMatchObject({
      name: 'Mentee',
      status: 'active',
    });
  });

  it('maps a sponsor with fallbacks', () => {
    expect(mapCatalogSponsor({ id: 's1', name: ' ', amount_cents: 500 })).toEqual({
      id: 's1',
      name: 'Sponsor',
      logoUrl: undefined,
      amountCents: 500,
    });
    expect(
      mapCatalogSponsor({ id: 's2', name: 'ACME', amount_cents: undefined as unknown as number })
        .amountCents,
    ).toBe(0);
  });
});

describe('fetchProgramCatalog', () => {
  it('drops empty and "all" filters and applies paging defaults', async () => {
    fetchMock.mockResolvedValue({ data: [], meta: { total: 0, limit: 15, offset: 0 } });
    await fetchProgramCatalog({ search: '', skill: 'all', status: 'all' });
    expect(fetchMock).toHaveBeenCalledWith('http://api/v1/programs/catalog', {
      query: {
        search: undefined,
        skill: undefined,
        status: undefined,
        sortBy: undefined,
        limit: 15,
        offset: 0,
      },
    });
  });

  it('passes real filters through', async () => {
    fetchMock.mockResolvedValue({ data: [], meta: { total: 0, limit: 5, offset: 10 } });
    await fetchProgramCatalog({
      search: 'k8s',
      skill: 'Go',
      status: 'acceptance',
      sortBy: 'name_asc',
      limit: 5,
      offset: 10,
    });
    expect(fetchMock.mock.calls[0]![1]).toEqual({
      query: {
        search: 'k8s',
        skill: 'Go',
        status: 'acceptance',
        sortBy: 'name_asc',
        limit: 5,
        offset: 10,
      },
    });
  });

  it.each([
    ['an upstream status', { statusCode: 503 }, 503],
    ['a network error', new Error('ECONNREFUSED'), 502],
    ['a nonsense status', { statusCode: 'abc' }, 502],
  ])('maps %s to a stable error', async (_label, upstream, status) => {
    fetchMock.mockRejectedValue(upstream);
    await expect(fetchProgramCatalog({})).rejects.toMatchObject({
      statusCode: status,
      message: 'Failed to load programs',
    });
  });
});

describe('program lookups by slug or id', () => {
  it.each([
    ['fetchProgramCatalogItem', fetchProgramCatalogItem, '/catalog', undefined],
    ['fetchProgramMentees', fetchProgramMentees, '/mentees', undefined],
    [
      'fetchProgramSponsors',
      fetchProgramSponsors,
      '/sponsors',
      { query: { categoryType: 'mentorship', aggregate: true } },
    ],
  ] as const)(
    '%s resolves the program before reading %s',
    async (_name, fetcher, suffix, options) => {
      fetchMock.mockResolvedValueOnce({ id: 'uid-1' }).mockResolvedValueOnce({ data: [] });
      await fetcher('my program/x');
      expect(fetchMock.mock.calls[0]![0]).toBe('http://api/v1/programs/resolve/my%20program%2Fx');
      expect(fetchMock.mock.calls[1]).toEqual(
        options
          ? [`http://api/v1/programs/uid-1${suffix}`, options]
          : [`http://api/v1/programs/uid-1${suffix}`],
      );
    },
  );

  it.each([
    ['fetchProgramCatalogItem', fetchProgramCatalogItem, 'Failed to load program'],
    ['fetchProgramMentees', fetchProgramMentees, 'Failed to load mentees'],
    ['fetchProgramSponsors', fetchProgramSponsors, 'Failed to load sponsors'],
  ] as const)(
    '%s reports a missing program and other failures distinctly',
    async (_name, fetcher, failure) => {
      fetchMock.mockRejectedValueOnce({ statusCode: 404 });
      await expect(fetcher('missing')).rejects.toMatchObject({
        statusCode: 404,
        message: 'Program not found',
      });

      fetchMock.mockResolvedValueOnce({ id: 'uid-1' }).mockRejectedValueOnce({ statusCode: 500 });
      await expect(fetcher('broken')).rejects.toMatchObject({ statusCode: 500, message: failure });
    },
  );
});
