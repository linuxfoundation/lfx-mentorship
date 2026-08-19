<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="border border-neutral-200 rounded-2xl bg-white p-6 md:p-8">
    <div class="flex flex-col gap-8 lg:flex-row lg:items-start lg:justify-between">
      <div class="flex min-w-0 flex-1 flex-col gap-6 md:flex-row md:items-start">
        <lfx-avatar
          :src="program.logoUrl"
          type="organization"
          size="xlarge"
          class="!rounded-xl shrink-0"
        />

        <div class="flex min-w-0 flex-1 flex-col gap-4">
          <div class="flex flex-wrap items-center gap-3">
            <span class="text-sm text-neutral-500">{{ program.foundationName }}</span>
            <lfx-tag
              :variation="statusConfig.variation"
              size="small"
              type="solid"
            >
              {{ statusConfig.label }}
            </lfx-tag>
          </div>

          <h1
            class="font-secondary text-2xl md:text-3xl font-light text-neutral-900 leading-tight break-words"
          >
            {{ program.name }}
          </h1>

          <div
            v-if="program.skills.length"
            class="flex flex-wrap gap-2"
          >
            <lfx-chip
              v-for="skill in program.skills"
              :key="skill"
              type="default"
              size="xsmall"
            >
              {{ skill }}
            </lfx-chip>
          </div>

          <div class="flex flex-wrap items-center gap-3 pt-1">
            <lfx-button
              v-if="program.repositoryUrl"
              label="Repository"
              icon="code-branch"
              type="outline"
              button-style="pill"
              @click="$emit('open-repository')"
            />
            <lfx-button
              label="Edit program"
              icon="pen"
              type="transparent"
              button-style="pill"
              @click="$emit('edit')"
            />
          </div>
        </div>
      </div>

      <aside
        class="w-full shrink-0 rounded-xl bg-neutral-50 border border-neutral-100 p-5 lg:w-72"
      >
        <p class="text-xs font-semibold uppercase tracking-wide text-neutral-500 mb-4">
          Term details
        </p>
        <dl class="flex flex-col gap-4">
          <div class="flex flex-col gap-1">
            <dt class="text-xs text-neutral-500">Term</dt>
            <dd class="text-sm font-medium text-neutral-900">
              {{ program.termDetailsLabel }}
            </dd>
          </div>
          <div
            v-if="program.applicationsCloseLabel"
            class="flex flex-col gap-1"
          >
            <dt class="text-xs text-neutral-500">Applications close</dt>
            <dd class="text-sm font-medium text-neutral-900">
              {{ program.applicationsCloseLabel }}
            </dd>
          </div>
          <div class="flex flex-col gap-1">
            <dt class="text-xs text-neutral-500">Current mentees</dt>
            <dd class="text-sm font-medium text-neutral-900">
              {{ program.stats.currentMentees }}
            </dd>
          </div>
          <div class="flex flex-col gap-1">
            <dt class="text-xs text-neutral-500">Mentors</dt>
            <dd class="text-sm font-medium text-neutral-900">
              {{ program.stats.mentors }}
            </dd>
          </div>
        </dl>
      </aside>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { ADMIN_PROGRAM_STATUS_CONFIG } from '../config/admin.config';
import type { AdminProgramDetail } from '~/types/admin.types';
import LfxAvatar from '~/components/uikit/avatar/avatar.vue';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxChip from '~/components/uikit/chip/chip.vue';
import LfxTag from '~/components/uikit/tag/tag.vue';

const props = defineProps<{ program: AdminProgramDetail }>();

defineEmits<{
  (e: 'open-repository'): void;
  (e: 'edit'): void;
}>();

const statusConfig = computed(() => ADMIN_PROGRAM_STATUS_CONFIG[props.program.status]);
</script>

<script lang="ts">
export default {
  name: 'AdminProgramDetailHeader',
};
</script>
