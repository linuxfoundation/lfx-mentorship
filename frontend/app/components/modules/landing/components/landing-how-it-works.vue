<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="container px-5 pb-10 md:px-10 md:pb-12">
    <div class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8">
      <div class="mb-8">
        <h2 class="font-secondary text-xl md:text-2xl font-normal text-neutral-900">How It Works</h2>
      </div>

      <ol class="grid grid-cols-1 gap-8 sm:grid-cols-2 lg:grid-cols-5 lg:gap-4">
        <li
          v-for="step in LANDING_HOW_IT_WORKS_STEPS"
          :key="step.step"
          class="flex flex-col gap-3"
        >
          <span
            class="flex size-8 items-center justify-center rounded-full bg-brand-50 text-sm font-semibold text-brand-600"
          >
            {{ step.step }}
          </span>
          <div>
            <h3 class="text-sm font-semibold text-neutral-900">{{ step.title }}</h3>
            <p class="mt-1 text-xs text-neutral-500 leading-5">{{ step.description }}</p>
          </div>
        </li>
      </ol>
    </div>
  </section>
</template>

<script setup lang="ts">
import { LANDING_HOW_IT_WORKS_STEPS } from '../config/landing.config';
</script>

<script lang="ts">
export default {
  name: 'LandingHowItWorks',
};
</script>
