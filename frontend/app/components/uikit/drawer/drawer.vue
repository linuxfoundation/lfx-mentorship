<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <div v-if="isDrawerOpened">
    <teleport to="body">
      <div
        class="c-drawer"
        :class="`c-drawer--${props.position}`"
        @click="requestClose()"
      >
        <div
          ref="contentRef"
          class="c-drawer__content"
          :style="props.position === 'bottom' ? { 'max-height': props.height } : { 'max-width': props.width }"
          role="dialog"
          aria-modal="true"
          :aria-label="props.ariaLabel"
          tabindex="-1"
          v-bind="$attrs"
          @click.stop
          @keydown="onKeydown"
        >
          <lfx-icon-button
            v-if="!props.hideCloseButton"
            type="transparent"
            icon="xmark"
            aria-label="Close"
            class="absolute top-0 right-0 mr-5 mt-5 z-[999]"
            @click="requestClose()"
          />
          <slot :close="requestClose" />
        </div>
      </div>
    </teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue';
import LfxIconButton from '~/components/uikit/icon-button/icon-button.vue';
import { focusFirst, lockBodyScroll, trapFocus, unlockBodyScroll } from '~/components/uikit/utils/overlay';

const props = withDefaults(
  defineProps<{
    modelValue: boolean;
    width?: string;
    height?: string;
    closeFunction?: () => boolean;
    position?: 'left' | 'right' | 'bottom';
    hideCloseButton?: boolean;
    ariaLabel?: string;
  }>(),
  {
    width: '37.5rem',
    height: '85vh',
    closeFunction: () => true,
    position: 'right',
    hideCloseButton: false,
    ariaLabel: 'Menu',
  },
);

const emit = defineEmits<{ (e: 'update:modelValue', value: boolean): void }>();

const contentRef = ref<HTMLElement | null>(null);
let previousFocus: HTMLElement | null = null;

const isDrawerOpened = computed<boolean>({
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

watch(
  () => isDrawerOpened.value,
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
  if (isDrawerOpened.value) {
    unlockBodyScroll();
  }
});
</script>

<script lang="ts">
export default {
  name: 'LfxDrawer',
  inheritAttrs: false,
};
</script>
