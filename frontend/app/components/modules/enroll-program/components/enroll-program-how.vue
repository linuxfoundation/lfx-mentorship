<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="container px-5 pb-10 md:px-10 md:pb-12">
    <div class="mb-8 max-w-3xl">
      <h2 class="font-secondary text-2xl md:text-3xl font-light text-neutral-900">
        {{ ENROLL_PROGRAM_HOW_TITLE }}
      </h2>
      <p class="mt-2 text-sm text-neutral-500 leading-5">
        {{ ENROLL_PROGRAM_HOW_SUBTITLE }}
      </p>
    </div>

    <ol class="flex flex-col gap-4">
      <li
        v-for="step in ENROLL_PROGRAM_STEPS"
        :key="step.step"
        class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8"
      >
        <div class="flex items-start gap-4">
          <span
            class="flex size-9 shrink-0 items-center justify-center rounded-full bg-brand-50 text-sm font-semibold text-brand-600"
          >
            {{ step.step }}
          </span>
          <div class="min-w-0 flex-1">
            <h3 class="text-base font-semibold text-neutral-900">
              {{ step.title }}
            </h3>
            <p class="mt-1 text-sm text-neutral-600 leading-5">
              {{ step.description }}
            </p>

            <ul class="mt-5 grid grid-cols-1 gap-x-6 gap-y-2.5 sm:grid-cols-2 lg:grid-cols-4">
              <li
                v-for="item in step.checklist"
                :key="item"
                class="flex items-start gap-2 text-sm text-neutral-700"
              >
                <lfx-icon
                  name="circle-check"
                  type="light"
                  :size="16"
                  class="text-positive-600 shrink-0 mt-0.5"
                />
                <span>{{ item }}</span>
              </li>
            </ul>
          </div>
        </div>
      </li>
    </ol>
  </section>
</template>

<script setup lang="ts">
import {
  ENROLL_PROGRAM_HOW_SUBTITLE,
  ENROLL_PROGRAM_HOW_TITLE,
  ENROLL_PROGRAM_STEPS,
} from '../config/enroll-program.config';
import LfxIcon from '~/components/uikit/icon/icon.vue';
</script>

<script lang="ts">
export default {
  name: 'EnrollProgramHow',
};
</script>
