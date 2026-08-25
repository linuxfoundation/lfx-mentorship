<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="container pb-16">
    <div
      v-if="isLoading"
      class="grid grid-cols-1 gap-8 sm:grid-cols-2 lg:grid-cols-3"
    >
      <program-card-loading
        v-for="n in 6"
        :key="n"
      />
    </div>

    <div
      v-else-if="error"
      class="flex items-center gap-2 text-negative-600"
    >
      <lfx-icon
        name="circle-exclamation"
        type="solid"
        :size="16"
      />
      <span class="text-body-1">Failed to load programs. Please try again.</span>
    </div>

    <div
      v-else-if="!programs.length"
      class="flex flex-col items-center justify-center gap-4 py-24 text-neutral-500"
    >
      <lfx-icon
        name="folder-open"
        type="light"
        :size="40"
      />
      <p class="text-base">No programs found.</p>
    </div>

    <template v-else>
      <div class="grid grid-cols-1 gap-8 md:grid-cols-2 lg:grid-cols-3">
        <program-card
          v-for="program in visiblePrograms"
          :key="program.id"
          :program="program"
        />
      </div>

      <div
        v-if="hasMore"
        class="flex justify-center mt-10"
      >
        <lfx-button
          label="Load More"
          type="tertiary"
          button-style="pill"
          @click="$emit('load-more')"
        />
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import ProgramCard from './program-card.vue';
import ProgramCardLoading from './program-card-loading.vue';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxIcon from '~/components/uikit/icon/icon.vue';
import type { Program } from '~/types/program.types';

const props = defineProps<{
  programs: Program[];
  visibleCount: number;
  isLoading: boolean;
  error: Error | null;
}>();

defineEmits<{ (e: 'load-more'): void }>();

const visiblePrograms = computed(() => props.programs.slice(0, props.visibleCount));
const hasMore = computed(() => props.visibleCount < props.programs.length);
</script>

<script lang="ts">
export default {
  name: 'ProgramsGrid',
};
</script>
