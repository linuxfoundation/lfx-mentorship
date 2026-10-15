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

const intercom = vi.hoisted(() => ({ boot: vi.fn(), show: vi.fn() }));
vi.mock('~/composables/useIntercom', () => ({ useIntercom: () => intercom }));

afterEach(() => {
  vi.restoreAllMocks();
  document.body.innerHTML = '';
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
  const support = (wrapper: Awaited<ReturnType<typeof mountSuspended>>) =>
    wrapper.findAll('button').find((b) => b.text() === 'Contact support')!;

  it('opens support chat', async () => {
    intercom.boot.mockResolvedValue(undefined);
    const wrapper = await mountSuspended(MentorshipFooter);
    for (const link of wrapper.findAll('a[target="_blank"]')) {
      expect(link.attributes('rel')).toBe('noopener noreferrer');
    }
    await support(wrapper).trigger('click');
    await flushPromises();
    expect(intercom.show).toHaveBeenCalled();
  });

  it('reports an unavailable support chat', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    const failure = new Error('blocked');
    intercom.boot.mockRejectedValue(failure);
    const wrapper = await mountSuspended(MentorshipFooter);
    await support(wrapper).trigger('click');
    await flushPromises();
    expect(consoleError).toHaveBeenCalledWith(
      '[MentorshipFooter] Intercom is unavailable',
      failure,
    );
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
