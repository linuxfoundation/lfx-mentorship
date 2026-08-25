<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div
    class="c-tag"
    :class="[
      `c-tag--${props.variation}`,
      `c-tag--${props.size}`,
      `c-tag--${props.type}`,
      { '!rounded-sm': props.roundedSize === 'small' },
      { '!rounded-md': props.roundedSize === 'medium' },
      { '!rounded-lg': props.roundedSize === 'large' },
      { '!rounded-full': props.roundedSize === 'full' },
    ]"
  >
    <slot />
  </div>
</template>

<script setup lang="ts">
import type { TagType, TagSize, TagStyle } from './types/tag.types';

const props = withDefaults(
  defineProps<{
    variation?: TagStyle;
    size?: TagSize;
    type?: TagType;
    roundedSize?: 'none' | 'small' | 'medium' | 'large' | 'full';
  }>(),
  {
    variation: 'default',
    size: 'medium',
    type: 'solid',
    roundedSize: 'small',
  },
);
</script>

<script lang="ts">
export default {
  name: 'LfxTag',
};
</script>
