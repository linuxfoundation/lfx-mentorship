<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="container px-5 pb-10 md:px-10 md:pb-12">
    <ul class="grid grid-cols-2 gap-4 lg:grid-cols-4">
      <li
        v-for="stat in stats"
        :key="stat.label"
        class="rounded-2xl border border-neutral-200 bg-white px-5 py-6"
      >
        <p class="text-2xl font-semibold text-neutral-900 leading-none">
          {{ stat.value }}
        </p>
        <p class="mt-2 text-xs text-neutral-500">{{ stat.label }}</p>
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { LANDING_STATS } from '../config/landing.config';
import type { LandingStat, LandingSummaryResponse } from '~/types/landing.types';
import { formatCompactUsd } from '~/utils/currency';


const props = defineProps<{
  summary: LandingSummaryResponse;
}>();
// Fixed order shared with LANDING_STATS: Accepting, Graduated, Mentors, Total.
// Any missing metric falls back to the placeholder from the config so the
// grid keeps its shape while the request is in flight (or fails).
const stats = computed(() => {
  return LANDING_STATS.map((stat) => {
    const value = props.summary[stat.key] ?? stat.value;
    return {
      ...stat,
      value: typeof value === 'number' ? formatCount(value, stat.key) : stat.value,
    };
  });
});

function formatCount(value: number, key: LandingStat['key']): string {
  if (key === 'stipendsPaid') {
    return formatCompactUsd(value);
  }
  return new Intl.NumberFormat('en-US').format(value);
}
</script>

<script lang="ts">
export default {
  name: 'LandingStats',
};
</script>
