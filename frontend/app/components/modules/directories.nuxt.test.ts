// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { mountSuspended, registerEndpoint } from '@nuxt/test-utils/runtime';
import { useQueryClient } from '@tanstack/vue-query';
import { getQuery } from 'h3';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h } from 'vue';
import MenteesView from './mentees/view/mentees.vue';
import MentorsView from './mentors/view/mentors.vue';
import ProgramsView from './programs/view/programs.vue';

type Mode = 'ok' | 'empty' | 'error' | 'fail-next';

const program = (n: number) => ({
  id: `p${n}`,
  slug: `p${n}`,
  name: `Program ${n}`,
  description: '',
  skills: [],
  status: 'completed',
  foundation: { id: '', name: '', slug: '' },
  terms: [],
  activeTerms: [],
  updatedAt: '',
  mentees: [],
  mentors: [],
  sponsors: [],
});
const mentee = (n: number) => ({
  id: `u${n}`,
  name: `Mentee ${n}`,
  introduction: '',
  skills: [],
  sinceLabel: '',
  joinedAt: '',
  program: { id: '', name: '', foundationLabel: '' },
  mentors: [],
});
const mentor = (n: number) => ({
  id: `m${n}`,
  name: `Mentor ${n}`,
  bio: '',
  skills: [],
  sinceLabel: '',
  joinedAt: '',
});

const directories = [
  { name: 'programs', view: ProgramsView, item: program, label: 'Program' },
  { name: 'mentees', view: MenteesView, item: mentee, label: 'Mentee' },
  { name: 'mentors', view: MentorsView, item: mentor, label: 'Mentor' },
] as const;

let mode: Mode = 'ok';
const requests: Record<string, Record<string, unknown>[]> = {
  programs: [],
  mentees: [],
  mentors: [],
};

for (const { name, item } of directories) {
  registerEndpoint(`/api/${name}`, (event) => {
    const query = getQuery(event);
    requests[name]!.push(query);
    const offset = Number(query.offset);
    if (mode === 'error' || (mode === 'fail-next' && offset > 0)) {
      throw createError({ statusCode: 500, message: `${name} backend down` });
    }
    if (mode === 'empty') return { data: [], total: 0, foundationCount: 0 };
    // Two items per page out of three, so the first page always leaves more to load.
    const items = offset === 0 ? [item(1), item(2)] : [item(3)];
    return { data: items, total: 3, foundationCount: 0 };
  });
}

afterEach(async () => {
  mode = 'ok';
  for (const key of Object.keys(requests)) requests[key] = [];
  // The Nuxt app, and so its query cache, is shared by every test in the file.
  await mountSuspended(
    defineComponent({
      setup() {
        useQueryClient().clear();
        return () => h('div');
      },
    }),
  );
});

const loadMore = (wrapper: Awaited<ReturnType<typeof mountSuspended>>) =>
  wrapper.findAll('button').find((b) => b.text() === 'Load More');

describe.each(directories)('$name directory', ({ name, view, label }) => {
  it('lists the first page, then loads more until everything is shown', async () => {
    const wrapper = await mountSuspended(view);
    await vi.waitFor(() => expect(wrapper.text()).toContain(`${label} 2`));
    expect(requests[name]![0]).toMatchObject({ offset: '0' });

    await loadMore(wrapper)!.trigger('click');
    await vi.waitFor(() => expect(wrapper.text()).toContain(`${label} 3`));
    expect(requests[name]!.at(-1)).toMatchObject({ offset: '2' });
    await vi.waitFor(() => expect(loadMore(wrapper)).toBeUndefined());
  });

  it('searches after the user stops typing', async () => {
    const wrapper = await mountSuspended(view);
    await vi.waitFor(() => expect(requests[name]).toHaveLength(1));
    await wrapper.get('input').setValue(`find-${name}`);
    await vi.waitFor(
      () => expect(requests[name]!.at(-1)).toMatchObject({ search: `find-${name}` }),
      { timeout: 2000 },
    );
  });

  it('says so when nothing matches', async () => {
    mode = 'empty';
    const wrapper = await mountSuspended(view);
    await wrapper.get('input').setValue(`nothing-${name}`);
    await vi.waitFor(() => expect(wrapper.text()).toContain(`No ${name} found.`), {
      timeout: 2000,
    });
  });

  it('shows an error state when the first page fails', async () => {
    mode = 'error';
    const wrapper = await mountSuspended(view);
    await wrapper.get('input').setValue(`broken-${name}`);
    await vi.waitFor(
      () => expect(wrapper.text()).toContain(`Unable to load ${name}. Please try again.`),
      { timeout: 4000 },
    );
  });

  it('keeps the loaded items when a later page fails, and the retry loads it', async () => {
    mode = 'fail-next';
    const search = `retry-${name}`;
    const forSearch = () => requests[name]!.filter((q) => q.search === search);
    const wrapper = await mountSuspended(view);
    await wrapper.get('input').setValue(search);
    // Wait for the searched first page, not the unfiltered one rendered on mount.
    await vi.waitFor(() => expect(forSearch()).toHaveLength(1), { timeout: 2000 });
    await vi.waitFor(() => expect(loadMore(wrapper)).toBeDefined());

    await loadMore(wrapper)!.trigger('click');
    await vi.waitFor(() => expect(forSearch().some((q) => q.offset === '2')).toBe(true));
    // While a page loads the button shows a spinner instead of its label, so the
    // label returning means the failed request (and the app's one retry) settled.
    await vi.waitFor(() => expect(loadMore(wrapper)).toBeDefined(), { timeout: 4000 });
    expect(wrapper.text()).toContain(`${label} 1`);
    expect(wrapper.text()).toContain(`${label} 2`);
    expect(wrapper.text()).not.toContain(`${label} 3`);

    mode = 'ok';
    await loadMore(wrapper)!.trigger('click');
    await vi.waitFor(() => expect(wrapper.text()).toContain(`${label} 3`));
    await vi.waitFor(() => expect(loadMore(wrapper)).toBeUndefined());
  });
});

describe.each([
  { name: 'programs', view: ProgramsView, tab: 'Accepting', status: 'acceptance' },
  { name: 'mentees', view: MenteesView, tab: 'Graduated', status: 'graduated' },
])('$name status filter', ({ name, view, tab, status }) => {
  it(`requests ${status} items when the ${tab} tab is chosen`, async () => {
    const wrapper = await mountSuspended(view);
    await vi.waitFor(() => expect(requests[name]).toHaveLength(1));
    await wrapper
      .findAll('[role="tab"]')
      .find((t) => t.text().includes(tab))!
      .trigger('click');
    await vi.waitFor(() => expect(requests[name]!.at(-1)).toMatchObject({ status }));
  });
});
