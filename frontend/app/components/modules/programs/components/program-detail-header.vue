<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="border border-neutral-200 rounded-lg bg-white p-6 md:p-8">
    <detail-back-link
      :to="AppRoute.FindProgram"
      label="Programs"
      class="mb-6"
    />
    <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
      <div class="flex min-w-0 flex-1 flex-col gap-6 md:flex-row md:items-start">
        <lfx-avatar
          :src="program.logoUrl"
          type="organization"
          size="xlarge"
          class="!rounded-lg shrink-0"
        />

        <div class="flex min-w-0 flex-1 flex-col gap-4">
          <div class="flex flex-wrap items-center gap-3">
            <span
              v-if="program.foundation.name"
              class="text-xs text-neutral-500"
            >{{ program.foundation.name }}</span>
            <lfx-tag
              :variation="statusConfig.variation"
              size="small"
              type="solid"
            >
              {{ statusConfig.label }}
            </lfx-tag>
          </div>

          <h1 class="font-secondary text-xl md:text-2xl font-normal text-neutral-900 leading-tight break-words">
            {{ program.name }}
          </h1>

          <div
            v-if="program.skills.length"
            class="flex flex-wrap gap-2"
          >
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

          <div class="flex flex-wrap items-center gap-3 pt-1">
            <lfx-button
              v-if="isAccepting && displayTerms.length === 1"
              label="Apply to This Program"
              icon="paper-plane"
              type="primary"
              button-style="rounded"
              @click="$emit('apply')"
            />
            <lfx-button
              v-if="program.repositoryUrl"
              label="Repository"
              icon="code-branch"
              type="outline"
              button-style="rounded"
              @click="$emit('open-repository')"
            />
            <lfx-button
              v-if="program.id"
              label="Donate"
              icon="heart"
              icon-type="solid"
              type="transparent"
              button-style="rounded"
              @click="$emit('donate')"
            />
          </div>
        </div>
      </div>

      <aside
        v-for="term in displayTerms"
        :key="term.id"
        class="w-full shrink-0 rounded-xl bg-neutral-50 border border-neutral-100 px-4.5 py-4 lg:w-52"
      >
        <p class="text-xs font-semibold uppercase tracking-wide text-neutral-500 mb-3">
          {{ isAccepting ? 'Accepting Applications' : 'Previous Term' }}
        </p>
        <dl class="flex flex-col gap-3">
          <div class="flex flex-col gap-1">
            <dt class="text-xxs text-neutral-500">Term</dt>
            <dd class="text-xs text-neutral-900">
              {{ formatTermLabel(term) }}
            </dd>
          </div>
          <div
            v-if="term.applicationsCloseAt"
            class="flex flex-col gap-1"
          >
            <dt class="text-xxs text-neutral-500">
              {{ isAccepting ? 'Applications close' : 'Applications closed' }}
            </dt>
            <dd class="text-xs text-neutral-900">
              {{ formatProgramDate(term.applicationsCloseAt) }}
            </dd>
          </div>
          <div
            v-if="isAccepting"
            class="flex flex-col gap-1"
          >
            <dt class="text-xxs text-neutral-500">Stipend</dt>
            <dd class="text-xs text-neutral-900">Amount determined by mentee location</dd>
          </div>
        </dl>
        <lfx-button
          v-if="displayTerms.length > 1"
          :label="`Apply for ${term.name}`"
          icon="paper-plane"
          type="primary"
          button-style="rounded"
          class="mt-3"
          @click="$emit('apply')"
        />
      </aside>
    </div>
  </section>
</template>

<script setup lang="ts">
import { PROGRAM_STATUS_CONFIG } from '../config/program-card.config';
import { AppRoute } from '~/config/routes';
import type { Program } from '~/types/program.types';
import { formatTermLabel } from '~/utils/program-terms';

const props = defineProps<{ program: Program }>();

defineEmits<{
  (e: 'open-repository' | 'donate' | 'apply'): void;
}>();

const statusConfig = computed(() => PROGRAM_STATUS_CONFIG[props.program.status]);
const activeTerms = computed(() => props.program.activeTerms);
const isAccepting = computed(() => activeTerms.value.length > 0);
const displayTerms = computed(() => {
  if (activeTerms.value.length) return activeTerms.value.slice(0, 2);
  const last = props.program.terms.at(-1);
  return last ? [last] : [];
});
</script>

<script lang="ts">
export default {
  name: 'ProgramDetailHeader',
};
</script>
