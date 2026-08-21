<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="flex flex-col gap-8">
    <section class="flex flex-col gap-2">
      <h2 class="text-lg font-semibold text-neutral-900">Program Description</h2>
      <!-- eslint-disable vue/no-v-html -- content is sanitized via useSanitize() -->
      <div
        class="lfx-rich-text text-neutral-600"
        v-html="descriptionHtml"
      />
      <!-- eslint-enable vue/no-v-html -->
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { Program } from '~/types/program.types';
import { useSanitize } from '~/composables/useSanitize';

const props = defineProps<{ program: Program }>();
const { renderDescription } = useSanitize();

const descriptionHtml = computed(() => renderDescription(props.program.description ?? ''));
</script>

<script lang="ts">
export default {
  name: 'ProgramDetailOverview',
};
</script>
