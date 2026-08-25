<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="container px-5 pt-12 pb-10 md:px-10 md:pt-16 md:pb-12">
    <div class="flex flex-col gap-6 max-w-3xl">
      <lfx-tag
        variation="info"
        size="small"
        type="solid"
        class="w-fit !h-auto px-2 py-1"
      >
        {{ ENROLL_PROGRAM_BADGE }}
      </lfx-tag>

      <div class="flex flex-col gap-4">
        <h1 class="font-secondary font-light text-4xl md:text-5xl leading-tight text-neutral-900">
          {{ ENROLL_PROGRAM_HERO_TITLE }}
        </h1>
        <p class="text-base text-neutral-600 leading-6 max-w-2xl">
          <template
            v-for="(part, i) in ENROLL_PROGRAM_HERO_SUBTITLE_PARTS"
            :key="i"
          >
            <strong
              v-if="part.bold"
              class="font-semibold text-neutral-800"
            >
              {{ part.text }}
            </strong>
            <template v-else>{{ part.text }}</template>
          </template>
        </p>
      </div>

      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:gap-5">
        <NuxtLink :to="ctaTo">
          <lfx-button
            :label="ENROLL_PROGRAM_CTA_LABEL"
            type="primary"
            button-style="pill"
            icon="arrow-right"
            icon-position="right"
            class="justify-center"
          />
        </NuxtLink>
        <p class="text-sm text-neutral-500 leading-5 max-w-xs">
          {{ ENROLL_PROGRAM_CTA_HELPER }}
        </p>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import {
  ENROLL_PROGRAM_BADGE,
  ENROLL_PROGRAM_CTA_HELPER,
  ENROLL_PROGRAM_CTA_LABEL,
  ENROLL_PROGRAM_HERO_SUBTITLE_PARTS,
  ENROLL_PROGRAM_HERO_TITLE,
} from '../config/enroll-program.config';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxTag from '~/components/uikit/tag/tag.vue';

defineProps<{
  ctaTo: string;
}>();
</script>

<script lang="ts">
export default {
  name: 'EnrollProgramHero',
};
</script>
