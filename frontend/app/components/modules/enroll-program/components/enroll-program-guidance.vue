<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="container px-5 pb-10 md:px-10 md:pb-12">
    <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <div class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8">
        <h2 class="text-base font-semibold text-neutral-900">
          {{ ENROLL_PROGRAM_AFTER_TITLE }}
        </h2>
        <ol class="mt-5 flex flex-col gap-4">
          <li
            v-for="(item, index) in ENROLL_PROGRAM_AFTER_ITEMS"
            :key="index"
            class="flex items-start gap-3"
          >
            <span
              class="flex size-7 shrink-0 items-center justify-center rounded-full bg-neutral-100 text-xs font-semibold text-neutral-500"
            >
              {{ index + 1 }}
            </span>
            <p class="text-sm text-neutral-600 leading-5 pt-0.5">
              {{ item.text }}
            </p>
          </li>
        </ol>
      </div>

      <div class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8">
        <h2 class="text-base font-semibold text-neutral-900">
          {{ ENROLL_PROGRAM_BEFORE_TITLE }}
        </h2>
        <ul class="mt-5 flex flex-col gap-4">
          <li
            v-for="(item, index) in ENROLL_PROGRAM_BEFORE_ITEMS"
            :key="index"
            class="flex items-start gap-3"
          >
            <lfx-icon
              name="circle-check"
              type="light"
              :size="18"
              class="text-positive-600 shrink-0 mt-0.5"
            />
            <p class="text-sm text-neutral-600 leading-5">
              {{ item.text }}
            </p>
          </li>
        </ul>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import {
  ENROLL_PROGRAM_AFTER_ITEMS,
  ENROLL_PROGRAM_AFTER_TITLE,
  ENROLL_PROGRAM_BEFORE_ITEMS,
  ENROLL_PROGRAM_BEFORE_TITLE,
} from '../config/enroll-program.config';
import LfxIcon from '~/components/uikit/icon/icon.vue';
</script>

<script lang="ts">
export default {
  name: 'EnrollProgramGuidance',
};
</script>
