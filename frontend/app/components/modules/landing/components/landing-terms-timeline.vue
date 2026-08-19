<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8 h-full flex flex-col">
    <h2 class="font-secondary text-2xl font-light text-neutral-900 mb-6">Terms & Timeline</h2>

    <ul class="flex flex-col gap-4 flex-1">
      <li
        v-for="term in LANDING_TERMS"
        :key="term.id"
        class="flex items-start justify-between gap-4"
      >
        <div class="min-w-0">
          <p class="text-sm font-semibold text-neutral-900">
            {{ term.name }}
            <span class="font-normal text-neutral-600">{{ term.dateRangeLabel }}</span>
          </p>
          <p class="mt-0.5 text-sm text-neutral-500">{{ term.applicationsLabel }}</p>
        </div>
        <lfx-tag
          :variation="LANDING_TERM_STATUS_CONFIG[term.status].variation"
          size="small"
          type="solid"
          class="shrink-0"
        >
          {{ LANDING_TERM_STATUS_CONFIG[term.status].label }}
        </lfx-tag>
      </li>
    </ul>

    <p class="mt-6 pt-4 border-t border-neutral-100 text-xs text-neutral-500 leading-5">
      {{ LANDING_TERMS_FOOTER }}
    </p>
  </div>
</template>

<script setup lang="ts">
import {
  LANDING_TERMS,
  LANDING_TERMS_FOOTER,
  LANDING_TERM_STATUS_CONFIG,
} from '../config/landing.config';
import LfxTag from '~/components/uikit/tag/tag.vue';
</script>

<script lang="ts">
export default {
  name: 'LandingTermsTimeline',
};
</script>
