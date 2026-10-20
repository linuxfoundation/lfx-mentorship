// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { createError } from 'h3';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  fetchMentor,
  fetchMentors,
  fetchMentorSummary,
  mapMentorDetail,
  mapMentorItem,
  toMentorsListResponse,
  type MentorItem,
} from './mentor';

const USER_ID = '2f1c9a7e-5b3d-4c8e-9f0a-1b2c3d4e5f60';
const fetchMock = vi.fn();
vi.stubGlobal('$fetch', fetchMock);
vi.stubGlobal('useRuntimeConfig', () => ({ apiBaseUrl: 'http://api' }));
vi.stubGlobal('createError', createError);

beforeEach(() => {
  fetchMock.mockReset();
});

const item = (overrides: Partial<MentorItem> = {}): MentorItem => ({
  user_id: 'u1',
  joined_at: '2026-03-15T12:00:00Z',
  ...overrides,
});

describe('mapMentorItem', () => {
  it('fills fallbacks for a sparse mentor', () => {
    expect(mapMentorItem(item({ joined_at: 'not-a-date' }))).toEqual({
      id: 'u1',
      name: 'Mentor',
      bio: '',
      skills: [],
      sinceLabel: '',
      joinedAt: 'not-a-date',
      avatarUrl: undefined,
    });
  });

  it('trims and maps a full mentor', () => {
    expect(
      mapMentorItem(
        item({ name: ' Ada ', introduction: ' Hi ', skills: ['Go'], avatar_url: 'a.png' }),
      ),
    ).toEqual({
      id: 'u1',
      name: 'Ada',
      bio: 'Hi',
      skills: ['Go'],
      sinceLabel: 'Since Mar. 2026',
      joinedAt: '2026-03-15T12:00:00Z',
      avatarUrl: 'a.png',
    });
  });
});

describe('mapMentorDetail', () => {
  it('defaults stats and lists when absent', () => {
    expect(mapMentorDetail(item())).toMatchObject({
      affiliationsLabel: '',
      stats: { programsMentoring: 0, currentMentees: 0, menteesGraduated: 0 },
      programs: [],
      currentMentees: [],
      graduatedMentees: [],
    });
  });

  it('maps programs, mentees, and stats', () => {
    const detail = mapMentorDetail({
      ...item(),
      github_url: 'https://github.com/ada',
      stats: { programs_mentoring: 2, current_mentees: 3, mentees_graduated: 4 },
      programs: [
        {
          id: 'p1',
          name: 'Cloud Native',
          slug: 'cn',
          mentors: [{ id: 'm1', user_id: 'mu1', name: ' ', introduction: ' Co-mentor ' }],
          terms: [
            {
              id: 't1',
              name: 'Fall',
              start_date_time: '2026-09-15T00:00:00Z',
              end_date_time: '2026-11-15T00:00:00Z',
            },
            { id: 't2', name: 'Old', status: 'closed' },
          ],
        },
        { id: 'p2', name: 'Kubernetes', slug: 'k8s', description: 'desc', skills: ['Go'] },
      ],
      current_mentees: [
        {
          user_id: 'e1',
          name: ' Bo ',
          introduction: ' Bio ',
          program_name: 'Cloud Native',
          term_name: 'Fall',
          status: 'accepted',
        },
      ],
      graduated_mentees: [
        { user_id: 'e2', program_name: 'Kubernetes', term_name: '', status: 'graduated' },
      ],
    });
    expect(detail.affiliationsLabel).toBe('Cloud Native, Kubernetes');
    expect(detail.githubUrl).toBe('https://github.com/ada');
    expect(detail.stats).toEqual({ programsMentoring: 2, currentMentees: 3, menteesGraduated: 4 });
    expect(detail.programs[0]).toMatchObject({
      title: 'Cloud Native',
      description: '',
      logoInitials: 'CN',
      // The open Fall term has no application window, so the program is running.
      status: 'in-progress',
      terms: [
        { id: 't1', label: 'Fall', dateRangeLabel: 'Sep 2026 - Nov 2026' },
        { id: 't2', label: 'Old', dateRangeLabel: '' },
      ],
      mentors: [{ id: 'mu1', name: 'Mentor', introduction: 'Co-mentor', avatarUrl: undefined }],
    });
    expect(detail.programs[1]).toMatchObject({
      status: 'completed',
      skills: ['Go'],
      logoInitials: 'KU',
    });
    expect(detail.currentMentees).toEqual([
      {
        id: 'e1',
        name: 'Bo',
        bio: 'Bio',
        programLabel: 'Cloud Native · Fall',
        avatarUrl: undefined,
      },
    ]);
    expect(detail.graduatedMentees).toEqual([
      { id: 'e2', name: 'Mentee', bio: '', programLabel: 'Kubernetes', avatarUrl: undefined },
    ]);
  });
});

describe('mentor fetchers', () => {
  it('drops empty and "all" filters and applies paging defaults', async () => {
    fetchMock.mockResolvedValue({ data: [], meta: { total: 0, limit: 20, offset: 0 } });
    await fetchMentors({ search: '', skill: 'all' });
    expect(fetchMock).toHaveBeenCalledWith('http://api/v1/mentors', {
      query: { search: undefined, skill: undefined, limit: 20, offset: 0 },
    });
    await fetchMentors({ search: 'ada', skill: 'Go', limit: 5, offset: 10 });
    expect(fetchMock.mock.calls[1]![1]).toEqual({
      query: { search: 'ada', skill: 'Go', limit: 5, offset: 10 },
    });
  });

  it('converts a page for the client', () => {
    expect(
      toMentorsListResponse({ data: [item()], meta: { total: 4, limit: 1, offset: 0 } }),
    ).toMatchObject({
      total: 4,
      data: [{ id: 'u1' }],
    });
  });

  it('maps the summary', async () => {
    fetchMock.mockResolvedValue({ mentor_count: 5, program_count: 2 });
    await expect(fetchMentorSummary()).resolves.toEqual({ mentorCount: 5, programCount: 2 });
  });

  it.each([
    ['list', () => fetchMentors({}), { statusCode: 503 }, 503, 'Failed to load mentors'],
    [
      'summary',
      () => fetchMentorSummary(),
      new Error('down'),
      502,
      'Failed to load mentor summary',
    ],
    ['missing mentor', () => fetchMentor(USER_ID), { statusCode: 404 }, 404, 'Mentor not found'],
    ['mentor', () => fetchMentor(USER_ID), { statusCode: 500 }, 500, 'Failed to load mentor'],
  ])('maps a %s failure', async (_label, call, upstream, statusCode, message) => {
    fetchMock.mockRejectedValue(upstream);
    await expect(call()).rejects.toMatchObject({ statusCode, message });
  });
});
