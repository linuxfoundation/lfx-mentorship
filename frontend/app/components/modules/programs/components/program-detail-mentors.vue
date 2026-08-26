<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div>
    <h3
      v-if="heading"
      class="text-base font-semibold text-neutral-900 mb-4"
    >
      {{ heading }}
    </h3>

    <div
      v-if="!mentors.length"
      class="py-10 text-sm text-neutral-500"
    >
      No {{ emptyLabel }} listed yet.
    </div>

    <ul
      v-else
      class="grid grid-cols-1 gap-4 sm:grid-cols-2"
    >
      <li
        v-for="member in mentors"
        :key="member.id"
        class="flex flex-col gap-3 rounded-xl border border-neutral-200 p-4"
      >
        <div class="min-w-0 flex items-center gap-3">
          <lfx-avatar
            :src="member.avatarUrl"
            type="member"
            size="normal"
          />
          <NuxtLink
            :to="mentorProfilePath(member.id)"
            class="block text-sm font-semibold text-brand-700 truncate hover:underline"
          >
            {{ member.name }}
          </NuxtLink>
        </div>
        <p
          v-if="member.intro"
          class="text-xs text-neutral-500 line-clamp-3"
        >
          {{ member.intro }}
        </p>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import type { ProgramMember } from '~/types/program.types';
import LfxAvatar from '~/components/uikit/avatar/avatar.vue';

defineProps<{
  mentors: ProgramMember[];
  emptyLabel: string;
  heading?: string;
}>();

const mentorProfilePath = (mentorId: string) => `/mentor/${mentorId}`;
</script>

<script lang="ts">
export default {
  name: 'ProgramDetailMentors',
};
</script>
