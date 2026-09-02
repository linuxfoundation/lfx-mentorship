<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <article
    class="relative flex flex-col justify-between border border-neutral-200 rounded-2xl p-6 h-full min-h-[260px] bg-white transition-shadow duration-200 hover:shadow-lg"
  >
    <div class="flex flex-col gap-5 w-full justify-between flex-grow">
      <div class="flex items-start gap-3">
        <profile-initials-avatar
          :name="mentor.name"
          :src="mentor.avatarUrl"
          size="normal"
        />
        <div class="flex flex-col gap-0.5 min-w-0">
          <h3 class="text-base font-normal text-neutral-900 leading-5 truncate">
            {{ mentor.name }}
          </h3>
          <p class="text-xs text-neutral-500 leading-4">
            {{ mentor.sinceLabel }}
          </p>
        </div>
      </div>

      <p class="text-xs text-neutral-600 leading-4 line-clamp-5">
        {{ mentor.bio }}
      </p>

      <div
        v-if="mentor.skills.length"
        class="flex flex-wrap gap-2"
      >
        <lfx-tag
          v-for="skill in visibleSkills"
          :key="skill"
          variation="neutral"
          type="outline"
          size="small"
        >
          {{ skill }}
        </lfx-tag>
        <lfx-tooltip
          v-if="overflowSkills.length"
          placement="top"
          class="inline-flex"
        >
          <lfx-tag
            variation="neutral"
            type="outline"
            size="small"
          >
            +{{ overflowSkills.length }}
          </lfx-tag>
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

    <div class="mt-3.5">
      <NuxtLink
        :to="mentorPath(mentor.id)"
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
import { MENTOR_SKILLS_VISIBLE_COUNT } from '../config/mentor-card.config';
import { mentorPath } from '~/config/routes';
import type { Mentor } from '~/types/mentor.types';
import ProfileInitialsAvatar from '~/components/shared/directory/profile-initials-avatar.vue';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxTooltip from '~/components/uikit/tooltip/tooltip.vue';

const props = defineProps<{ mentor: Mentor }>();

const visibleSkills = computed(() => props.mentor.skills.slice(0, MENTOR_SKILLS_VISIBLE_COUNT));
const overflowSkills = computed(() => props.mentor.skills.slice(MENTOR_SKILLS_VISIBLE_COUNT));
</script>

<script lang="ts">
export default {
  name: 'MentorCard',
};
</script>
