<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <article
    class="relative flex flex-col justify-between border border-neutral-200 rounded-2xl p-6 h-full min-h-[360px] bg-white transition-shadow duration-200 hover:shadow-lg"
  >
    <lfx-tag
      class="absolute top-6 right-6"
      :variation="statusConfig.variation"
      size="small"
      type="solid"
    >
      {{ statusConfig.label }}
    </lfx-tag>

    <div class="flex flex-col gap-5 w-full">
      <div class="flex items-start gap-3 pr-24">
        <profile-initials-avatar
          :name="mentee.name"
          size="large"
        />
        <div class="flex flex-col gap-0.5 min-w-0">
          <h3 class="text-lg font-semibold text-neutral-900 leading-7 truncate">
            {{ mentee.name }}
          </h3>
          <p class="text-sm text-neutral-500 leading-5">
            {{ mentee.sinceLabel }}
          </p>
        </div>
      </div>

      <p class="text-sm text-neutral-600 leading-5 line-clamp-3">
        {{ mentee.bio }}
      </p>

      <div
        v-if="mentee.skills.length"
        class="flex flex-wrap gap-2"
      >
        <lfx-chip
          v-for="skill in visibleSkills"
          :key="skill"
          type="default"
          size="xsmall"
        >
          {{ skill }}
        </lfx-chip>
        <lfx-tooltip
          v-if="overflowSkills.length"
          placement="top"
          class="inline-flex"
        >
          <lfx-chip
            type="default"
            size="xsmall"
          >
            +{{ overflowSkills.length }}
          </lfx-chip>
          <template #content>
            <div class="flex flex-col gap-1 text-xs">
              <span
                v-for="skill in overflowSkills"
                :key="skill"
              >
                {{ skill }}
              </span>
            </div>
          </template>
        </lfx-tooltip>
      </div>
    </div>

    <div class="flex flex-col gap-4 w-full pt-6 mt-6 border-t border-neutral-100">
      <div class="grid grid-cols-2 gap-4">
        <div class="flex flex-col gap-1 min-w-0">
          <p class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
            Project
          </p>
          <p class="text-sm font-medium text-neutral-900 truncate">
            {{ mentee.project.foundationLabel }} · {{ mentee.project.name }}
          </p>
        </div>
        <div class="flex flex-col gap-1">
          <p class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
            Mentors
          </p>
          <div
            v-if="mentee.mentors.length"
            class="flex items-center -space-x-2"
          >
            <profile-initials-avatar
              v-for="mentor in visibleMentors"
              :key="mentor.id"
              :name="mentor.name"
              size="small"
              class="ring-2 ring-white"
            />
            <span
              v-if="overflowMentors.length"
              class="inline-flex h-8 w-8 items-center justify-center rounded-full bg-neutral-100 text-xs font-medium text-neutral-600 ring-2 ring-white"
            >
              +{{ overflowMentors.length }}
            </span>
          </div>
          <p
            v-else
            class="text-sm text-neutral-400"
          >
            None yet
          </p>
        </div>
      </div>

      <NuxtLink
        :to="menteePath(mentee.id)"
        class="w-full"
      >
        <lfx-button
          label="View Profile"
          type="secondary"
          button-style="rounded"
          size="medium"
          class="!w-full justify-center"
        />
      </NuxtLink>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import {
  MENTEE_MENTORS_VISIBLE_COUNT,
  MENTEE_SKILLS_VISIBLE_COUNT,
  MENTEE_STATUS_CONFIG,
} from '../config/mentee-card.config';
import { menteePath } from '~/config/routes';
import type { Mentee } from '~/types/mentee.types';
import ProfileInitialsAvatar from '~/components/shared/directory/profile-initials-avatar.vue';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxChip from '~/components/uikit/chip/chip.vue';
import LfxTag from '~/components/uikit/tag/tag.vue';
import LfxTooltip from '~/components/uikit/tooltip/tooltip.vue';

const props = defineProps<{ mentee: Mentee }>();

const statusConfig = computed(() => MENTEE_STATUS_CONFIG[props.mentee.status]);
const visibleSkills = computed(() => props.mentee.skills.slice(0, MENTEE_SKILLS_VISIBLE_COUNT));
const overflowSkills = computed(() => props.mentee.skills.slice(MENTEE_SKILLS_VISIBLE_COUNT));
const visibleMentors = computed(() =>
  props.mentee.mentors.slice(0, MENTEE_MENTORS_VISIBLE_COUNT),
);
const overflowMentors = computed(() =>
  props.mentee.mentors.slice(MENTEE_MENTORS_VISIBLE_COUNT),
);
</script>

<script lang="ts">
export default {
  name: 'MenteeCard',
};
</script>
