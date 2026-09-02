<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="space-y-6">
    <p class="text-sm text-neutral-600 max-w-3xl">
      {{ PROGRAM_SPONSORS_INTRO }}
    </p>

    <div
      v-if="!sponsors.length"
      class="py-6 text-sm text-neutral-500"
    >
      No sponsors listed yet.
    </div>

    <ul
      v-else
      class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4"
    >
      <li
        v-for="sponsor in sponsors"
        :key="sponsor.id"
        class="flex flex-col items-center justify-center rounded-xl border border-neutral-200 bg-white p-4 text-center"
      >
        <p class="text-sm font-semibold text-neutral-900 break-word">{{ sponsor.name }}</p>
        <p class="mt-1 text-xs text-neutral-500">
          {{ formatUsdFromCents(sponsor.amountCents) }}
        </p>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { PROGRAM_SPONSORS_INTRO } from '../config/program-detail.config';
import type { ProgramSponsor } from '~/types/program.types';
import { formatUsdFromCents } from '~/utils/currency';

defineProps<{ sponsors: ProgramSponsor[] }>();
</script>

<script lang="ts">
export default {
  name: 'ProgramDetailSponsors',
};
</script>
