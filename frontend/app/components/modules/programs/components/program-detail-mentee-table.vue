<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="lfx-table">
    <div class="lfx-table-header uppercase tracking-wide">
      <span class="name-col">Mentee</span>
      <span class="value-col">Term</span>
    </div>

    <div
      v-for="mentee in mentees"
      :key="mentee.id"
      class="lfx-table-row py-3"
    >
      <div class="name-col min-w-0">
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
      <p class="value-col text-sm text-neutral-500 shrink-0">
        {{ mentee.termLabel }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import ProfileInitialsAvatar from '~/components/shared/directory/profile-initials-avatar.vue';
import { menteePath } from '~/config/routes';
import type { ProgramMentee } from '~/types/program.types';

defineProps<{
  mentees: ProgramMentee[];
}>();
</script>

<script lang="ts">
export default {
  name: 'ProgramDetailMenteeTable',
};
</script>
