// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { mountSuspended } from '@nuxt/test-utils/runtime';
import { flushPromises } from '@vue/test-utils';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h, ref } from 'vue';
import LfxDropdown from './dropdown/dropdown.vue';
import LfxDropdownItem from './dropdown/dropdown-item.vue';
import LfxDropdownSearch from './dropdown/dropdown-search.vue';
import LfxDropdownSelector from './dropdown/dropdown-selector.vue';
import LfxDropdownSubmenu from './dropdown/dropdown-submenu.vue';
import LfxPopover from './popover/popover.vue';
import LfxOption from './select/option.vue';
import LfxSelect from './select/select.vue';

const panel = () => document.body.querySelector<HTMLElement>('.c-popover__content');
const options = () => [...document.body.querySelectorAll<HTMLElement>('[role="option"]')];

afterEach(() => {
  vi.useRealTimers();
});

describe('LfxPopover', () => {
  const mountPopover = (props: Record<string, unknown> = {}) =>
    mountSuspended(LfxPopover, {
      props,
      slots: {
        default: () => h('button', 'Open'),
        content: () => h('p', { id: 'inside' }, 'Panel'),
      },
      attachTo: document.body,
    });

  it('toggles on click and closes on Escape or an outside click', async () => {
    const wrapper = await mountPopover();
    await wrapper.get('.c-popover__trigger').trigger('click');
    await flushPromises();
    expect(panel()?.textContent).toBe('Panel');

    document.body.querySelector<HTMLElement>('#inside')!.click();
    await flushPromises();
    expect(panel()).not.toBeNull();

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await flushPromises();
    expect(panel()).toBeNull();

    await wrapper.get('.c-popover__trigger').trigger('click');
    await flushPromises();
    document.body.click();
    await flushPromises();
    expect(panel()).toBeNull();
    expect(wrapper.emitted('update:visibility')).toEqual([[true], [false], [true], [false]]);
  });

  it('opens on hover and closes shortly after the pointer leaves', async () => {
    vi.useFakeTimers();
    const wrapper = await mountPopover({ triggerEvent: 'hover' });
    const trigger = wrapper.get('.c-popover__trigger').element;
    trigger.dispatchEvent(new Event('mouseenter'));
    await flushPromises();
    expect(panel()).not.toBeNull();

    trigger.dispatchEvent(new Event('mouseleave'));
    panel()!.dispatchEvent(new Event('mouseenter'));
    vi.advanceTimersByTime(200);
    await flushPromises();
    expect(panel()).not.toBeNull();

    panel()!.dispatchEvent(new Event('mouseleave'));
    vi.advanceTimersByTime(200);
    await flushPromises();
    expect(panel()).toBeNull();
  });

  it('follows the visibility prop, can start open, and closes a modal panel on click', async () => {
    const wrapper = await mountPopover({
      visibility: true,
      isModal: true,
      matchWidth: true,
      popoverClass: 'wide',
    });
    await flushPromises();
    expect(panel()!.className).toContain('is-modal');
    expect(panel()!.className).toContain('wide');
    panel()!.click();
    await flushPromises();
    expect(panel()).toBeNull();
    await wrapper.setProps({ visibility: false });
    await wrapper.setProps({ visibility: true });
    await flushPromises();
    expect(panel()).not.toBeNull();
  });

  it('never opens while disabled', async () => {
    const wrapper = await mountPopover({ disabled: true });
    await wrapper.get('.c-popover__trigger').trigger('click');
    await flushPromises();
    expect(panel()).toBeNull();
  });
});

