// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { mountSuspended, registerEndpoint } from '@nuxt/test-utils/runtime';
import { useQueryClient } from '@tanstack/vue-query';
import { getQuery } from 'h3';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h, ref } from 'vue';
import { useMentee } from './mentees/useMentee';
import { useMentees } from './mentees/useMentees';
import { useMenteesSummary } from './mentees/useMenteesSummary';
import { useMentor } from './mentors/useMentor';
import { useMentors } from './mentors/useMentors';
import { useMentorsSummary } from './mentors/useMentorsSummary';
import { useProgram } from './programs/useProgram';
import { useProgramMentees } from './programs/useProgramMentees';
import { useProgramSponsors } from './programs/useProgramSponsors';
import { usePrograms } from './programs/usePrograms';
import { useLandingSummary } from './useLandingSummary';

let clear: (() => void) | undefined;

/** Runs a composable inside a mounted component, as a page would. */
async function withSetup<T>(composable: () => T): Promise<T> {
  let result!: T;
  await mountSuspended(
    defineComponent({
      setup() {
        result = composable();
        const client = useQueryClient();
        clear = () => client.clear();
        return () => h('div');
      },
    }),
  );
  return result;
}

afterEach(() => {
  clear?.();
});

describe('single-resource queries', () => {
  it.each([
    ['useProgram', (id: string) => useProgram(id), '/api/programs/p1', { id: 'p1' }],
    [
      'useProgramMentees',
      (id: string) => useProgramMentees(id),
      '/api/programs/p1/mentees',
      [{ id: 'u1' }],
    ],
    [
      'useProgramSponsors',
      (id: string) => useProgramSponsors(id),
      '/api/programs/p1/sponsors',
      { sponsors: [{ id: 's1' }], crowdfundingInitiative: true },
    ],
    ['useMentee', (id: string) => useMentee(id), '/api/mentees/p1', { id: 'p1' }],
    ['useMentor', (id: string) => useMentor(id), '/api/mentors/p1', { id: 'p1' }],
  ] as const)('%s reads %s', async (_name, composable, url, body) => {
    registerEndpoint(url, () => body);
    const query = await withSetup(() => composable('p1'));
    await vi.waitFor(() => expect(query.data.value).toEqual(body));
  });

  it.each([
    ['useProgram', () => useProgram('')],
    ['useProgramMentees', () => useProgramMentees('')],
    ['useProgramSponsors', () => useProgramSponsors('')],
    ['useMentee', () => useMentee('')],
    ['useMentor', () => useMentor('')],
  ])('%s waits for an id', async (_name, composable) => {
    const query = await withSetup(composable);
    expect(query.fetchStatus.value).toBe('idle');
    expect(query.data.value).toBeUndefined();
  });

  it('fetches program mentees and sponsors only when enabled', async () => {
    const enabled = ref(false);
    registerEndpoint('/api/programs/p2/mentees', () => [{ id: 'u2' }]);
    const query = await withSetup(() => useProgramMentees('p2', enabled));
    expect(query.fetchStatus.value).toBe('idle');
    enabled.value = true;
    await vi.waitFor(() => expect(query.data.value).toEqual([{ id: 'u2' }]));
  });

  it('refetches when the id changes', async () => {
    registerEndpoint('/api/mentors/a', () => ({ id: 'a' }));
    registerEndpoint('/api/mentors/b', () => ({ id: 'b' }));
    const id = ref('a');
    const query = await withSetup(() => useMentor(id));
    await vi.waitFor(() => expect(query.data.value).toEqual({ id: 'a' }));
    id.value = 'b';
    await vi.waitFor(() => expect(query.data.value).toEqual({ id: 'b' }));
  });

  it('surfaces an API error', async () => {
    registerEndpoint('/api/programs/missing', () => {
      throw createError({ statusCode: 404, message: 'Program not found' });
    });
    const query = await withSetup(() => useProgram('missing'));
    // The app retries once before giving up.
    await vi.waitFor(() => expect(query.isError.value).toBe(true), { timeout: 3000 });
  });
});

describe('summary queries', () => {
  it.each([
    ['useLandingSummary', useLandingSummary, '/api/summary', { programCount: 3 }],
    ['useMenteesSummary', useMenteesSummary, '/api/mentees/summary', { menteeCount: 4 }],
    ['useMentorsSummary', useMentorsSummary, '/api/mentors/summary', { mentorCount: 5 }],
  ] as const)('%s reads %s', async (_name, composable, url, body) => {
    registerEndpoint(url, () => body);
    const query = await withSetup(composable);
    await vi.waitFor(() => expect(query.data.value).toEqual(body));
  });
});

describe('paged directory queries', () => {
  it('pages programs with the filters until the total is loaded', async () => {
    const seen: Record<string, unknown>[] = [];
    registerEndpoint('/api/programs', (event) => {
      const query = getQuery(event);
      seen.push(query);
      return Number(query.offset) === 0
        ? { data: [{ id: 'a' }, { id: 'b' }], total: 3 }
        : { data: [{ id: 'c' }], total: 3 };
    });
    const programs = await withSetup(() =>
      usePrograms({
        search: ref(''),
        status: ref('all'),
        skill: ref('Go'),
        sortBy: ref('name_asc'),
      }),
    );
    await vi.waitFor(() => expect(programs.data.value?.pages).toHaveLength(1));
    expect(seen[0]).toMatchObject({ status: 'all', skill: 'Go', sortBy: 'name_asc', offset: '0' });
    expect(seen[0]).not.toHaveProperty('search');
    expect(programs.hasNextPage.value).toBe(true);

    await programs.fetchNextPage();
    expect(seen[1]).toMatchObject({ offset: '2' });
    expect(programs.hasNextPage.value).toBe(false);
  });

  it('pages mentees with the filters', async () => {
    const seen: Record<string, unknown>[] = [];
    registerEndpoint('/api/mentees', (event) => {
      seen.push(getQuery(event));
      return { data: [{ id: 'a' }], total: 1 };
    });
    const mentees = await withSetup(() =>
      useMentees({ search: ref('ada'), status: ref('graduated'), skill: ref('all') }),
    );
    await vi.waitFor(() => expect(mentees.data.value?.pages).toHaveLength(1));
    expect(seen[0]).toMatchObject({
      search: 'ada',
      status: 'graduated',
      skill: 'all',
      offset: '0',
    });
    expect(mentees.hasNextPage.value).toBe(false);
  });

  it('pages mentors and refetches when a filter changes', async () => {
    const seen: Record<string, unknown>[] = [];
    registerEndpoint('/api/mentors', (event) => {
      seen.push(getQuery(event));
      return { data: [], total: 0 };
    });
    const skill = ref('all');
    const mentors = await withSetup(() => useMentors({ search: ref(''), skill }));
    await vi.waitFor(() => expect(seen).toHaveLength(1));
    skill.value = 'Rust';
    await vi.waitFor(() => expect(seen).toHaveLength(2));
    expect(seen[1]).toMatchObject({ skill: 'Rust', offset: '0' });
    expect(mentors.hasNextPage.value).toBe(false);
  });
});
