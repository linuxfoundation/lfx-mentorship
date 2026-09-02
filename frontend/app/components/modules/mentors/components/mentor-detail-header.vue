<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="border border-neutral-200 rounded-lg bg-white p-6 md:p-8">
    <detail-back-link
      :to="AppRoute.Mentors"
      label="Mentors"
      class="mb-6"
    />
    <div class="flex flex-col gap-6 lg:flex-row lg:justify-between lg:gap-0">
      <div class="flex min-w-0 flex-1 gap-4 md:gap-5">
        <div class="flex min-w-0 flex-1 flex-col gap-3">
          <div class="flex flex-wrap items-center gap-4 mb-5">
            <profile-initials-avatar
              :name="mentor.name"
              :src="mentor.avatarUrl"
              size="xlarge"
            />
            <div class="flex flex-col gap-1 flex-1">
              <h1 class="font-secondary text-xl md:text-3xl font-normal text-neutral-900 leading-tight">
                {{ mentor.name }}
              </h1>
              <p class="text-xs text-neutral-500 leading-4">
                {{ [mentor.sinceLabel, mentor.affiliationsLabel].filter(Boolean).join(' · ') }}
              </p>
            </div>
          </div>

          <p class="text-sm text-neutral-600 leading-5 max-w-[80%]">
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
              size="small"
              @click="openExternal(mentor.githubUrl)"
            />
            <lfx-button
              v-if="mentor.linkedinUrl"
              label="LinkedIn Profile"
              icon="linkedin"
              icon-type="brands"
              type="outline"
              button-style="rounded"
              size="small"
              @click="openExternal(mentor.linkedinUrl)"
            />
          </div>
        </div>
      </div>

      <aside class="w-full shrink-0 lg:w-56 space-y-6 lg:pl-6 lg:border-l lg:border-neutral-100">
        <div
          v-if="mentor.skills.length"
          class="flex flex-col gap-3"
        >
          <p class="text-xs font-semibold uppercase tracking-wide text-neutral-500">Featured skills</p>
          <div class="flex flex-wrap gap-2 flex-grow">
            <lfx-tag
              v-for="skill in mentor.skills"
              :key="skill"
              variation="neutral"
              type="outline"
              size="small"
              :title="skill"
              class="truncate !block leading-5"
            >
              {{ skill }}
            </lfx-tag>
          </div>
        </div>

        <dl class="flex flex-col gap-3 border-t border-neutral-100 pt-5">
          <div class="flex items-center justify-between gap-4 text-xs">
            <dt class="text-neutral-500">Programs mentoring</dt>
            <dd class=" text-neutral-900 tabular-nums">
              {{ mentor.stats.programsMentoring }}
            </dd>
          </div>
          <div class="flex items-center justify-between gap-4 text-xs">
            <dt class="text-neutral-500">Current mentees</dt>
            <dd class=" text-neutral-900 tabular-nums">
              {{ mentor.stats.currentMentees }}
            </dd>
          </div>
          <div class="flex items-center justify-between gap-4 text-xs">
            <dt class="text-neutral-500">Mentees graduated</dt>
            <dd class=" text-neutral-900 tabular-nums">
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
import LfxTag from '~/components/uikit/tag/tag.vue';

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
