<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="space-y-6">
    <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
      <div class="space-y-1 min-w-0">
        <h2 class="text-base font-semibold text-neutral-900">
          {{ ADMIN_MY_PROGRAMS_HEADING }}
        </h2>
        <p class="text-sm text-neutral-500">
          {{ adminMyProgramsSubcopy(programs.length) }}
        </p>
      </div>

      <div class="flex flex-col gap-3 sm:flex-row sm:items-center shrink-0">
        <lfx-select
          :model-value="statusFilter"
          :placeholder="ADMIN_STATUS_FILTER_PLACEHOLDER"
          class="sm:w-44 bg-white"
          @update:model-value="$emit('update:statusFilter', $event)"
        >
          <lfx-dropdown-item
            v-for="option in ADMIN_STATUS_FILTER_OPTIONS"
            :key="option.value"
            :value="option.value"
            :label="option.label"
          />
        </lfx-select>

        <lfx-input
          :model-value="search"
          placeholder="Search"
          class="sm:w-48 bg-white min-w-[30%]"
          @update:model-value="$emit('update:search', String($event))"
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

        <lfx-button
          label="Enroll a Program"
          icon="plus"
          type="primary"
          button-style="pill"
          class="justify-center w-1/3 whitespace-nowrap"
          @click="$emit('enroll')"
        />
      </div>
    </div>

    <div
      v-if="filteredPrograms.length"
      class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3"
    >
      <admin-program-card
        v-for="program in filteredPrograms"
        :key="program.id"
        :program="program"
        @open="$emit('open', $event)"
        @edit="$emit('edit', $event)"
        @hide="$emit('hide', $event)"
      />
    </div>
    <p
      v-else
      class="py-10 text-sm text-neutral-500"
    >
      No programs match your filters.
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import AdminProgramCard from './admin-program-card.vue';
import {
  ADMIN_MY_PROGRAMS_HEADING,
  ADMIN_STATUS_FILTER_OPTIONS,
  ADMIN_STATUS_FILTER_PLACEHOLDER,
  adminMyProgramsSubcopy,
} from '../config/admin.config';
import type { AdminProgram } from '~/types/admin.types';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxDropdownItem from '~/components/uikit/dropdown/dropdown-item.vue';
import LfxIcon from '~/components/uikit/icon/icon.vue';
import LfxInput from '~/components/uikit/input/input.vue';
import LfxSelect from '~/components/uikit/select/select.vue';

const props = defineProps<{
  programs: AdminProgram[];
  statusFilter: string;
  search: string;
}>();

defineEmits<{
  (e: 'update:statusFilter', value: string): void;
  (e: 'update:search', value: string): void;
  (e: 'enroll'): void;
  (e: 'open', id: string): void;
  (e: 'edit', id: string): void;
  (e: 'hide', id: string): void;
}>();

const filteredPrograms = computed(() => {
  const query = props.search.trim().toLowerCase();
  return props.programs.filter((program) => {
    if (program.hidden) return false;
    if (props.statusFilter !== 'all' && program.status !== props.statusFilter) return false;
    if (!query) return true;
    return (
      program.name.toLowerCase().includes(query) ||
      program.foundationName.toLowerCase().includes(query) ||
      program.termLabel.toLowerCase().includes(query)
    );
  });
});
</script>

<script lang="ts">
export default {
  name: 'AdminProgramsTab',
};
</script>
