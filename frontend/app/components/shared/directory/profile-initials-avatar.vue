<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <div
    class="inline-flex shrink-0 items-center justify-center rounded-full bg-brand-50 text-brand-700 font-semibold select-none overflow-hidden"
    :class="sizeClass"
    :aria-label="name"
    role="img"
  >
    <img
      v-if="src && !imageFailed"
      :src="src"
      :alt="name"
      class="h-full w-full object-cover"
      @error="imageFailed = true"
    />
    <template v-else>{{ initial }}</template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';

const props = withDefaults(
  defineProps<{
    name: string;
    src?: string;
    size?: 'xlarge' | 'large' | 'normal' | 'small' | 'xsmall';
  }>(),
  {
    size: 'large',
    src: undefined,
  },
);

const imageFailed = ref(false);

watch(
  () => props.src,
  () => {
    imageFailed.value = false;
  },
);

const initial = computed(() => {
  const trimmed = props.name.trim();
  return trimmed ? trimmed.charAt(0).toUpperCase() : '?';
});

const sizeClass = computed(() => {
  switch (props.size) {
    case 'xlarge':
      return 'h-16 w-16 text-xl';
    case 'large':
      return 'h-12 w-12 text-base';
    case 'small':
      return 'h-8 w-8 text-xs';
    case 'xsmall':
      return 'h-6 w-6 text-xs';
    default:
      return 'h-10 w-10 text-sm';
  }
});
</script>

<script lang="ts">
export default {
  name: 'ProfileInitialsAvatar',
};
</script>
