<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <div
    v-if="isVisible"
    :class="`p-chip p-chip-size-${props.size} p-chip-type-${props.type}`"
  >
    <slot />
    <button
      v-if="props.removable"
      type="button"
      class="p-chip-remove-icon"
      aria-label="Remove"
      @click="dismiss"
    >
      <i
        class="fa-solid fa-circle-xmark"
        aria-hidden="true"
      />
    </button>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import type { ChipSize, ChipType } from './types/chip.types';

const props = withDefaults(
  defineProps<{
    size?: ChipSize;
    type?: ChipType;
    removable?: boolean;
  }>(),
  {
    type: 'default',
    size: 'default',
    removable: false,
  },
);
const emit = defineEmits<{ (e: 'dismissed'): void }>();

const isVisible = ref(true);

const dismiss = () => {
  isVisible.value = false;
  emit('dismissed');
};
</script>

<script lang="ts">
export default {
  name: 'LfxChip',
};
</script>