describe('LfxSelect', () => {
  const Host = defineComponent({
    props: { initial: { type: String, default: '' } },
    setup(props) {
      const value = ref(props.initial);
      return () =>
        h('div', [
          h(
            LfxSelect,
            {
              modelValue: value.value,
              placeholder: 'Pick a skill',
              'onUpdate:modelValue': (v: string) => (value.value = v),
            },
            () => [
              h(LfxOption, { value: 'go', label: 'Go' }),
              h(LfxOption, { value: 'rust', label: 'Rust' }),
            ],
          ),
          h('output', value.value),
        ]);
    },
  });

  it('shows the placeholder, then the chosen option', async () => {
    const wrapper = await mountSuspended(Host, { attachTo: document.body });
    expect(wrapper.text()).toContain('Pick a skill');
    await wrapper.get('.c-popover__trigger').trigger('click');
    await flushPromises();
    expect(options().map((o) => o.textContent?.trim())).toEqual(['Go', 'Rust']);

    options()[1]!.click();
    await flushPromises();
    expect(wrapper.get('output').text()).toBe('rust');
    expect(wrapper.text()).toContain('Rust');
    expect(panel()).toBeNull();
  });

  it('labels a preselected value from its option, or falls back to the raw value', async () => {
    expect((await mountSuspended(Host, { props: { initial: 'go' } })).text()).toContain('Go');
    expect((await mountSuspended(Host, { props: { initial: 'zig' } })).text()).toContain('zig');
  });

  it('marks the selected option and selects with the keyboard', async () => {
    const wrapper = await mountSuspended(Host, {
      props: { initial: 'go' },
      attachTo: document.body,
    });
    await wrapper.get('.c-popover__trigger').trigger('click');
    await flushPromises();
    expect(options()[0]!.getAttribute('aria-selected')).toBe('true');
    options()[1]!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }));
    await flushPromises();
    expect(wrapper.get('output').text()).toBe('rust');
  });
});

describe('dropdown parts', () => {
  it('ignores an item without a value', async () => {
    const wrapper = await mountSuspended(LfxDropdownItem, {
      props: { label: 'Header', checkmarkBefore: true },
    });
    await wrapper.trigger('click');
    expect(wrapper.attributes('aria-selected')).toBe('false');
  });

  it('emits search input immediately, or debounced when lazy, and clears', async () => {
    const eager = await mountSuspended(LfxDropdownSearch, { props: { modelValue: '' } });
    expect(eager.get('input').attributes('placeholder')).toBe('Search...');
    await eager.get('input').setValue('go');
    expect(eager.emitted('update:modelValue')).toEqual([['go']]);
    await eager.get('[aria-label="Clear search"]').trigger('click');
    expect(eager.emitted('update:modelValue')!.at(-1)).toEqual(['']);
    await eager.setProps({ modelValue: 'rust' });
    expect((eager.get('input').element as HTMLInputElement).value).toBe('rust');

    vi.useFakeTimers();
    const lazy = await mountSuspended(LfxDropdownSearch, {
      props: { modelValue: '', lazy: true, placeholder: 'Find' },
    });
    await lazy.get('input').setValue('k8s');
    expect(lazy.emitted('update:modelValue')).toBeUndefined();
    await vi.advanceTimersByTimeAsync(300);
    expect(lazy.emitted('update:modelValue')).toEqual([['k8s']]);
  });

  it('opens a submenu inside a dropdown and goes back', async () => {
    await mountSuspended(LfxDropdown, {
      props: { visibility: true },
      slots: {
        trigger: () => h('button', 'Menu'),
        default: () =>
          h(
            LfxDropdownSubmenu,
            { name: 'sort', label: 'Sort by' },
            { item: () => 'Sort', default: () => h('p', { class: 'sub' }, 'Name') },
          ),
      },
      attachTo: document.body,
    });
    await flushPromises();
    const sub = () => document.body.querySelector<HTMLElement>('.c-dropdown__sub-menu')!;
    expect(sub().style.display).toBe('none');
    document.body.querySelector<HTMLElement>('.c-dropdown__sub > .c-dropdown__item')!.click();
    await flushPromises();
    expect(sub().style.display).toBe('');
    const back = [...document.body.querySelectorAll<HTMLElement>('.c-dropdown__item')].find((el) =>
      el.textContent?.includes('Sort by'),
    )!;
    back.click();
    await flushPromises();
    expect(sub().style.display).toBe('none');
  });

  it('styles the selector by size and type', async () => {
    const wrapper = await mountSuspended(LfxDropdownSelector, {
      props: { size: 'small', type: 'filled' },
      slots: { default: () => 'All' },
    });
    expect(wrapper.classes()).toEqual(
      expect.arrayContaining(['c-dropdown__selector--small', 'c-dropdown__selector--filled']),
    );
  });
});
