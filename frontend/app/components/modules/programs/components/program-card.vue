<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <article
    class="relative flex flex-col justify-between border border-neutral-200 rounded-2xl p-6 h-full min-h-[260px] bg-white transition-shadow duration-200 hover:shadow-lg"
  >
    <lfx-tag
      class="absolute top-6 right-6"
      :variation="statusConfig.variation"
      size="small"
      type="outline"
    >
      {{ statusConfig.label }}
    </lfx-tag>

    <div class="flex flex-col gap-6 w-full">
      <div class="flex items-center gap-4 pr-24">
        <lfx-avatar
          :src="program.logoUrl"
          type="organization"
          size="xlarge"
        />
        <div class="flex flex-col max-w-[calc(100%-64px)]">
          <p class="text-sm text-neutral-500 leading-5 truncate">
            {{ program.foundation.name }} · {{ program.activeTerm.name }}
          </p>
          <h3 class="text-lg font-semibold text-neutral-900 leading-7 truncate">
            {{ program.name }}
          </h3>
        </div>
      </div>

      <div class="flex flex-col gap-4 w-full">
        <div class="flex flex-col gap-1 w-full">
          <p class="text-sm text-neutral-600 leading-5 line-clamp-2">
            {{ plainDescription }}
          </p>
        </div>

        <div
          v-if="program.skills.length"
          class="flex flex-wrap gap-2"
        >
          <lfx-chip
            v-for="skill in visibleSkills"
            :key="skill"
            type="bordered"
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
              type="bordered"
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
    </div>

    <div class="flex items-center justify-between gap-4 w-full pt-6 mt-6 border-t border-neutral-100">
      <div class="flex items-center gap-4 min-w-0">
        <lfx-popover
          v-for="group in memberGroups"
          :key="group.key"
          placement="top-start"
          trigger-event="hover"
          :spacing="8"
          popover-class="!p-0"
        >
          <div
            class="inline-flex items-center gap-2 text-sm text-neutral-600 cursor-default"
            @click.stop
          >
            <lfx-icon
              :name="group.icon"
              type="light"
              :size="16"
            />
            <span>{{ group.countLabel }}</span>
          </div>

          <template #content>
            <div class="bg-white border border-neutral-200 rounded-lg shadow-lg p-4 min-w-48">
              <p class="text-xs font-semibold uppercase tracking-wide text-neutral-500 mb-2">
                {{ group.title }}
              </p>
              <ul
                v-if="group.members.length"
                class="flex flex-col gap-1"
              >
                <li
                  v-for="member in group.members"
                  :key="member.id"
                  class="text-sm text-neutral-800"
                >
                  {{ member.name }}
                </li>
              </ul>
              <p
                v-else
                class="text-sm text-neutral-400"
              >
                None yet
              </p>
            </div>
          </template>
        </lfx-popover>
      </div>

      <NuxtLink :to="programPath(program.id)">
        <lfx-button
          label="View program"
          type="secondary"
          button-style="pill"
          size="small"
        />
      </NuxtLink>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import {
  PROGRAM_SKILLS_VISIBLE_COUNT,
  PROGRAM_STATUS_CONFIG,
} from '../config/program-card.config';
import { programPath } from '~/config/routes';
import type { Program, ProgramMember } from '~/types/program.types';
import LfxAvatar from '~/components/uikit/avatar/avatar.vue';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxChip from '~/components/uikit/chip/chip.vue';
import LfxIcon from '~/components/uikit/icon/icon.vue';
import LfxPopover from '~/components/uikit/popover/popover.vue';
import LfxTag from '~/components/uikit/tag/tag.vue';
import LfxTooltip from '~/components/uikit/tooltip/tooltip.vue';
import { useSanitize } from '~/composables/useSanitize';

const props = defineProps<{ program: Program }>();

const { stripHtml } = useSanitize();

const plainDescription = computed(() => stripHtml(props.program.description ?? ''));
const statusConfig = computed(() => PROGRAM_STATUS_CONFIG[props.program.status]);
const visibleSkills = computed(() => props.program.skills.slice(0, PROGRAM_SKILLS_VISIBLE_COUNT));
const overflowSkills = computed(() => props.program.skills.slice(PROGRAM_SKILLS_VISIBLE_COUNT));
const memberGroups = computed(() => [
  memberGroup('mentees', 'Mentees', 'mentee', 'user-graduate', props.program.mentees),
  memberGroup('mentors', 'Mentors', 'mentor', 'user-tie', props.program.mentors),
]);

function memberGroup(
  key: string,
  title: string,
  singular: string,
  icon: string,
  members: ProgramMember[],
) {
  const count = members.length;
  return {
    key,
    title,
    icon,
    members,
    countLabel: `${count} ${count === 1 ? singular : `${singular}s`}`,
  };
}
</script>

<script lang="ts">
export default {
  name: 'ProgramCard',
};
</script>
