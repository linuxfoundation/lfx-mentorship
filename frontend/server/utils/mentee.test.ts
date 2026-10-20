// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { createError } from 'h3';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  fetchMentee,
  fetchMentees,
  fetchMenteeSummary,
  mapMenteeDetail,
  mapMenteeItem,
  toMenteesListResponse,
  type MenteeItem,
} from './mentee';

const USER_ID = '2f1c9a7e-5b3d-4c8e-9f0a-1b2c3d4e5f60';
const fetchMock = vi.fn();
vi.stubGlobal('$fetch', fetchMock);
vi.stubGlobal('useRuntimeConfig', () => ({ apiBaseUrl: 'http://api' }));
vi.stubGlobal('createError', createError);

beforeEach(() => {
  fetchMock.mockReset();
});

const item = (overrides: Partial<MenteeItem> = {}): MenteeItem => ({
  user_id: 'u1',
  joined_at: '2026-03-15T12:00:00Z',
  ...overrides,
});

describe('mapMenteeItem', () => {
  it('fills fallbacks for a sparse mentee', () => {
    expect(mapMenteeItem(item({ joined_at: 'not-a-date' }))).toEqual({
      id: 'u1',
      name: 'Mentee',
      introduction: '',
      skills: [],
      status: undefined,
      sinceLabel: '',
      joinedAt: 'not-a-date',
      program: { id: '', name: '', foundationLabel: '' },
      mentors: [],
      avatarUrl: undefined,
    });
  });

  it('maps a full mentee', () => {
    const mentee = mapMenteeItem(
      item({
        name: ' Ada ',
        introduction: ' Hello ',
        skills: ['Go'],
        status: 'accepted',
        avatar_url: 'a.png',
        program: { id: 'p1', name: 'Program', slug: 'program' },
        mentors: [{ id: 'm1', user_id: 'mu1', name: ' ', introduction: ' ' }],
      }),
    );
    expect(mentee).toMatchObject({
      name: 'Ada',
      introduction: 'Hello',
      skills: ['Go'],
      status: 'active',
      sinceLabel: 'Since Mar. 2026',
      program: { id: 'p1', name: 'Program', foundationLabel: '' },
      mentors: [{ id: 'mu1', name: 'Mentor', introduction: undefined, avatarUrl: undefined }],
      avatarUrl: 'a.png',
    });
  });

  it.each([
    ['active', 'active'],
    ['accepted', 'active'],
    ['graduated', 'graduated'],
    ['withdrawn', undefined],
    ['', undefined],
  ])('maps status %j to %j', (status, want) => {
    expect(mapMenteeItem(item({ status })).status).toBe(want);
  });
});

describe('mapMenteeDetail', () => {
  it('maps profile links and programs', () => {
    const detail = mapMenteeDetail({
      ...item(),
      github_url: 'https://github.com/ada',
      linkedin_url: 'https://linkedin.com/in/ada',
      programs: [
        {
          id: 'p1',
          name: 'Cloud Native Program',
          slug: 'cn',
          status: 'graduated',
          terms: [
            {
              id: 't1',
              name: 'Fall',
              start_date_time: '2026-09-15T00:00:00Z',
              end_date_time: '2026-11-15T00:00:00Z',
              application_status: 'accepted',
            },
            {
              id: 't2',
              name: 'Spring',
              start_date_time: '2027-03-15T00:00:00Z',
              application_status: 'accepted',
            },
            { id: 't3', name: 'Undated', start_date_time: 'bad', application_status: 'accepted' },
          ],
        },
        {
          id: 'p2',
          name: 'k8s',
          slug: 'k8s',
          status: 'active',
          description: 'desc',
          skills: ['Go'],
        },
      ],
    });
    expect(detail.githubUrl).toBe('https://github.com/ada');
    expect(detail.linkedinUrl).toBe('https://linkedin.com/in/ada');
    expect(detail.programs[0]).toMatchObject({
      id: 'p1',
      title: 'Cloud Native Program',
      description: '',
      status: 'graduated',
      skills: [],
      logoInitials: 'CN',
      terms: [
        { id: 't1', label: 'Fall', dateRangeLabel: 'Sep 2026 - Nov 2026' },
        { id: 't2', label: 'Spring', dateRangeLabel: 'Mar 2027' },
        { id: 't3', label: 'Undated', dateRangeLabel: '' },
      ],
    });
    expect(detail.programs[1]).toMatchObject({
      status: 'active',
      description: 'desc',
      logoInitials: 'K8',
    });
  });

  it('defaults to no programs', () => {
    expect(mapMenteeDetail(item()).programs).toEqual([]);
  });
});

describe('mentee fetchers', () => {
  it('drops empty and "all" filters and applies paging defaults', async () => {
    fetchMock.mockResolvedValue({ data: [], meta: { total: 0, limit: 20, offset: 0 } });
    await fetchMentees({ search: '', skill: 'all', status: 'all' });
    expect(fetchMock).toHaveBeenCalledWith('http://api/v1/mentees', {
      query: { search: undefined, skill: undefined, status: undefined, limit: 20, offset: 0 },
    });
    await fetchMentees({ search: 'ada', skill: 'Go', status: 'graduated', limit: 5, offset: 10 });
    expect(fetchMock.mock.calls[1]![1]).toEqual({
      query: { search: 'ada', skill: 'Go', status: 'graduated', limit: 5, offset: 10 },
    });
  });

  it('converts a page for the client', () => {
    expect(
      toMenteesListResponse({ data: [item()], meta: { total: 7, limit: 1, offset: 0 } }),
    ).toMatchObject({
      total: 7,
      data: [{ id: 'u1' }],
    });
  });

  it('maps the summary', async () => {
    fetchMock.mockResolvedValue({ mentee_count: 3, program_count: 2 });
    await expect(fetchMenteeSummary()).resolves.toEqual({ menteeCount: 3, programCount: 2 });
    expect(fetchMock).toHaveBeenCalledWith('http://api/v1/mentees/summary');
  });

  it.each([
    ['list', () => fetchMentees({}), { statusCode: 503 }, 503, 'Failed to load mentees'],
    [
      'summary',
      () => fetchMenteeSummary(),
      new Error('down'),
      502,
      'Failed to load mentee summary',
    ],
    ['missing mentee', () => fetchMentee(USER_ID), { statusCode: 404 }, 404, 'Mentee not found'],
    ['mentee', () => fetchMentee(USER_ID), { statusCode: 500 }, 500, 'Failed to load mentee'],
  ])('maps a %s failure', async (_label, call, upstream, statusCode, message) => {
    fetchMock.mockRejectedValue(upstream);
    await expect(call()).rejects.toMatchObject({ statusCode, message });
  });
});
