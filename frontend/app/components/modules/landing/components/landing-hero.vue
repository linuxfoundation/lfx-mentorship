<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="container px-5 pt-12 pb-10 md:px-10 md:pt-16 md:pb-12">
    <div class="flex flex-col gap-8 max-w-3xl">
      <div class="flex items-center gap-3">
        <lfx-avatar-group
          v-if="summary.graduatedMentees.length"
          type="member"
        >
          <lfx-avatar
            v-for="(mentee, i) in summary.graduatedMentees"
            :key="mentee.avatarUrl ?? mentee.name ?? i"
            type="member"
            :src="mentee.avatarUrl"
            size="small"
          />
        </lfx-avatar-group>
        <span class="text-sm text-neutral-600">{{ summary.graduatedMenteeCount + ' ' + LANDING_GRADUATED_COUNT_LABEL }}</span>
      </div>

      <div class="flex flex-col gap-4">
        <h1 class="font-secondary font-light text-5xl md:text-6xl leading-tight text-neutral-900">
          {{ LANDING_HERO_TITLE }}
        </h1>
        <p class="text-base text-neutral-600 leading-6 max-w-2xl">
          {{ LANDING_HERO_SUBTITLE }}
        </p>
      </div>

      <ul class="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:gap-x-8 sm:gap-y-3">
        <li
          v-for="feature in LANDING_HERO_FEATURES(summary)"
          :key="feature.label"
          class="flex items-center gap-2.5 text-sm text-neutral-800"
        >
          <lfx-icon
            :name="feature.icon"
            :type="feature.iconType ?? 'solid'"
            :size="16"
            class="text-positive-600 shrink-0"
          />
          <span>{{ feature.label }}</span>
        </li>
      </ul>

      <div class="flex flex-col gap-4 sm:flex-row sm:flex-wrap sm:items-center">
        <lfx-button
          label="Find a Program"
          type="primary"
          size="large"
          button-style="rounded"
          icon="magnifying-glass"
          class="justify-center"
          @click="navigateTo(AppRoute.FindProgram)"
        />
        <lfx-button
          label="Enroll a Program"
          type="outline"
          size="large"
          button-style="rounded"
          class="justify-center"
          @click="navigateTo(AppRoute.EnrollProgram)"
        />
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import {
  LANDING_GRADUATED_COUNT_LABEL,
  LANDING_HERO_FEATURES,
  LANDING_HERO_SUBTITLE,
  LANDING_HERO_TITLE,
} from '../config/landing.config';
import { AppRoute } from '~/config/routes';
import LfxAvatar from '~/components/uikit/avatar/avatar.vue';
import LfxAvatarGroup from '~/components/uikit/avatar-group/avatar-group.vue';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxIcon from '~/components/uikit/icon/icon.vue';
import type { LandingSummaryResponse } from '~/types/landing.types';

const props = defineProps<{
  summary: LandingSummaryResponse;
}>();
</script>

<script lang="ts">
export default {
  name: 'LandingHero',
};
</script>
