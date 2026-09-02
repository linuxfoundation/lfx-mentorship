<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="container px-5 pb-16 md:px-10 md:pb-20">
    <div
      class="flex flex-col gap-6 rounded-2xl border border-neutral-200 bg-white p-6 md:flex-row md:items-center md:justify-between md:p-8"
    >
      <p class="max-w-xl text-sm text-neutral-600 leading-5 md:text-base">
        {{ ENROLL_PROGRAM_BOTTOM_CTA_COPY }}
      </p>
      <NuxtLink
        :to="ctaTo"
        class="shrink-0"
      >
        <lfx-button
          :label="ENROLL_PROGRAM_CTA_LABEL"
          type="primary"
          button-style="rounded"
          icon="arrow-right"
          icon-position="right"
        />
      </NuxtLink>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ENROLL_PROGRAM_BOTTOM_CTA_COPY, ENROLL_PROGRAM_CTA_LABEL } from '../config/enroll-program.config';
import LfxButton from '~/components/uikit/button/button.vue';

defineProps<{
  ctaTo: string;
}>();
</script>

<script lang="ts">
export default {
  name: 'EnrollProgramCta',
};
</script>
