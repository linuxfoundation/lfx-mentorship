<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="border border-neutral-200 rounded-2xl bg-white p-6 md:p-8">
    <detail-back-link
      :to="AppRoute.Mentees"
      label="Mentees"
      class="mb-6"
    />
    <div class="flex flex-col gap-8 lg:flex-row lg:items-start lg:justify-between lg:gap-12">
      <div class="flex min-w-0 flex-1 gap-4 md:gap-5">
        <profile-initials-avatar
          :name="mentee.name"
          size="xlarge"
        />

        <div class="flex min-w-0 flex-1 flex-col gap-3">
          <div class="flex flex-wrap items-center gap-2.5">
            <h1 class="font-secondary text-3xl md:text-4xl font-light text-neutral-900 leading-tight">
              {{ mentee.name }}
            </h1>
            <lfx-tag
              :variation="statusConfig.variation"
              size="small"
              type="solid"
            >
              {{ statusConfig.label }}
            </lfx-tag>
          </div>

          <p class="text-sm text-neutral-500 leading-5">
            {{ mentee.sinceLabel }} · {{ mentee.project.foundationLabel }} ·
            {{ mentee.project.name }}
          </p>

          <p class="text-sm text-neutral-600 leading-6 max-w-xl">
            {{ mentee.bio }}
          </p>

          <div class="flex flex-wrap items-center gap-3 pt-1">
            <lfx-button
              v-if="mentee.githubUrl"
              label="Github Profile"
              icon="github"
              icon-type="brands"
              type="outline"
              button-style="rounded"
              @click="openExternal(mentee.githubUrl)"
            />
            <lfx-button
              v-if="mentee.linkedinUrl"
              label="LinkedIn Profile"
              icon="linkedin"
              icon-type="brands"
              type="outline"
              button-style="rounded"
              @click="openExternal(mentee.linkedinUrl)"
            />
          </div>
        </div>
      </div>

      <aside class="w-full shrink-0 lg:w-72 lg:border-l lg:border-neutral-100 lg:pl-12 space-y-6">
        <div
          v-if="mentee.skills.length"
          class="flex flex-col gap-3"
        >
          <p class="text-xs font-semibold uppercase tracking-wide text-neutral-500">Featured skills</p>
          <div class="flex flex-wrap gap-2">
            <lfx-chip
              v-for="skill in mentee.skills"
              :key="skill"
              type="default"
              size="xsmall"
            >
              {{ skill }}
            </lfx-chip>
          </div>
        </div>

        <dl class="flex flex-col gap-3 border-t border-neutral-100 pt-5">
          <div class="flex items-center justify-between gap-4 text-sm">
            <dt class="text-neutral-500">Programs</dt>
            <dd class="font-semibold text-neutral-900 tabular-nums">
              {{ mentee.stats.programs }}
            </dd>
          </div>
          <div class="flex items-center justify-between gap-4 text-sm">
            <dt class="text-neutral-500">Terms completed</dt>
            <dd class="font-semibold text-neutral-900 tabular-nums">
              {{ mentee.stats.termsCompleted }}
            </dd>
          </div>
          <div class="flex items-center justify-between gap-4 text-sm">
            <dt class="text-neutral-500">Mentors</dt>
            <dd class="font-semibold text-neutral-900 tabular-nums">
              {{ mentee.stats.mentors }}
            </dd>
          </div>
        </dl>
      </aside>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { MENTEE_STATUS_CONFIG } from '../config/mentee-card.config';
import { AppRoute } from '~/config/routes';
import ProfileInitialsAvatar from '~/components/shared/directory/profile-initials-avatar.vue';
import DetailBackLink from '~/components/shared/detail-back-link.vue';
import type { MenteeDetail } from '~/types/mentee.types';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxChip from '~/components/uikit/chip/chip.vue';
import LfxTag from '~/components/uikit/tag/tag.vue';

const props = defineProps<{ mentee: MenteeDetail }>();

const statusConfig = computed(() => MENTEE_STATUS_CONFIG[props.mentee.status]);

function openExternal(url: string) {
  if (!import.meta.client) return;
  window.open(url, '_blank', 'noopener,noreferrer');
}
</script>

<script lang="ts">
export default {
  name: 'MenteeDetailHeader',
};
</script>
