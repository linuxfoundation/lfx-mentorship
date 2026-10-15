// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { mountSuspended, registerEndpoint } from '@nuxt/test-utils/runtime';
import { flushPromises } from '@vue/test-utils';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import SignInToApplyModal from '../components/sign-in-to-apply-modal.vue';
import ProgramDetailView from './program-detail.vue';
import type { Program } from '~/types/program.types';

const day = 24 * 60 * 60 * 1000;
const iso = (offsetDays: number) => new Date(Date.now() + offsetDays * day).toISOString();

const fail = (statusCode: number, message: string) => () => {
  throw createError({ statusCode, message });
};

function program(id: string, overrides: Partial<Program> = {}): Program {
  const open = {
    id: `${id}-t1`,
    name: 'Fall 2026',
    status: 'open' as const,
    startsAt: '2026-09-01',
    endsAt: '2026-11-30',
    applicationsStartAt: iso(-3),
    applicationsCloseAt: iso(3),
  };
  return {
    id,
    slug: id,
    name: `Program ${id}`,
    description: 'About the program',
    skills: ['Go'],
    status: 'acceptance',
    foundation: { id: '', name: '', slug: '' },
    terms: [
      open,
      {
        id: `${id}-t0`,
        name: 'Spring 2025',
        status: 'closed',
        startsAt: '2025-03-01',
        endsAt: '2025-05-31',
      },
    ],
    activeTerms: [open],
    updatedAt: '2026-10-01T00:00:00Z',
    repositoryUrl: 'https://github.com/example/repo',
    mentees: [],
    mentors: [{ id: 'm1', name: 'Grace Hopper' }],
    sponsors: [],
    ...overrides,
  };
}

let openSpy: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  openSpy = vi.spyOn(window, 'open').mockImplementation(() => null);
});

afterEach(() => {
  vi.restoreAllMocks();
});

async function mountProgram(id: string) {
  const wrapper = await mountSuspended(ProgramDetailView, {
    props: { programId: id },
    attachTo: document.body,
  });
  await vi.waitFor(() => expect(wrapper.text()).not.toContain('Loading program'));
  return wrapper;
}

const tab = (wrapper: Awaited<ReturnType<typeof mountProgram>>, label: string) =>
  wrapper
    .findAll('[role="tab"]')
    .find((t) => t.text() === label)!
    .trigger('click');

const button = (label: string) =>
  [...document.body.querySelectorAll<HTMLButtonElement>('button')].find(
    (b) => b.textContent?.trim() === label,
  )!;

describe('ProgramDetailView', () => {
  it('shows a not-found state when the program fails to load', async () => {
    registerEndpoint('/api/programs/gone', fail(404, 'Program not found'));
    const wrapper = await mountSuspended(ProgramDetailView, {
      props: { programId: 'gone' },
      attachTo: document.body,
    });
    await vi.waitFor(() => expect(wrapper.text()).toContain('Program not found.'), {
      timeout: 3000,
    });
  });

  it('walks every tab', async () => {
    registerEndpoint('/api/programs/tabs', () => program('tabs'));
    registerEndpoint('/api/programs/tabs/mentees', () => [
      { id: 'u1', name: 'Ada Lovelace', status: 'active', termLabel: 'Fall 2026' },
      { id: 'u2', name: 'Alan Turing', status: 'graduated', termLabel: 'Spring 2025' },
    ]);
    registerEndpoint('/api/programs/tabs/sponsors', () => [
      { id: 's1', name: 'ACME Corp', amountCents: 250000 },
    ]);
    const wrapper = await mountProgram('tabs');
    expect(wrapper.text()).toContain('About the program');

    await tab(wrapper, 'Terms');
    expect(wrapper.text()).toContain('Spring 2025');
    expect(wrapper.text()).toContain('—');

    await tab(wrapper, 'Mentors');
    expect(wrapper.text()).toContain('Grace Hopper');

    await tab(wrapper, 'Mentees');
    await vi.waitFor(() => expect(wrapper.text()).toContain('Alan Turing'));
    expect(wrapper.text()).toContain('Ada Lovelace');

    await tab(wrapper, 'Sponsors');
    await vi.waitFor(() => expect(wrapper.text()).toContain('ACME Corp'));
    expect(
      wrapper
        .findAll('[role="tab"]')
        .find((t) => t.text() === 'Sponsors')!
        .attributes('aria-selected'),
    ).toBe('true');
  });

  it('reports mentees and sponsors that fail to load', async () => {
    registerEndpoint('/api/programs/broken', () => program('broken'));
    registerEndpoint('/api/programs/broken/mentees', fail(500, 'Mentees unavailable'));
    registerEndpoint('/api/programs/broken/sponsors', fail(500, 'Sponsors unavailable'));
    const wrapper = await mountProgram('broken');
    await tab(wrapper, 'Mentees');
    await vi.waitFor(() => expect(wrapper.text()).toContain('Unable to load mentees.'), {
      timeout: 3000,
    });
    await tab(wrapper, 'Sponsors');
    await vi.waitFor(() => expect(wrapper.text()).toContain('Unable to load sponsors.'), {
      timeout: 3000,
    });
  });

  it('opens the sign-in modal for a term and sends the applicant to Self Serve', async () => {
    registerEndpoint('/api/programs/apply', () => program('apply'));
    const wrapper = await mountProgram('apply');
    await wrapper
      .findAll('button')
      .find((b) => b.text().includes('Apply to This Program'))!
      .trigger('click');
    await flushPromises();
    expect(document.body.textContent).toContain('Fall 2026');

    button('Sign In').click();
    await flushPromises();
    expect(openSpy).toHaveBeenCalledWith(
      expect.stringMatching(/programId=apply&programTermId=apply-t1/),
      '_blank',
      'noopener,noreferrer',
    );
    expect(document.body.querySelector('.c-modal')).toBeNull();
  });

  it('opens the repository and the donation page', async () => {
    registerEndpoint('/api/programs/links', () => program('links'));
    const wrapper = await mountProgram('links');
    const click = (label: string) =>
      wrapper
        .findAll('button')
        .find((b) => b.text().includes(label))!
        .trigger('click');
    await click('Donate');
    expect(openSpy).toHaveBeenLastCalledWith(
      expect.stringMatching(/\/initiatives\/links$/),
      '_blank',
      'noopener,noreferrer',
    );
    await click('Repository');
    expect(openSpy).toHaveBeenLastCalledWith(
      'https://github.com/example/repo',
      '_blank',
      'noopener,noreferrer',
    );
  });

  it('returns to the overview when the program changes', async () => {
    registerEndpoint('/api/programs/first', () => program('first'));
    registerEndpoint('/api/programs/second', () =>
      program('second', { activeTerms: [], description: 'Second program' }),
    );
    const wrapper = await mountProgram('first');
    await tab(wrapper, 'Mentors');
    await wrapper.setProps({ programId: 'second' });
    await vi.waitFor(() => expect(wrapper.text()).toContain('Second program'));
  });
});

describe('SignInToApplyModal', () => {
  it('disables both actions without an apply URL', async () => {
    const wrapper = await mountSuspended(SignInToApplyModal, {
      props: { modelValue: true, programName: 'P', applyUrl: '' },
      attachTo: document.body,
    });
    await flushPromises();
    expect(button('Sign In').disabled).toBe(true);
    expect(button('Create an Account').disabled).toBe(true);
    document.body.querySelector<HTMLButtonElement>('[aria-label="Close"]')!.click();
    expect(wrapper.emitted('update:modelValue')).toEqual([[false]]);
    expect(openSpy).not.toHaveBeenCalled();
  });
});
