<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="rounded-2xl border border-neutral-200 bg-white px-4 py-5 md:px-6">
    <ol class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
      <li
        v-for="(step, index) in ADMIN_ENROLL_STEPS_ORDER"
        :key="step"
        class="flex items-center gap-3 min-w-0 flex-1"
      >
        <div class="flex items-center gap-3 min-w-0">
          <span
            class="flex size-7 shrink-0 items-center justify-center rounded-full text-xs font-semibold"
            :class="circleClass(step)"
          >
            <lfx-icon
              v-if="isComplete(step)"
              name="check"
              type="solid"
              :size="12"
              class="text-white"
            />
            <span
              v-else-if="step === current"
              class="size-3 rounded-full bg-accent-500"
            />
            <span
              v-else
              class="size-3 rounded-full bg-neutral-250"
            />
          </span>
          <span
            class="text-sm truncate"
            :class="labelClass(step)"
          >
            {{ ADMIN_ENROLL_STEP_LABELS[step] }}
          </span>
        </div>
        <div
          v-if="index < ADMIN_ENROLL_STEPS_ORDER.length - 1"
          class="hidden sm:block h-px flex-1 mx-2"
          :class="isComplete(step) ? 'bg-positive-500' : 'bg-neutral-200'"
          aria-hidden="true"
        />
      </li>
    </ol>
  </div>
</template>

<script setup lang="ts">
import {
  ADMIN_ENROLL_STEP_LABELS,
  ADMIN_ENROLL_STEPS_ORDER,
} from '../config/admin.config';
import type { AdminEnrollStep } from '~/types/admin.types';
import LfxIcon from '~/components/uikit/icon/icon.vue';

const props = defineProps<{ current: AdminEnrollStep }>();

function stepIndex(step: AdminEnrollStep): number {
  return ADMIN_ENROLL_STEPS_ORDER.indexOf(step);
}

function isComplete(step: AdminEnrollStep): boolean {
  return stepIndex(step) < stepIndex(props.current);
}

function circleClass(step: AdminEnrollStep): string {
  if (isComplete(step)) return 'bg-positive-500 text-white';
  if (step === props.current) return 'bg-brand-150 border-accent-500 border text-white';
  return 'bg-neutral-150 text-neutral-500';
}

function labelClass(step: AdminEnrollStep): string {
  if (isComplete(step)) return 'text-positive-600 font-medium';
  if (step === props.current) return 'text-brand-700 font-semibold';
  return 'text-neutral-500';
}
</script>

<script lang="ts">
export default {
  name: 'AdminEnrollStepper',
};
</script>
