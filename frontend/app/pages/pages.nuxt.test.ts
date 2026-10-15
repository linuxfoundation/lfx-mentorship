// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { mountSuspended, registerEndpoint } from '@nuxt/test-utils/runtime';
import { getQuery } from 'h3';
import { describe, expect, it, vi } from 'vitest';
import App from '~/app.vue';
import type { MenteeDetail, MenteesListResponse } from '~/types/mentee.types';
import type { MentorDetail, MentorsListResponse } from '~/types/mentor.types';
import type { Program, ProgramsListResponse } from '~/types/program.types';

const day = 24 * 60 * 60 * 1000;
const iso = (offsetDays: number) => new Date(Date.now() + offsetDays * day).toISOString();

const program: Program = {
  id: 'p1',
  slug: 'cloud-native',
  name: 'Cloud Native Mentorship',
  description: '<p>Learn <strong>Kubernetes</strong> internals.</p>',
  logoUrl: 'https://example.org/logo.png',
  skills: ['Go', 'Kubernetes'],
  status: 'acceptance',
  foundation: { id: '', name: '', slug: '' },
  terms: [
    {
      id: 't1',
      name: 'Fall 2026',
      status: 'open',
      dateRangeLabel: 'Sep-Nov 2026',
      startsAt: '2026-09-01',
      endsAt: '2026-11-30',
      applicationsStartAt: iso(-5),
      applicationsCloseAt: iso(5),
    },
    {
      id: 't0',
      name: 'Spring 2025',
      status: 'closed',
      startsAt: '2025-03-01',
      endsAt: '2025-05-31',
    },
  ],
  activeTerms: [],
  updatedAt: '2026-10-01T00:00:00Z',
  repositoryUrl: 'https://github.com/example/repo',
  codeOfConduct: 'https://example.org/coc',
  mentees: [],
  mentors: [{ id: 'm1', name: 'Grace Hopper', intro: 'Compiler person' }],
  sponsors: [],
  isPaid: true,
};
program.activeTerms = [program.terms[0]!];

const menteeDetail: MenteeDetail = {
  id: 'u1',
  name: 'Ada Lovelace',
  introduction: 'Analytical engines enthusiast',
  skills: ['Go'],
  status: 'active',
  sinceLabel: 'Since Mar. 2026',
  joinedAt: '2026-03-15T00:00:00Z',
  program: { id: 'p1', name: 'Cloud Native Mentorship', foundationLabel: '' },
  mentors: [{ id: 'm1', name: 'Grace Hopper' }],
  githubUrl: 'https://github.com/ada',
  linkedinUrl: 'https://linkedin.com/in/ada',
  programs: [
    {
      id: 'p1',
      title: 'Cloud Native Mentorship',
      description: 'desc',
      foundationLabel: '',
      status: 'active',
      skills: ['Go'],
      terms: [{ id: 't1', label: 'Fall 2026', dateRangeLabel: 'Sep 2026 - Nov 2026' }],
      mentors: [{ id: 'm1', name: 'Grace Hopper' }],
      logoInitials: 'CN',
    },
  ],
};

const mentorDetail: MentorDetail = {
  id: 'm1',
  name: 'Grace Hopper',
  bio: 'Compiler person',
  skills: ['COBOL'],
  sinceLabel: 'Since Jan. 2026',
  joinedAt: '2026-01-15T00:00:00Z',
  affiliationsLabel: 'Cloud Native Mentorship',
  githubUrl: 'https://github.com/grace',
  stats: { programsMentoring: 1, currentMentees: 1, menteesGraduated: 2 },
  programs: menteeDetail.programs,
  currentMentees: [
    { id: 'u1', name: 'Ada Lovelace', bio: 'Engines', programLabel: 'Cloud Native · Fall 2026' },
  ],
  graduatedMentees: [
    { id: 'u2', name: 'Alan Turing', bio: '', programLabel: 'Cloud Native · Spring 2025' },
  ],
};

const programs: ProgramsListResponse = { data: [program], total: 1, foundationCount: 0 };
const mentees: MenteesListResponse = { data: [menteeDetail], total: 1 };
const mentors: MentorsListResponse = { data: [mentorDetail], total: 1 };

registerEndpoint('/api/summary', () => ({
  programCount: 12,
  acceptingProgramCount: 3,
  mentorCount: 40,
  graduatedMenteeCount: 99,
  foundationCount: 5,
  stipendsPaid: 1300000,
  graduatedMentees: [{ name: 'Ada Lovelace' }, {}],
}));
registerEndpoint('/api/programs', () => programs);
registerEndpoint('/api/programs/p1', () => program);
registerEndpoint('/api/programs/p1/mentees', () => [
  { id: 'u1', name: 'Ada Lovelace', status: 'active', termId: 't1', termLabel: 'Fall 2026' },
  { id: 'u2', name: 'Alan Turing', status: 'graduated', termId: 't0', termLabel: 'Spring 2025' },
]);
registerEndpoint('/api/programs/p1/sponsors', () => [
  { id: 's1', name: 'ACME Corp', amountCents: 500000 },
]);
registerEndpoint('/api/mentees', () => mentees);
registerEndpoint('/api/mentees/summary', () => ({ menteeCount: 1, programCount: 1 }));
registerEndpoint('/api/mentees/u1', () => menteeDetail);
registerEndpoint('/api/mentors', (event) =>
  getQuery(event).search === 'nobody' ? { data: [], total: 0 } : mentors,
);
registerEndpoint('/api/mentors/summary', () => ({ mentorCount: 1, programCount: 1 }));
registerEndpoint('/api/mentors/m1', () => mentorDetail);

const render = (route: string) => mountSuspended(App, { route });

describe('public pages', () => {
  it('renders the landing page with live counts', async () => {
    const page = await render('/');
    await vi.waitFor(() => expect(page.text()).toContain('40'));
    expect(page.find('header').exists()).toBe(true);
    expect(page.find('footer').exists()).toBe(true);
  });

  it('renders the program directory', async () => {
    const page = await render('/programs');
    await vi.waitFor(() => expect(page.text()).toContain('Cloud Native Mentorship'));
    expect(page.text()).toContain('Go');
  });

  it('renders the program enrollment landing page', async () => {
    const page = await render('/programs/enroll');
    expect(page.text().length).toBeGreaterThan(0);
  });

  it('renders a program with its terms, mentors, mentees, and sponsors', async () => {
    const page = await render('/programs/p1');
    await vi.waitFor(() => expect(page.text()).toContain('Cloud Native Mentorship'));
    expect(page.text()).toContain('Kubernetes');
    expect(page.html()).not.toContain('<script>');
  });

  it('renders the mentee directory and a mentee profile', async () => {
    const list = await render('/mentees');
    await vi.waitFor(() => expect(list.text()).toContain('Ada Lovelace'));
    const detail = await render('/mentees/u1');
    await vi.waitFor(() => expect(detail.text()).toContain('Analytical engines enthusiast'));
    expect(detail.text()).toContain('Cloud Native Mentorship');
  });

  it('renders the mentor directory and a mentor profile', async () => {
    const list = await render('/mentors');
    await vi.waitFor(() => expect(list.text()).toContain('Grace Hopper'));
    const detail = await render('/mentors/m1');
    await vi.waitFor(() => expect(detail.text()).toContain('Compiler person'));
    expect(detail.text()).toContain('Alan Turing');
  });
});
