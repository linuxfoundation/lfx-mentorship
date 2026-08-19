<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="space-y-5">
    <div class="flex items-center justify-between gap-3">
      <h2 class="text-lg font-semibold text-neutral-900">Terms Management</h2>
      <lfx-button
        label="Create Term"
        icon="plus"
        type="outline"
        button-style="rounded"
        @click="$emit('create-term')"
      />
    </div>

    <lfx-table>
      <thead>
        <tr>
          <th>Term</th>
          <th>Status</th>
          <th>Pending</th>
          <th>Declined</th>
          <th>Accepted</th>
          <th>Graduated</th>
          <th>Dates</th>
          <th>Application Dates</th>
          <th />
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="term in rows"
          :key="term.id"
        >
          <td>
            <button
              type="button"
              class="text-sm font-medium text-brand-600 hover:text-brand-700"
              @click="$emit('open-term', term.id)"
            >
              {{ term.name }}
            </button>
          </td>
          <td class="text-brand-600">{{ ADMIN_TERM_STATUS_LABEL[term.status] }}</td>
          <td class="font-semibold text-warning-700">{{ term.pending }}</td>
          <td class="font-semibold text-negative-600">{{ term.declined }}</td>
          <td class="font-semibold text-positive-600">{{ term.accepted }}</td>
          <td class="font-semibold text-positive-600">{{ term.graduated }}</td>
          <td class="text-xs text-neutral-500 whitespace-nowrap">
            <p>Start: {{ term.startLabel }}</p>
            <p>End: {{ term.endLabel }}</p>
          </td>
          <td class="text-xs text-neutral-500 whitespace-nowrap">
            <p>Start: {{ term.applicationStartLabel }}</p>
            <p>End: {{ term.applicationEndLabel }}</p>
          </td>
          <td class="text-right">
            <lfx-icon-button
              icon="ellipsis"
              type="transparent"
              size="small"
              @click="$emit('term-actions', term.id)"
            />
          </td>
        </tr>
        <tr v-if="!rows.length">
          <td
            colspan="9"
            class="text-neutral-500"
          >
            No terms yet.
          </td>
        </tr>
      </tbody>
    </lfx-table>
  </div>
</template>

<script setup lang="ts">
import { ADMIN_TERM_STATUS_LABEL } from '../config/admin-program-detail.config';
import type { AdminManagedTerm } from '~/types/admin.types';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxIconButton from '~/components/uikit/icon-button/icon-button.vue';
import LfxTable from '~/components/uikit/table/table.vue';

defineProps<{ rows: AdminManagedTerm[] }>();

defineEmits<{
  (e: 'create-term'): void;
  (e: 'open-term', id: string): void;
  (e: 'term-actions', id: string): void;
}>();
</script>

<script lang="ts">
export default {
  name: 'AdminProgramDetailTerms',
};
</script>
