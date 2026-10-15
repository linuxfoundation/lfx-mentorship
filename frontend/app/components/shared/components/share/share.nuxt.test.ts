// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { mountSuspended } from '@nuxt/test-utils/runtime';
import { flushPromises } from '@vue/test-utils';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h, onMounted } from 'vue';
import ShareActions from './share-actions.vue';
import ShareModal from './share-modal.vue';
import ShareModalGlobal from './share-modal-global.vue';
import { useShareModalStore } from './store/share-modal.store';
import LfxToast from '~/components/uikit/toast/toast.vue';
import useToastService from '~/components/uikit/toast/toast.service';
import { ToastTypesEnum } from '~/components/uikit/toast/types/toast.types';

const defaults = { url: 'https://mentorship.example.org/programs/a b', title: 'Cloud & Native' };

let openSpy: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  openSpy = vi.spyOn(window, 'open').mockImplementation(() => null);
});

afterEach(() => {
  vi.restoreAllMocks();
});

const click = async (wrapper: Awaited<ReturnType<typeof mountSuspended>>, label: string) => {
  await wrapper.get(`[aria-label="${label}"]`).trigger('click');
};

describe('ShareActions', () => {
  it.each([
    [
      'Share on X',
      'https://twitter.com/intent/tweet?text=Explore%20Cloud%20%26%20Native%20https%3A%2F%2Fmentorship.example.org%2Fprograms%2Fa%20b',
    ],
    [
      'Share on LinkedIn',
      'https://www.linkedin.com/sharing/share-offsite/?url=https%3A%2F%2Fmentorship.example.org%2Fprograms%2Fa%20b',
    ],
    [
      'Share on Reddit',
      'https://www.reddit.com/submit?title=Explore%20Cloud%20%26%20Native&url=https%3A%2F%2Fmentorship.example.org%2Fprograms%2Fa%20b',
    ],
  ])('%s opens an encoded share URL without an opener', async (label, url) => {
    const wrapper = await mountSuspended(ShareActions, { props: { defaults } });
    await click(wrapper, label);
    expect(openSpy).toHaveBeenCalledWith(
      url,
      '_blank',
      expect.stringContaining('noopener,noreferrer'),
    );
  });

  it('opens a mail draft, with a generic subject when there is no title', async () => {
    const wrapper = await mountSuspended(ShareActions, {
      props: { defaults: { url: 'https://x.org' } },
    });
    const trigger = wrapper
      .findAll('.c-popover__trigger')
      .find((d) => d.find('[aria-label="Send email"]').exists());
    await trigger!.trigger('click');
    expect(openSpy).toHaveBeenCalledWith(
      'mailto:?subject=Check%20this%20out&body=https%3A%2F%2Fx.org',
      '_blank',
    );
  });

  it('copies the link, then emits copied', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } });
    const wrapper = await mountSuspended(ShareActions, { props: { defaults } });
    await wrapper.findAll('button').at(-1)!.trigger('click');
    await flushPromises();
    expect(writeText).toHaveBeenCalledWith(defaults.url);
    expect(wrapper.emitted('copied')).toHaveLength(1);
    vi.unstubAllGlobals();
  });

  it('does not emit copied when the clipboard refuses', async () => {
    vi.stubGlobal('navigator', {
      ...navigator,
      clipboard: { writeText: vi.fn().mockRejectedValue(new Error('denied')) },
    });
    const wrapper = await mountSuspended(ShareActions, { props: { defaults } });
    await wrapper.findAll('button').at(-1)!.trigger('click');
    await flushPromises();
    expect(wrapper.emitted('copied')).toBeUndefined();
    vi.unstubAllGlobals();
  });
});

describe('ShareModal', () => {
  it('closes from the close button and after a copy', async () => {
    const wrapper = await mountSuspended(ShareModal, {
      props: { modelValue: true, defaults: { ...defaults, area: 'Programs' } },
    });
    expect(document.body.textContent).toContain('Programs');
    await wrapper.findComponent(ShareActions).vm.$emit('copied');
    expect(wrapper.emitted('update:modelValue')).toEqual([[false]]);
  });

  it('opens from the store and clears it on close', async () => {
    const wrapper = await mountSuspended(ShareModalGlobal);
    const store = useShareModalStore();
    expect(wrapper.findComponent(ShareModal).exists()).toBe(false);
    store.openShareModal(defaults);
    await flushPromises();
    expect(wrapper.findComponent(ShareModal).exists()).toBe(true);
    store.closeShareModal();
    await flushPromises();
    expect(store.shareData).toBeNull();
    expect(wrapper.findComponent(ShareModal).exists()).toBe(false);
  });
});

describe('LfxToast', () => {
  const Host = defineComponent({
    props: { show: { type: Function, required: true } },
    setup(props) {
      const { showToast } = useToastService();
      // The toast container must be listening before a message is sent.
      onMounted(() => (props.show as (fn: typeof showToast) => void)(showToast));
      return () => h(LfxToast);
    },
  });

  it.each([
    [ToastTypesEnum.positive, 'fa-circle-check'],
    [ToastTypesEnum.negative, 'fa-circle-exclamation'],
    [ToastTypesEnum.warning, 'fa-triangle-exclamation'],
    [ToastTypesEnum.info, 'fa-circle-info'],
  ])('shows a %s toast with its icon and sanitized detail', async (type, icon) => {
    await mountSuspended(Host, {
      props: {
        show: (show: (m: string, t: string) => void) =>
          show('<b>Saved</b><img src=x onerror=alert(1)>', type),
      },
    });
    await vi.waitFor(() => expect(document.body.innerHTML).toContain(icon));
    expect(document.body.innerHTML).toContain('<b>Saved</b>');
    expect(document.body.innerHTML).not.toContain('onerror');
  });

  it('shows a heading and runs the action', async () => {
    const action = vi.fn();
    await mountSuspended(Host, {
      props: {
        show: (show: (m: string, t: string, i?: string, d?: number, c?: object) => void) =>
          show('Body', ToastTypesEnum.default, 'fa-light fa-star', 3000, {
            title: 'Heading',
            actionLabel: 'Undo',
            action,
          }),
      },
    });
    await vi.waitFor(() => expect(document.body.textContent).toContain('Heading'));
    expect(document.body.innerHTML).toContain('fa-star');
    const undo = [...document.body.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('Undo'),
    )!;
    undo.click();
    expect(action).toHaveBeenCalled();
  });
});
