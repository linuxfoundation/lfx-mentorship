<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="container px-5 pb-10 md:px-10 md:pb-12">
    <ul class="grid grid-cols-2 gap-4 lg:grid-cols-4">
      <li
        v-for="stat in LANDING_STATS"
        :key="stat.label"
        class="rounded-2xl border border-neutral-200 bg-white px-5 py-6"
      >
        <p class="text-3xl md:text-4xl font-semibold text-neutral-900 leading-none">
          {{ stat.value }}
        </p>
        <p class="mt-2 text-sm text-neutral-500">{{ stat.label }}</p>
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import { LANDING_STATS } from '../config/landing.config';
</script>

<script lang="ts">
export default {
  name: 'LandingStats',
};
</script>
