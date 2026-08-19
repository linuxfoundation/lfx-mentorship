<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="container px-5 pb-16 md:px-10 md:pb-20">
    <div class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8">
      <div class="flex flex-wrap items-center justify-between gap-3 mb-6">
        <h2 class="font-secondary text-2xl md:text-3xl font-light text-neutral-900">
          Programs Accepting Applications
        </h2>
        <NuxtLink
          :to="AppRoute.FindProgram"
          class="text-sm font-medium text-brand-600 hover:text-brand-700"
        >
          View all{{ acceptingCountLabel ? ` ${acceptingCountLabel}` : '' }} programs →
        </NuxtLink>
      </div>

      <div
        v-if="isLoading"
        class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3"
      >
        <div
          v-for="n in LANDING_PROGRAMS_PREVIEW_COUNT"
          :key="n"
          class="h-48 rounded-2xl border border-neutral-100 bg-neutral-50 animate-pulse"
        />
      </div>

      <div
        v-else-if="error"
        class="flex items-center gap-2 text-negative-600 py-6"
      >
        <lfx-icon
          name="circle-exclamation"
          type="solid"
          :size="16"
        />
        <span class="text-sm">Failed to load programs. Please try again.</span>
      </div>

      <div
        v-else-if="programs.length"
        class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3"
      >
        <landing-program-card
          v-for="program in programs"
          :key="program.id"
          :program="program"
        />
      </div>

      <p
        v-else
        class="py-8 text-sm text-neutral-500"
      >
        No programs are accepting applications right now.
      </p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import LandingProgramCard from './landing-program-card.vue';
import { LANDING_PROGRAMS_PREVIEW_COUNT } from '../config/landing.config';
import { usePrograms } from '~/composables/programs/usePrograms';
import { AppRoute } from '~/config/routes';
import LfxIcon from '~/components/uikit/icon/icon.vue';

const { data, isLoading, error } = usePrograms({
  search: '',
  status: 'acceptance',
  skill: 'all',
  sortBy: 'accepting_first',
});

const programs = computed(
  () => data.value?.data.slice(0, LANDING_PROGRAMS_PREVIEW_COUNT) ?? [],
);

const acceptingCountLabel = computed(() => {
  const total = data.value?.total ?? data.value?.programCount;
  return total != null ? String(total) : '';
});
</script>

<script lang="ts">
export default {
  name: 'LandingPrograms',
};
</script>
