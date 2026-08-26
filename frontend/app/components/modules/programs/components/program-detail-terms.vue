<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="space-y-6">
    <p class="text-sm text-neutral-600 max-w-3xl">
      {{ PROGRAM_TERMS_INTRO }}
    </p>

    <div
      v-if="!sortedTerms.length"
      class="py-6 text-sm text-neutral-500"
    >
      No terms listed yet.
    </div>

    <lfx-table v-else>
      <thead>
        <tr>
          <th class="w-[40%]">Term</th>
          <th>Dates</th>
          <th>Applications</th>
          <th>Status</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="term in sortedTerms"
          :key="term.id"
        >
          <td class="!text-neutral-900 !font-medium truncate w-[40%] max-w-0">
            {{ term.name }}
          </td>
          <td class="text-neutral-600 whitespace-nowrap">
            {{ formatProgramDateRange(term.startsAt, term.endsAt) }}
          </td>
          <td class="text-neutral-600 whitespace-nowrap">
            <template v-if="term.applicationsStartAt && term.applicationsCloseAt">
              {{ formatProgramDateRange(term.applicationsStartAt, term.applicationsCloseAt) }}
            </template>
            <span
              v-else
              class="text-neutral-400"
              >—</span
            >
          </td>
          <td>
            <lfx-tag
              :variation="statusConfig(term).variation"
              size="small"
              type="solid"
              rounded-size="full"
            >
              {{ statusConfig(term).label }}
            </lfx-tag>
          </td>
        </tr>
      </tbody>
    </lfx-table>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { PROGRAM_TERMS_INTRO, PROGRAM_TERM_STATUS_CONFIG } from '../config/program-detail.config';
import type { ProgramTerm } from '~/types/program.types';
import { formatProgramDateRange } from '~/utils/date';
import { getProgramTermDisplayStatus, sortTermsNewestFirst } from '~/utils/program-terms';
import LfxTable from '~/components/uikit/table/table.vue';
import LfxTag from '~/components/uikit/tag/tag.vue';

const props = defineProps<{
  terms: ProgramTerm[];
}>();

const sortedTerms = computed(() => sortTermsNewestFirst(props.terms));

function statusConfig(term: ProgramTerm) {
  return PROGRAM_TERM_STATUS_CONFIG[getProgramTermDisplayStatus(term)];
}
</script>

<script lang="ts">
export default {
  name: 'ProgramDetailTerms',
};
</script>
