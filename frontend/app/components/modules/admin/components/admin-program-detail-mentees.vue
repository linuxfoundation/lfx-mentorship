<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="space-y-5">
    <div class="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
      <lfx-input
        v-model="search"
        placeholder="Search"
        class="bg-white xl:max-w-sm xl:flex-1"
      >
        <template #prefix>
          <lfx-icon
            name="magnifying-glass"
            type="light"
            :size="14"
            class="text-neutral-400"
          />
        </template>
      </lfx-input>

      <div class="flex flex-wrap items-center gap-2">
        <template v-if="variant === 'current'">
          <lfx-button
            label="Add Task"
            icon="plus"
            type="outline"
            button-style="rounded"
            size="small"
            @click="$emit('add-task')"
          />
          <lfx-button
            label="Decline by Term"
            type="outline"
            button-style="rounded"
            size="small"
            class="!text-warning-700 !border-warning-300"
            @click="$emit('decline-by-term')"
          />
        </template>

        <lfx-select
          v-model="statusFilter"
          class="bg-white min-w-[10rem]"
          is-fit-width
        >
          <lfx-dropdown-item
            v-for="option in ADMIN_MENTEE_STATUS_FILTER_OPTIONS"
            :key="option.value"
            :value="option.value"
            :label="option.label"
          />
        </lfx-select>

        <lfx-select
          v-model="termFilter"
          class="bg-white min-w-[10rem]"
          is-fit-width
        >
          <lfx-dropdown-item
            v-for="option in ADMIN_MENTEE_TERM_FILTER_OPTIONS"
            :key="option.value"
            :value="option.value"
            :label="option.label"
          />
        </lfx-select>

        <lfx-button
          label="Download By Status"
          icon="download"
          type="outline"
          button-style="rounded"
          size="small"
          @click="$emit('download')"
        />
      </div>
    </div>

    <div
      v-if="variant === 'current'"
      class="rounded-xl border border-neutral-200 bg-neutral-50 px-4 py-3 text-sm text-neutral-600 leading-5"
    >
      <span class="font-semibold text-neutral-800">Note:</span>
      {{ ADMIN_CURRENT_MENTEES_NOTE }}
    </div>

    <lfx-table>
      <thead>
        <tr>
          <th>Mentee</th>
          <th>Term</th>
          <th>Application Status</th>
          <th>Application Dates</th>
          <th>Other Active Applications</th>
          <th />
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="row in pageRows"
          :key="row.id"
        >
          <td>
            <div class="flex items-start gap-3 min-w-0">
              <profile-initials-avatar
                :name="row.name"
                size="small"
              />
              <div class="min-w-0">
                <button
                  type="button"
                  class="text-sm font-medium text-brand-600 hover:text-brand-700 truncate"
                  @click="$emit('open-mentee', row.id)"
                >
                  {{ row.name }}
                </button>
                <button
                  type="button"
                  class="block text-xs text-neutral-400 hover:text-neutral-600 mt-0.5"
                  @click="$emit('add-note', row.id)"
                >
                  Add note
                </button>
              </div>
            </div>
          </td>
          <td class="text-neutral-600 whitespace-nowrap">{{ row.termLabel }}</td>
          <td>
            <lfx-tag
              :variation="statusConfig(row.status).variation"
              size="small"
              type="solid"
            >
              {{ statusConfig(row.status).label }}
            </lfx-tag>
          </td>
          <td class="text-xs text-neutral-500 whitespace-nowrap">
            <p>Created: {{ row.createdLabel }}</p>
            <p>Updated: {{ row.updatedLabel }}</p>
          </td>
          <td>
            <div
              v-if="row.otherApplications.length"
              class="flex flex-col gap-1"
            >
              <p
                v-for="(app, index) in row.otherApplications"
                :key="`${row.id}-${index}`"
                class="text-sm"
              >
                <span class="text-brand-600">{{ app.programName }}</span>
                <span class="text-neutral-500"> — {{ statusConfig(app.status).label }}</span>
              </p>
            </div>
            <span
              v-else
              class="text-neutral-400"
            >—</span>
          </td>
          <td class="text-right">
            <lfx-button
              label="View Tasks"
              type="primary"
              button-style="rounded"
              size="small"
              @click="$emit('view-tasks', row.id)"
            />
          </td>
        </tr>
        <tr v-if="!pageRows.length">
          <td
            colspan="6"
            class="text-neutral-500"
          >
            No mentees match your filters.
          </td>
        </tr>
      </tbody>
    </lfx-table>

    <admin-table-pagination
      v-model:page="page"
      :page-size="ADMIN_TABLE_PAGE_SIZE"
      :total="filteredRows.length"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import AdminTablePagination from './admin-table-pagination.vue';
import {
  ADMIN_APPLICATION_STATUS_CONFIG,
  ADMIN_CURRENT_MENTEES_NOTE,
  ADMIN_MENTEE_STATUS_FILTER_OPTIONS,
  ADMIN_MENTEE_TERM_FILTER_OPTIONS,
  ADMIN_TABLE_PAGE_SIZE,
} from '../config/admin-program-detail.config';
import ProfileInitialsAvatar from '~/components/shared/directory/profile-initials-avatar.vue';
import type { AdminApplicationStatus, AdminMenteeApplication } from '~/types/admin.types';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxDropdownItem from '~/components/uikit/dropdown/dropdown-item.vue';
import LfxIcon from '~/components/uikit/icon/icon.vue';
import LfxInput from '~/components/uikit/input/input.vue';
import LfxSelect from '~/components/uikit/select/select.vue';
import LfxTable from '~/components/uikit/table/table.vue';
import LfxTag from '~/components/uikit/tag/tag.vue';

const props = defineProps<{
  rows: AdminMenteeApplication[];
  variant: 'current' | 'past';
}>();

defineEmits<{
  (e: 'add-task'): void;
  (e: 'decline-by-term'): void;
  (e: 'download'): void;
  (e: 'open-mentee', id: string): void;
  (e: 'add-note', id: string): void;
  (e: 'view-tasks', id: string): void;
}>();

const search = ref('');
const statusFilter = ref('all');
const termFilter = ref('all');
const page = ref(1);

const filteredRows = computed(() => {
  const query = search.value.trim().toLowerCase();
  return props.rows.filter((row) => {
    if (statusFilter.value !== 'all' && row.status !== statusFilter.value) return false;
    if (termFilter.value !== 'all' && row.termLabel !== termFilter.value) return false;
    if (!query) return true;
    return (
      row.name.toLowerCase().includes(query) ||
      row.termLabel.toLowerCase().includes(query) ||
      row.otherApplications.some((app) => app.programName.toLowerCase().includes(query))
    );
  });
});

const pageRows = computed(() => {
  const start = (page.value - 1) * ADMIN_TABLE_PAGE_SIZE;
  return filteredRows.value.slice(start, start + ADMIN_TABLE_PAGE_SIZE);
});

watch([search, statusFilter, termFilter], () => {
  page.value = 1;
});

function statusConfig(status: AdminApplicationStatus) {
  return ADMIN_APPLICATION_STATUS_CONFIG[status];
}
</script>

<script lang="ts">
export default {
  name: 'AdminProgramDetailMentees',
};
</script>
