<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <article
    class="relative flex cursor-pointer flex-col gap-4 rounded-2xl border border-neutral-200 bg-white transition-shadow hover:shadow-md"
    role="link"
    tabindex="0"
    @click="$emit('open', program.id)"
    @keydown.enter.prevent="$emit('open', program.id)"
  >
  <div class="px-5 pt-5">
    <div class="flex items-start justify-between gap-3">
      <lfx-tag
        :variation="statusConfig.variation"
        size="small"
        type="solid"
      >
        {{ statusConfig.label }}
      </lfx-tag>
      <div
        class="flex items-center gap-3"
        @click.stop
      >
        <lfx-button
          label="Edit"
          icon="pen"
          type="transparent"
          size="small"
          class="!text-brand-600 !px-0"
          @click="$emit('edit', program.id)"
        />
        <lfx-button
          label="Hide"
          icon="eye-slash"
          type="transparent"
          size="small"
          class="!text-negative-600 !px-0"
          @click="$emit('hide', program.id)"
        />
      </div>
    </div>

    <div class="flex items-start gap-3 min-w-0">
      <lfx-avatar
        :src="program.logoUrl"
        type="organization"
        size="normal"
      />
      <div class="min-w-0">
        <h3 class="text-sm font-semibold text-neutral-900 truncate">
          {{ program.name }}
        </h3>
        <p class="text-xs text-neutral-500 mt-0.5 truncate">
          {{ program.foundationName }} · {{ program.termLabel }}
        </p>
      </div>
    </div>
  </div>

    <dl class="grid grid-cols-2 gap-4 pt-2 border-t border-neutral-100 px-5 pb-5 bg-neutral-50 rounded-b-2xl">
      <div>
        <dt class="text-xs text-neutral-500">Mentors</dt>
        <dd class="text-lg font-semibold text-neutral-900">{{ program.stats.mentors }}</dd>
      </div>
      <div>
        <dt class="text-xs text-neutral-500">Current Mentees</dt>
        <dd class="text-lg font-semibold text-neutral-900">
          {{ program.stats.currentMentees }}
        </dd>
      </div>
      <div>
        <dt class="text-xs text-neutral-500">Funding To Date</dt>
        <dd class="text-lg font-semibold text-brand-600">
          {{ formatUsdFromCents(program.stats.fundingToDateCents) }}
        </dd>
      </div>
      <div>
        <dt class="text-xs text-neutral-500">Graduated Mentees</dt>
        <dd class="text-lg font-semibold text-neutral-900">{{ program.stats.graduatedMentees }}</dd>
      </div>
    </dl>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { ADMIN_PROGRAM_STATUS_CONFIG } from '../config/admin.config';
import type { AdminProgram } from '~/types/admin.types';
import { formatUsdFromCents } from '~/utils/currency';
import LfxAvatar from '~/components/uikit/avatar/avatar.vue';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxTag from '~/components/uikit/tag/tag.vue';

const props = defineProps<{ program: AdminProgram }>();

defineEmits<{
  (e: 'open', id: string): void;
  (e: 'edit', id: string): void;
  (e: 'hide', id: string): void;
}>();

const statusConfig = computed(() => ADMIN_PROGRAM_STATUS_CONFIG[props.program.status]);
</script>

<script lang="ts">
export default {
  name: 'AdminProgramCard',
};
</script>
