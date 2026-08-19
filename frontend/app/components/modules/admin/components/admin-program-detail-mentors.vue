<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="space-y-5">
    <div class="flex flex-col gap-3 lg:flex-row lg:items-center">
      <lfx-input
        v-model="search"
        placeholder="Search"
        class="bg-white lg:flex-1"
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

      <lfx-input
        v-model="inviteQuery"
        placeholder="Search and select mentors..."
        class="bg-white lg:w-72"
      >
        <template #suffix>
          <lfx-icon
            name="chevron-down"
            type="light"
            :size="12"
            class="text-neutral-400"
          />
        </template>
      </lfx-input>

      <lfx-button
        label="Invite"
        icon="plus"
        type="outline"
        button-style="rounded"
        @click="$emit('invite')"
      />
    </div>

    <lfx-table>
      <thead>
        <tr>
          <th>Mentor</th>
          <th>Status</th>
          <th>Invitation / Application</th>
          <th>Profile Created?</th>
          <th />
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="row in visibleRows"
          :key="row.id"
        >
          <td>
            <div class="flex items-center gap-3 min-w-0">
              <profile-initials-avatar
                :name="row.name"
                size="small"
              />
              <button
                type="button"
                class="text-sm font-medium text-brand-600 hover:text-brand-700 truncate"
                @click="$emit('open-mentor', row.id)"
              >
                {{ row.name }}
              </button>
            </div>
          </td>
          <td>
            <lfx-tag
              :variation="ADMIN_MENTOR_STATUS_CONFIG[row.status].variation"
              size="small"
              type="solid"
            >
              {{ ADMIN_MENTOR_STATUS_CONFIG[row.status].label }}
            </lfx-tag>
          </td>
          <td class="text-neutral-600">{{ row.entryLabel }}</td>
          <td
            class="font-semibold"
            :class="row.profileCreated ? 'text-positive-600' : 'text-negative-600'"
          >
            {{ row.profileCreated ? 'YES' : 'NO' }}
          </td>
          <td class="text-right">
            <lfx-icon-button
              icon="trash"
              type="transparent"
              size="small"
              class="!text-negative-600"
              @click="$emit('remove', row.id)"
            />
          </td>
        </tr>
        <tr v-if="!filteredRows.length">
          <td
            colspan="5"
            class="text-neutral-500"
          >
            No mentors match your search.
          </td>
        </tr>
      </tbody>
    </lfx-table>

    <div
      v-if="hasMore"
      class="flex justify-center pt-2"
    >
      <lfx-button
        label="Load More"
        type="outline"
        button-style="rounded"
        @click="loadMore"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import {
  ADMIN_MENTOR_PAGE_SIZE,
  ADMIN_MENTOR_STATUS_CONFIG,
} from '../config/admin-program-detail.config';
import ProfileInitialsAvatar from '~/components/shared/directory/profile-initials-avatar.vue';
import type { AdminProgramMentorRow } from '~/types/admin.types';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxIcon from '~/components/uikit/icon/icon.vue';
import LfxIconButton from '~/components/uikit/icon-button/icon-button.vue';
import LfxInput from '~/components/uikit/input/input.vue';
import LfxTable from '~/components/uikit/table/table.vue';
import LfxTag from '~/components/uikit/tag/tag.vue';

const props = defineProps<{ rows: AdminProgramMentorRow[] }>();

defineEmits<{
  (e: 'invite'): void;
  (e: 'open-mentor', id: string): void;
  (e: 'remove', id: string): void;
}>();

const search = ref('');
const inviteQuery = ref('');
const visibleCount = ref(ADMIN_MENTOR_PAGE_SIZE);

const filteredRows = computed(() => {
  const query = search.value.trim().toLowerCase();
  if (!query) return props.rows;
  return props.rows.filter(
    (row) =>
      row.name.toLowerCase().includes(query) ||
      row.entryLabel.toLowerCase().includes(query) ||
      row.status.toLowerCase().includes(query),
  );
});

const visibleRows = computed(() => filteredRows.value.slice(0, visibleCount.value));
const hasMore = computed(() => visibleCount.value < filteredRows.value.length);

watch(search, () => {
  visibleCount.value = ADMIN_MENTOR_PAGE_SIZE;
});

function loadMore() {
  visibleCount.value += ADMIN_MENTOR_PAGE_SIZE;
}
</script>

<script lang="ts">
export default {
  name: 'AdminProgramDetailMentors',
};
</script>
