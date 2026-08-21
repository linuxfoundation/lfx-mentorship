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
      v-if="!members.length"
      class="py-10 text-sm text-neutral-500"
    >
      No {{ emptyLabel }} listed yet.
    </div>

    <ul
      v-else
      class="grid grid-cols-1 gap-4 sm:grid-cols-2"
    >
      <li
        v-for="member in members"
        :key="member.id"
        class="flex items-center gap-3 rounded-xl border border-neutral-200 p-4"
      >
        <lfx-avatar
          :src="member.avatarUrl"
          type="member"
          size="normal"
        />
        <div class="min-w-0">
          <p class="text-sm font-semibold text-neutral-900 truncate">{{ member.name }}</p>
          <p
            v-if="member.intro"
            class="text-xs text-neutral-500 line-clamp-3"
          >
            {{ member.intro }}
          </p>
        </div>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import type { ProgramMember } from '~/types/program.types';
import LfxAvatar from '~/components/uikit/avatar/avatar.vue';

defineProps<{
  members: ProgramMember[];
  emptyLabel: string;
  heading?: string;
}>();
</script>

<script lang="ts">
export default {
  name: 'ProgramDetailMembers',
};
</script>
