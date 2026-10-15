// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { mountSuspended } from '@nuxt/test-utils/runtime';
import { flushPromises } from '@vue/test-utils';
import { afterEach, describe, expect, it } from 'vitest';
import { h } from 'vue';
import LfxDrawer from './drawer/drawer.vue';
import LfxModal from './modal/modal.vue';

const overlays = [
  ['modal', LfxModal, '.c-modal', '.c-modal__content', {}],
  // The drawer's own close button would otherwise be the first focusable control.
  ['drawer', LfxDrawer, '.c-drawer', '.c-drawer__content', { hideCloseButton: true }],
] as const;

const twoButtons = () => [
  h('button', { id: 'first' }, 'First'),
  h('button', { id: 'last' }, 'Last'),
];
const $ = <T extends Element = HTMLElement>(selector: string) =>
  document.body.querySelector<T>(selector);
const keyup = (key: string) => window.dispatchEvent(new KeyboardEvent('keyup', { key }));
const tab = (target: Element, shiftKey = false) => {
  const event = new KeyboardEvent('keydown', {
    key: 'Tab',
    shiftKey,
    bubbles: true,
    cancelable: true,
  });
  target.dispatchEvent(event);
  return event;
};

afterEach(() => {
  document.body.innerHTML = '';
  document.documentElement.style.overflow = '';
});

describe.each(overlays)('%s', (_name, component, backdrop, content, baseProps) => {
  const open = async (props: Record<string, unknown> = {}, slots = { default: twoButtons }) => {
    const trigger = document.createElement('button');
    document.body.appendChild(trigger);
    trigger.focus();
    const wrapper = await mountSuspended(component, {
      props: { modelValue: false, ...baseProps, ...props },
      slots,
    });
    await wrapper.setProps({ modelValue: true });
    await flushPromises();
    return { wrapper, trigger };
  };

  it('renders nothing while closed', async () => {
    await mountSuspended(component, { props: { modelValue: false } });
    expect($(backdrop)).toBeNull();
  });

  it('locks scrolling and focuses the first control when opened', async () => {
    await open({ ariaLabel: 'Share' });
    expect($(content)!.getAttribute('aria-label')).toBe('Share');
    expect(document.documentElement.style.overflow).toBe('hidden');
    expect(document.activeElement?.id).toBe('first');
  });

  it('closes on Escape and on a backdrop click, but not on a content click', async () => {
    const { wrapper } = await open();
    $(content)!.click();
    expect(wrapper.emitted('update:modelValue')).toBeUndefined();
    keyup('Enter');
    expect(wrapper.emitted('update:modelValue')).toBeUndefined();
    keyup('Escape');
    $(backdrop)!.click();
    expect(wrapper.emitted('update:modelValue')).toEqual([[false], [false]]);
  });

  it('stays open when the close guard refuses', async () => {
    const { wrapper } = await open({ closeFunction: () => false });
    keyup('Escape');
    expect(wrapper.emitted('update:modelValue')).toBeUndefined();
  });

  it('keeps Tab focus inside', async () => {
    await open();
    const first = $('#first')!;
    const last = $('#last')!;
    last.focus();
    expect(tab(last).defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(first);
    expect(tab(first, true).defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(last);
    first.focus();
    expect(tab(first).defaultPrevented).toBe(false);
  });

  it('focuses the dialog itself when it has no controls', async () => {
    await open({}, { default: () => [h('p', 'Text only')] });
    const dialog = $(content)!;
    expect(document.activeElement).toBe(dialog);
    expect(tab(dialog).defaultPrevented).toBe(true);
  });

  it('restores scroll and focus when closed', async () => {
    const { wrapper, trigger } = await open();
    await wrapper.setProps({ modelValue: false });
    await flushPromises();
    expect($(backdrop)).toBeNull();
    expect(document.documentElement.style.overflow).toBe('');
    expect(document.activeElement).toBe(trigger);
  });

  it('unlocks scrolling when unmounted while open', async () => {
    const { wrapper } = await open();
    wrapper.unmount();
    expect(document.documentElement.style.overflow).toBe('');
  });
});

describe('overlay variants', () => {
  it.each(['centered', 'floating', 'cover', 'mobile-cover'] as const)(
    'applies the %s modal type',
    async (type) => {
      await mountSuspended(LfxModal, { props: { modelValue: true, type, width: '20rem' } });
      await flushPromises();
      expect($('.c-modal')!.classList).toContain(`c-modal--${type}`);
      expect($<HTMLElement>('.c-modal__content')!.style.maxWidth).toBe(
        type === 'cover' ? '' : '20rem',
      );
    },
  );

  it.each([
    ['right', 'maxWidth', '30rem'],
    ['bottom', 'maxHeight', '85vh'],
  ] as const)('sizes a %s drawer', async (position, property, value) => {
    await mountSuspended(LfxDrawer, { props: { modelValue: true, position, width: '30rem' } });
    await flushPromises();
    expect($('.c-drawer')!.classList).toContain(`c-drawer--${position}`);
    expect($<HTMLElement>('.c-drawer__content')!.style[property]).toBe(value);
  });

  it('closes the drawer from its close button', async () => {
    const wrapper = await mountSuspended(LfxDrawer, { props: { modelValue: true } });
    await flushPromises();
    $<HTMLButtonElement>('[aria-label="Close"]')!.click();
    expect(wrapper.emitted('update:modelValue')).toEqual([[false]]);
  });
});
