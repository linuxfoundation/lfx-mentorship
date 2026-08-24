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
      { '!rounded-none': !props.rounded },
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
    rounded?: boolean;
  }>(),
  {
    variation: 'default',
    size: 'medium',
    type: 'solid',
    rounded: true,
  },
);
</script>

<script lang="ts">
export default {
  name: 'LfxTag',
};
</script>
