<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="border border-neutral-200 rounded-2xl bg-white p-6 md:p-8">
    <detail-back-link
      :to="AppRoute.Mentors"
      label="Mentors"
      class="mb-6"
    />
    <div class="flex flex-col gap-8 lg:flex-row lg:items-start lg:justify-between lg:gap-12">
      <div class="flex min-w-0 flex-1 gap-4 md:gap-5">
        <profile-initials-avatar
          :name="mentor.name"
          size="xlarge"
        />

        <div class="flex min-w-0 flex-1 flex-col gap-3">
          <h1 class="font-secondary text-3xl md:text-4xl font-light text-neutral-900 leading-tight">
            {{ mentor.name }}
          </h1>

          <p class="text-sm text-neutral-500 leading-5">{{ mentor.sinceLabel }} · {{ mentor.affiliationsLabel }}</p>

          <p class="text-sm text-neutral-600 leading-6 max-w-xl">
            {{ mentor.bio }}
          </p>

          <div class="flex flex-wrap items-center gap-3 pt-1">
            <lfx-button
              v-if="mentor.githubUrl"
              label="Github Profile"
              icon="github"
              icon-type="brands"
              type="outline"
              button-style="rounded"
              @click="openExternal(mentor.githubUrl)"
            />
            <lfx-button
              v-if="mentor.linkedinUrl"
              label="LinkedIn Profile"
              icon="linkedin"
              icon-type="brands"
              type="outline"
              button-style="rounded"
              @click="openExternal(mentor.linkedinUrl)"
            />
          </div>
        </div>
      </div>

      <aside class="w-full shrink-0 lg:w-72 lg:border-l lg:border-neutral-100 lg:pl-12 space-y-6">
        <div
          v-if="mentor.skills.length"
          class="flex flex-col gap-3"
        >
          <p class="text-xs font-semibold uppercase tracking-wide text-neutral-500">Featured skills</p>
          <div class="flex flex-wrap gap-2">
            <lfx-chip
              v-for="skill in mentor.skills"
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
            <dt class="text-neutral-500">Programs mentoring</dt>
            <dd class="font-semibold text-neutral-900 tabular-nums">
              {{ mentor.stats.programsMentoring }}
            </dd>
          </div>
          <div class="flex items-center justify-between gap-4 text-sm">
            <dt class="text-neutral-500">Current mentees</dt>
            <dd class="font-semibold text-neutral-900 tabular-nums">
              {{ mentor.stats.currentMentees }}
            </dd>
          </div>
          <div class="flex items-center justify-between gap-4 text-sm">
            <dt class="text-neutral-500">Mentees graduated</dt>
            <dd class="font-semibold text-neutral-900 tabular-nums">
              {{ mentor.stats.menteesGraduated }}
            </dd>
          </div>
        </dl>
      </aside>
    </div>
  </section>
</template>

<script setup lang="ts">
import { AppRoute } from '~/config/routes';
import ProfileInitialsAvatar from '~/components/shared/directory/profile-initials-avatar.vue';
import DetailBackLink from '~/components/shared/detail-back-link.vue';
import type { MentorDetail } from '~/types/mentor.types';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxChip from '~/components/uikit/chip/chip.vue';

defineProps<{ mentor: MentorDetail }>();

function openExternal(url: string) {
  if (!import.meta.client) return;
  window.open(url, '_blank', 'noopener,noreferrer');
}
</script>

<script lang="ts">
export default {
  name: 'MentorDetailHeader',
};
</script>
