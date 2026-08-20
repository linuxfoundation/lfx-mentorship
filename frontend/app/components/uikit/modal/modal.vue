<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <template v-if="isModalOpened">
    <teleport to="body">
      <div
        class="c-modal"
        :class="modalClass"
        @click="requestClose()"
      >
        <div
          ref="contentRef"
          class="c-modal__content"
          :class="props.contentClass"
          :style="props.type === 'cover' ? {} : { 'max-width': props.width, 'max-height': props.height }"
          role="dialog"
          aria-modal="true"
          :aria-label="props.ariaLabel"
          tabindex="-1"
          v-bind="$attrs"
          @click.stop
          @keydown="onKeydown"
        >
          <slot :close="requestClose" />
        </div>
      </div>
    </teleport>
  </template>
</template>

<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue';
import { focusFirst, lockBodyScroll, trapFocus, unlockBodyScroll } from '~/components/uikit/utils/overlay';

const props = withDefaults(
  defineProps<{
    modelValue: boolean;
    type?: 'default' | 'floating' | 'cover' | 'mobile-cover';
    contentClass?: string;
    width?: string;
    height?: string;
    closeFunction?: () => boolean;
    ariaLabel?: string;
  }>(),
  {
    type: 'default',
    width: '37.5rem',
    height: 'auto',
    closeFunction: () => true,
    contentClass: undefined,
    ariaLabel: 'Dialog',
  },
);

const emit = defineEmits<{ (e: 'update:modelValue', value: boolean): void }>();

const contentRef = ref<HTMLElement | null>(null);
let previousFocus: HTMLElement | null = null;

const isModalOpened = computed<boolean>({
  get() {
    return props.modelValue;
  },
  set(value: boolean) {
    emit('update:modelValue', value);
  },
});

const requestClose = () => {
  if (props.closeFunction()) {
    emit('update:modelValue', false);
  }
};

const onKeydown = (event: KeyboardEvent) => {
  if (!contentRef.value) return;
  trapFocus(event, contentRef.value);
};

const onEscapeKeyUp = (event: KeyboardEvent) => {
  if (event.key === 'Escape') {
    requestClose();
  }
};

const modalClass = computed(() => {
  return {
    'c-modal--floating': props.type === 'floating',
    'c-modal--cover': props.type === 'cover',
    'c-modal--mobile-cover': props.type === 'mobile-cover',
  };
});

watch(
  () => props.modelValue,
  async (show: boolean) => {
    if (!import.meta.client) return;
    if (!show) {
      window.removeEventListener('keyup', onEscapeKeyUp);
      unlockBodyScroll();
      previousFocus?.focus();
      previousFocus = null;
    } else {
      previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
      window.addEventListener('keyup', onEscapeKeyUp);
      lockBodyScroll();
      await nextTick();
      if (contentRef.value) {
        focusFirst(contentRef.value);
      }
    }
  },
  { immediate: true },
);

onUnmounted(() => {
  if (!import.meta.client) return;
  window.removeEventListener('keyup', onEscapeKeyUp);
  if (props.modelValue) {
    unlockBodyScroll();
  }
});
</script>

<script lang="ts">
export default {
  name: 'LfxModal',
  inheritAttrs: false,
};
</script>
