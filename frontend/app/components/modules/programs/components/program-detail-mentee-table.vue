<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <lfx-table>
    <thead>
      <tr>
        <th class="w-[80%]">Mentee</th>
        <th class="w-[20%]">Term</th>
      </tr>
    </thead>
    <tbody>
      <tr
        v-for="mentee in mentees"
        :key="`${mentee.id}-${mentee.termId ?? mentee.termLabel}`"
      >
        <td class="w-[80%] max-w-0">
          <div class="flex items-center gap-3 min-w-0">
            <profile-initials-avatar
              :name="mentee.name"
              size="normal"
            />
            <div class="min-w-0">
              <NuxtLink
                :to="menteePath(mentee.id)"
                class="block text-sm font-semibold text-brand-700 truncate hover:underline"
              >
                {{ mentee.name }}
              </NuxtLink>
              <p class="text-xs text-neutral-500 truncate">
                {{ mentee.email }}
              </p>
            </div>
          </div>
        </td>
        <td class="w-[20%] max-w-0 text-neutral-600 truncate">
          {{ mentee.termLabel }}
        </td>
      </tr>
    </tbody>
  </lfx-table>
</template>

<script setup lang="ts">
import ProfileInitialsAvatar from '~/components/shared/directory/profile-initials-avatar.vue';
import { menteePath } from '~/config/routes';
import type { ProgramMentee } from '~/types/program.types';
import LfxTable from '~/components/uikit/table/table.vue';

defineProps<{
  mentees: ProgramMentee[];
}>();
</script>

<script lang="ts">
export default {
  name: 'ProgramDetailMenteeTable',
};
</script>
