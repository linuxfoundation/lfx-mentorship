// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { mountSuspended } from '@nuxt/test-utils/runtime';
import { flushPromises } from '@vue/test-utils';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { h } from 'vue';
import LfxEditor from './editor/editor.client.vue';
import LfxMaintainHeight from './maintain-height/maintain-height.vue';
import LfxScrollArea from './scroll-view/scroll-area.vue';
import LfxScrollView from './scroll-view/scroll-view.vue';
import SkillFilterSelect from '~/components/shared/skill-filter-select.vue';

const markdown = vi.hoisted(() => ({ configure: vi.fn() }));
vi.mock('~/utils/quill-markdown-paste', () => ({
  configureQuillMarkdownEditor: markdown.configure,
}));

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  document.body.innerHTML = '';
});

describe('SkillFilterSelect', () => {
  const skills = ['Go', 'Kubernetes', 'Rust'];
  const items = () =>
    [...document.body.querySelectorAll<HTMLElement>('[role="option"]')].map((o) =>
      o.textContent?.trim(),
    );

  it('filters skills as the user types and emits the chosen one', async () => {
    const wrapper = await mountSuspended(SkillFilterSelect, {
      props: { modelValue: 'all', skillOptions: skills },
      attachTo: document.body,
    });
    expect(wrapper.text()).toContain('All skills');
    await wrapper.get('.c-popover__trigger').trigger('click');
    await flushPromises();
    expect(items()).toEqual(['All skills', 'Go', 'Kubernetes', 'Rust']);

    const search = document.body.querySelector<HTMLInputElement>(
      'input[placeholder="Search skills"]',
    )!;
    search.value = 'KU';
    search.dispatchEvent(new Event('input'));
    await flushPromises();
    expect(items()).toEqual(['All skills', 'Kubernetes']);

    search.value = 'zzz';
    search.dispatchEvent(new Event('input'));
    await flushPromises();
    expect(document.body.textContent).toContain('No skills found');

    search.value = 'rust';
    search.dispatchEvent(new Event('input'));
    await flushPromises();
    document.body.querySelectorAll<HTMLElement>('[role="option"]')[1]!.click();
    await flushPromises();
    expect(wrapper.emitted('update:modelValue')).toEqual([['Rust']]);

    // Reopening starts from the full list again.
    await wrapper.get('.c-popover__trigger').trigger('click');
    await flushPromises();
    expect(items()).toHaveLength(4);
  });

  it('labels the trigger with the selected skill', async () => {
    const wrapper = await mountSuspended(SkillFilterSelect, {
      props: { modelValue: 'Go', skillOptions: skills },
    });
    expect(wrapper.text()).toContain('Go');
    expect(wrapper.text()).not.toContain('All skills');
  });
});

describe('scroll spy', () => {
  type Callback = (entries: Partial<IntersectionObserverEntry>[]) => void;
  let fire: Callback;
  const observed = new Set<Element>();
  const disconnect = vi.fn();

  const stubObserver = () =>
    vi.stubGlobal(
      'IntersectionObserver',
      class {
        constructor(callback: Callback) {
          fire = callback;
        }
        observe = (el: Element) => observed.add(el);
        unobserve = (el: Element) => observed.delete(el);
        disconnect = disconnect;
      },
    );

  it('reports the section scrolled into view, or the first one at the top of the page', async () => {
    stubObserver();
    const wrapper = await mountSuspended(LfxScrollArea, {
      slots: {
        default: ({ observer }: { observer: IntersectionObserver | null }) => [
          h(LfxScrollView, { id: 'intro', observer }),
          h(LfxScrollView, { id: 'terms', observer }),
        ],
      },
      attachTo: document.body,
    });
    await flushPromises();
    expect(observed.size).toBe(2);
    const terms = document.getElementById('terms')!;

    fire([{ target: terms, intersectionRatio: 0.2 }]);
    expect(wrapper.emitted('scrolledToView')).toBeUndefined();

    fire([{ target: terms, intersectionRatio: 0.8 }]);
    expect(wrapper.emitted('scrolledToView')).toEqual([['intro']]);

    document.documentElement.scrollTop = 400;
    document.dispatchEvent(new Event('scroll'));
    await flushPromises();
    fire([{ target: terms, intersectionRatio: 0.8 }]);
    expect(wrapper.emitted('scrolledToView')!.at(-1)).toEqual(['terms']);

    const view = wrapper.findAllComponents(LfxScrollView)[1]!;
    expect(view.emitted('scrolledToView')).toEqual([['terms'], ['terms']]);
    wrapper.unmount();
    expect(disconnect).toHaveBeenCalled();
  });
});

describe('LfxMaintainHeight', () => {
  it('reserves the measured height once the page has scrolled', async () => {
    vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(120);
    const wrapper = await mountSuspended(LfxMaintainHeight, {
      props: { loaded: false, scrollTop: 0 },
      slots: { default: () => 'Header' },
    });
    expect(wrapper.html()).not.toContain('height: 120px');
    await wrapper.setProps({ loaded: true, scrollTop: 50 });
    await flushPromises();
    expect(wrapper.html()).toContain('height: 120px');
    expect(wrapper.text()).toContain('Header');
  });
});

describe('LfxEditor', () => {
  it('shows a placeholder until Quill loads, then wires Markdown input once', async () => {
    const wrapper = await mountSuspended(LfxEditor, {
      props: { modelValue: '<p>Hi</p>', invalid: true, disabled: true, maxLength: 500 },
    });
    expect(wrapper.classes()).toEqual(expect.arrayContaining(['is-invalid', 'is-disabled']));
    await vi.waitFor(() => expect(markdown.configure).toHaveBeenCalledTimes(1), { timeout: 5000 });
    expect(markdown.configure).toHaveBeenCalledWith(
      expect.objectContaining({ root: expect.anything() }),
      500,
    );
    expect(wrapper.text()).not.toContain('Loading editor');

    const editor = wrapper.findComponent({ name: 'Editor' });
    editor.vm.$emit('load', { instance: {} });
    editor.vm.$emit('load', {});
    expect(markdown.configure).toHaveBeenCalledTimes(1);

    editor.vm.$emit('update:modelValue', null);
    expect(wrapper.emitted('update:modelValue')).toEqual([['']]);
  });
});
