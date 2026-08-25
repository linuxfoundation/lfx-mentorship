<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="container px-5 pb-16 md:px-10 md:pb-20">
    <div
      class="flex flex-col gap-6 rounded-2xl border border-neutral-200 bg-white p-6 md:flex-row md:items-center md:justify-between md:p-8"
    >
      <div class="max-w-xl">
        <h2 class="font-secondary text-xl md:text-2xl font-normal text-neutral-900">
          {{ LANDING_CTA_TITLE }}
        </h2>
        <p class="mt-2 text-sm text-neutral-500 leading-5">
          {{ LANDING_CTA_SUBTITLE }}
        </p>
      </div>
      <NuxtLink
        :to="AppRoute.FindProgram"
        class="shrink-0"
      >
        <lfx-button
          label="Find a Program"
          type="primary"
          button-style="rounded"
          size="large"
          icon="arrow-right"
          icon-position="right"
        />
      </NuxtLink>
    </div>
  </section>
</template>

<script setup lang="ts">
import { LANDING_CTA_SUBTITLE, LANDING_CTA_TITLE } from '../config/landing.config';
import { AppRoute } from '~/config/routes';
import LfxButton from '~/components/uikit/button/button.vue';
</script>

<script lang="ts">
export default {
  name: 'LandingCta',
};
</script>
