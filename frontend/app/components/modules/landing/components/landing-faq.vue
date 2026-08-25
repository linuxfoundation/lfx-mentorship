<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="container px-5 pb-10 md:px-10 md:pb-12">
    <div class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8">
      <div class="mb-8 flex items-center justify-between">
        <h2 class="font-secondary text-xl md:text-2xl font-normal text-neutral-900">Frequently Asked</h2>
        <a
          href="https://docs.linuxfoundation.org/lfx/mentorship/mentee-guide/mentee-faqs"
          target="_blank"
        >
          <lfx-button
            label="Full FAQ & documentation"
            type="transparent"
            icon-position="right"
            icon="arrow-up-right-from-square"
          />
        </a>
      </div>

      <ul class="grid grid-cols-1 gap-x-10 gap-y-8 md:grid-cols-2 lg:grid-cols-3">
        <li
          v-for="item in LANDING_FAQ_ITEMS"
          :key="item.question"
        >
          <h3 class="text-sm font-semibold text-neutral-900">{{ item.question }}</h3>
          <p class="mt-2 text-sm text-neutral-600 leading-5">{{ item.answer }}</p>
        </li>
      </ul>
    </div>
  </section>
</template>

<script setup lang="ts">
import { LANDING_FAQ_ITEMS } from '../config/landing.config';
</script>

<script lang="ts">
export default {
  name: 'LandingFaq',
};
</script>
