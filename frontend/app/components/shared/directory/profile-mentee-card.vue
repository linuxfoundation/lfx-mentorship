<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <NuxtLink
    :to="menteePath(mentee.id)"
    class="block border border-neutral-200 rounded-2xl bg-white p-4 h-full transition-shadow hover:shadow-md"
  >
    <div class="flex items-start gap-3">
      <profile-initials-avatar
        :name="mentee.name"
        :src="mentee.avatarUrl"
        size="large"
      />
      <div class="min-w-0 flex flex-col gap-1">
        <h3 class="text-sm font-semibold text-neutral-900 truncate">
          {{ mentee.name }}
        </h3>
        <p class="text-xs text-neutral-500 truncate">
          {{ mentee.programLabel }}
        </p>
        <p class="text-sm text-neutral-600 leading-5 line-clamp-3 mt-1">
          {{ mentee.bio }}
        </p>
      </div>
    </div>
  </NuxtLink>
</template>

<script setup lang="ts">
import ProfileInitialsAvatar from '~/components/shared/directory/profile-initials-avatar.vue';
import { menteePath } from '~/config/routes';
import type { MentorMenteeSummary } from '~/types/mentor.types';

defineProps<{ mentee: MentorMenteeSummary }>();
</script>

<script lang="ts">
export default {
  name: 'ProfileMenteeCard',
};
</script>
