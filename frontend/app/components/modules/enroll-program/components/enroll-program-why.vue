<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="container px-5 pb-10 md:px-10 md:pb-12">
    <div class="mb-8 max-w-2xl">
      <h2 class="font-secondary text-2xl md:text-3xl font-light text-neutral-900">
        {{ ENROLL_PROGRAM_WHY_TITLE }}
      </h2>
      <p class="mt-2 text-sm text-neutral-500">
        {{ ENROLL_PROGRAM_WHY_SUBTITLE }}
      </p>
    </div>

    <ul class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
      <li
        v-for="benefit in ENROLL_PROGRAM_BENEFITS"
        :key="benefit.title"
        class="rounded-2xl border border-neutral-200 bg-white p-6"
      >
        <lfx-icon
          :name="benefit.icon"
          type="solid"
          :size="22"
          class="text-brand-500"
        />
        <h3 class="mt-4 text-base font-semibold text-neutral-900">
          {{ benefit.title }}
        </h3>
        <p class="mt-2 text-sm text-neutral-600 leading-5">
          {{ benefit.description }}
        </p>
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import {
  ENROLL_PROGRAM_BENEFITS,
  ENROLL_PROGRAM_WHY_SUBTITLE,
  ENROLL_PROGRAM_WHY_TITLE,
} from '../config/enroll-program.config';
import LfxIcon from '~/components/uikit/icon/icon.vue';
</script>

<script lang="ts">
export default {
  name: 'EnrollProgramWhy',
};
</script>
