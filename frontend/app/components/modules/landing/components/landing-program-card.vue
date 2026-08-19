<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <NuxtLink
    :to="programPath(program.id)"
    class="block h-full rounded-2xl border border-neutral-200 bg-white p-5 transition-shadow duration-200 hover:shadow-lg"
  >
    <div class="flex items-start justify-between gap-3 mb-3">
      <p class="text-sm text-neutral-500 truncate">{{ program.foundation.name }}</p>
      <lfx-tag
        :variation="statusConfig.variation"
        size="small"
        type="solid"
        class="shrink-0"
      >
        {{ statusConfig.label }}
      </lfx-tag>
    </div>

    <h3 class="text-base font-semibold text-neutral-900 leading-6 line-clamp-3 mb-2">
      {{ program.name }}
    </h3>
    <p class="text-sm text-neutral-600 leading-5 line-clamp-3 mb-4">
      {{ plainDescription }}
    </p>

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
    </div>
  </NuxtLink>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { PROGRAM_SKILLS_VISIBLE_COUNT, PROGRAM_STATUS_CONFIG } from '~/components/modules/programs/config/program-card.config';
import type { Program } from '~/types/program.types';
import { programPath } from '~/config/routes';
import LfxChip from '~/components/uikit/chip/chip.vue';
import LfxTag from '~/components/uikit/tag/tag.vue';

const props = defineProps<{ program: Program }>();

const statusConfig = computed(() => PROGRAM_STATUS_CONFIG[props.program.status]);
const visibleSkills = computed(() =>
  props.program.skills.slice(0, PROGRAM_SKILLS_VISIBLE_COUNT + 1),
);

const plainDescription = computed(() =>
  props.program.description.replace(/<[^>]*>/g, '').trim(),
);
</script>

<script lang="ts">
export default {
  name: 'LandingProgramCard',
};
</script>
