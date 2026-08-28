<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <article class="border border-neutral-200 rounded-2xl bg-white p-5 md:p-6 flex flex-col gap-5 h-full">
    <div class="flex items-center gap-3 relative">
      <lfx-avatar
        :src="program.logoUrl"
        size="large"
        type="organization"
      />
      <div class="flex flex-col gap-1">
        <p class="text-xs text-neutral-500 leading-4 truncate pr-12">{{ program.foundationLabel }}</p>
        <h3 class="text-base font-normal text-neutral-900 leading-6 break-words">
          {{ program.title }}
        </h3>
      </div>
      <lfx-tag
        class="shrink-0 absolute top-0 right-0"
        :variation="statusConfig.variation"
        size="small"
        type="solid"
      >
        {{ statusConfig.label }}
      </lfx-tag>
    </div>

    <p class="text-sm text-neutral-600 leading-5">
      {{ program.description }}
    </p>

    <div
      v-if="program.skills.length"
      class="flex flex-col gap-2"
    >
      <p class="text-xs font-semibold uppercase tracking-wide text-neutral-500">Required skills</p>
      <div class="flex flex-wrap gap-2">
        <lfx-tag
          v-for="skill in program.skills"
          :key="skill"
          variation="neutral"
          type="outline"
          size="small"
        >
          {{ skill }}
        </lfx-tag>
      </div>
    </div>

    <div
      v-if="program.terms.length"
      class="flex flex-col gap-2"
    >
      <p class="text-xs font-semibold uppercase tracking-wide text-neutral-500">Terms</p>
      <div class="flex flex-wrap gap-2">
        <div
          v-for="term in program.terms"
          :key="term.id"
          class="rounded-lg border border-neutral-200 bg-neutral-50 px-3 py-2 min-w-[9rem]"
        >
          <p class="text-xs font-normal text-neutral-900">{{ term.label }}</p>
          <p
            v-if="term.dateRangeLabel"
            class="text-xxs text-neutral-500 mt-0.5"
          >
            {{ term.dateRangeLabel }}
          </p>
        </div>
      </div>
    </div>

    <div
      v-if="program.mentors?.length"
      class="flex flex-col gap-3"
    >
      <p class="text-xs font-semibold uppercase tracking-wide text-neutral-500">Mentors</p>
      <ul class="flex flex-col gap-4 sm:flex-row sm:flex-wrap sm:gap-x-8 sm:gap-y-4">
        <li
          v-for="mentor in program.mentors"
          :key="mentor.id"
          class="flex items-center gap-3 min-w-0"
        >
          <profile-initials-avatar
            :name="mentor.name"
            :src="mentor.avatarUrl"
            size="small"
          />
          <div class="min-w-0">
            <p class="text-xs font-normal text-neutral-900 truncate">{{ mentor.name }}</p>
            <p
              v-if="mentor.title"
              class="text-xxs text-neutral-500 truncate"
            >
              {{ mentor.title }}
            </p>
          </div>
        </li>
      </ul>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { PROFILE_PROGRAM_STATUS_CONFIG } from '~/components/modules/mentees/config/mentee-detail.config';
import ProfileInitialsAvatar from '~/components/shared/directory/profile-initials-avatar.vue';
import type { ProfileProgram } from '~/types/mentee.types';
import LfxTag from '~/components/uikit/tag/tag.vue';

const props = defineProps<{ program: ProfileProgram }>();

const statusConfig = computed(() => PROFILE_PROGRAM_STATUS_CONFIG[props.program.status]);
</script>

<script lang="ts">
export default {
  name: 'ProfileProgramCard',
};
</script>
