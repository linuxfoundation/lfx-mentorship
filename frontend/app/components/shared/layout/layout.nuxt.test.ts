// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { mountSuspended } from '@nuxt/test-utils/runtime';
import { flushPromises } from '@vue/test-utils';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h } from 'vue';
import DesktopNav from './components/desktop-nav.vue';
import MentorshipFooter from './footer.vue';
import MentorshipHeader from './header.vue';
import ErrorPage from '~/error.vue';

afterEach(() => {
  vi.restoreAllMocks();
});

const drawer = () => document.body.querySelector<HTMLElement>('[aria-label="Main menu"]');
const linkTexts = (root: ParentNode) =>
  [...root.querySelectorAll('a')].map((a) => a.textContent?.trim());

describe('MentorshipHeader', () => {
  it('opens the mobile menu and closes it from its button or a link', async () => {
    const wrapper = await mountSuspended(MentorshipHeader, { attachTo: document.body });
    expect(drawer()).toBeNull();

    await wrapper.get('[aria-label="Open menu"]').trigger('click');
    await flushPromises();
    expect(linkTexts(drawer()!)).toEqual(
      expect.arrayContaining(['Find a Program', 'Mentees', 'Mentors', 'Enroll a Program']),
    );

    drawer()!.querySelector<HTMLElement>('[aria-label="Close menu"]')!.click();
    await flushPromises();
    expect(drawer()).toBeNull();

    await wrapper.get('[aria-label="Open menu"]').trigger('click');
    await flushPromises();
    drawer()!.querySelector('a')!.click();
    await flushPromises();
    expect(drawer()).toBeNull();
  });
});

describe('DesktopNav', () => {
  it('links each directory', async () => {
    const wrapper = await mountSuspended(DesktopNav);
    expect(linkTexts(wrapper.element)).toEqual(
      expect.arrayContaining(['Find a Program', 'Mentees', 'Mentors']),
    );
  });
});

describe('MentorshipFooter', () => {
  const button = (label: string) =>
    [...document.body.querySelectorAll<HTMLButtonElement>('button')].find(
      (b) => b.textContent?.trim() === label,
    )!;
  const dialog = () => document.body.querySelector('[role="dialog"]');

  it('points support requests to Self Serve, where chat is available', async () => {
    const open = vi.spyOn(window, 'open').mockImplementation(() => null);
    const wrapper = await mountSuspended(MentorshipFooter, { attachTo: document.body });
    for (const link of wrapper.findAll('a[target="_blank"]')) {
      expect(link.attributes('rel')).toBe('noopener noreferrer');
    }
    expect(dialog()).toBeNull();

    button('Contact support').click();
    await flushPromises();
    expect(dialog()?.textContent).toContain('sign in to LFX Self Serve');

    button('Go to LFX Self Serve').click();
    await flushPromises();
    expect(open).toHaveBeenCalledWith(
      expect.stringMatching(/\/mentorship\/mentee$/),
      '_blank',
      'noopener,noreferrer',
    );
    expect(dialog()).toBeNull();
  });

  it('closes the support popup without leaving the site', async () => {
    const open = vi.spyOn(window, 'open').mockImplementation(() => null);
    await mountSuspended(MentorshipFooter, { attachTo: document.body });
    button('Contact support').click();
    await flushPromises();

    document.body.querySelector<HTMLButtonElement>('[aria-label="Close"]')!.click();
    await flushPromises();
    expect(dialog()).toBeNull();
    expect(open).not.toHaveBeenCalled();
  });
});

describe('error page', () => {
  it.each([
    [404, 'Page not found'],
    [500, 'Something went wrong'],
  ])('explains a %i and clears the error on the way home', async (statusCode, title) => {
    let error!: ReturnType<typeof useError>;
    const Host = defineComponent({
      setup() {
        error = useError();
        error.value = createError({ statusCode });
        return () => h(ErrorPage);
      },
    });
    const wrapper = await mountSuspended(Host);
    expect(wrapper.text()).toContain(String(statusCode));
    expect(wrapper.text()).toContain(title);
    await wrapper
      .findAll('button')
      .find((b) => b.text().includes('Back to home'))!
      .trigger('click');
    await flushPromises();
    expect(error.value).toBeFalsy();
  });
});
