<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div>
    <p
      v-if="!currentMentees.length && !graduatedMentees.length"
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

defineProps<{
  currentMentees: ProgramMentee[];
  graduatedMentees: ProgramMentee[];
}>();
</script>

<script lang="ts">
export default {
  name: 'ProgramDetailMentees',
};
</script>
