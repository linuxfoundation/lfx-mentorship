<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <div>
    <div
      v-if="isLoading"
      class="flex items-center justify-center gap-2 text-neutral-500 py-16"
    >
      <lfx-spinner />
      <span>Loading mentees…</span>
    </div>

    <p
      v-else-if="loadFailed"
      class="py-10 text-sm text-neutral-500"
    >
      Unable to load mentees.
    </p>

    <p
      v-else-if="!currentMentees.length && !graduatedMentees.length"
      class="py-10 text-sm text-neutral-500"
    >
      No mentees listed yet.
    </p>

    <div
      v-else
      class="space-y-10"
    >
      <section
        v-if="currentMentees.length"
        aria-labelledby="current-mentees-heading"
      >
        <h3
          id="current-mentees-heading"
          class="text-base font-semibold text-neutral-900 mb-5"
        >
          {{ PROGRAM_CURRENT_MENTEES_HEADING }}
        </h3>
        <program-detail-mentee-table :mentees="currentMentees" />
      </section>

      <section
        v-if="graduatedMentees.length"
        aria-labelledby="graduated-mentees-heading"
      >
        <h3
          id="graduated-mentees-heading"
          class="text-base font-semibold text-neutral-900 mb-5"
        >
          {{ PROGRAM_GRADUATED_MENTEES_HEADING }}
        </h3>
        <program-detail-mentee-table :mentees="graduatedMentees" />
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import ProgramDetailMenteeTable from './program-detail-mentee-table.vue';
import { PROGRAM_CURRENT_MENTEES_HEADING, PROGRAM_GRADUATED_MENTEES_HEADING } from '../config/program-detail.config';
import type { ProgramMentee } from '~/types/program.types';
import LfxSpinner from '~/components/uikit/spinner/spinner.vue';

defineProps<{
  currentMentees: ProgramMentee[];
  graduatedMentees: ProgramMentee[];
  isLoading?: boolean;
  loadFailed?: boolean;
}>();
</script>

<script lang="ts">
export default {
  name: 'ProgramDetailMentees',
};
</script>
