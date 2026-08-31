<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="border border-neutral-200 rounded-lg bg-white p-6 md:p-8">
    <detail-back-link
      :to="AppRoute.Mentees"
      label="Mentees"
      class="mb-6"
    />
    <div class="flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between lg:gap-0">
      <div
        class="flex min-w-0 flex-1 gap-4 md:gap-5"
        :class="{ 'lg:border-r lg:border-neutral-100 lg:pr-6': mentee.skills.length }"
      >
        <div class="flex min-w-0 flex-1 flex-col gap-3">
          <div class="flex flex-wrap items-center gap-4 mb-5">
            <profile-initials-avatar
              :name="mentee.name"
              :src="mentee.avatarUrl"
              size="xlarge"
            />
            <div class="flex flex-col gap-1 flex-1">
              <div class="flex flex-wrap items-center gap-2.5">
                <h1 class="font-secondary text-xl md:text-3xl font-normal text-neutral-900 leading-tight">
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
              <p class="text-xs text-neutral-500 leading-4">
                {{ mentee.sinceLabel }} · {{ mentee.project.foundationLabel }} ·
                {{ mentee.project.name }}
              </p>
            </div>
          </div>

          <p class="text-sm text-neutral-600 leading-5 max-w-xl">
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
              size="small"
              @click="openExternal(mentee.githubUrl)"
            />
            <lfx-button
              v-if="mentee.linkedinUrl"
              label="LinkedIn Profile"
              icon="linkedin"
              icon-type="brands"
              type="outline"
              button-style="rounded"
              size="small"
              @click="openExternal(mentee.linkedinUrl)"
            />
          </div>
        </div>
      </div>

      <aside
        v-if="mentee.skills.length"
        class="w-full shrink-0 lg:w-56 space-y-6 lg:pl-6"
      >
        <div class="flex flex-col gap-3">
          <p class="text-xs font-semibold uppercase tracking-wide text-neutral-500">Featured skills</p>
          <div class="flex flex-wrap gap-2">
            <lfx-tag
              v-for="skill in mentee.skills"
              :key="skill"
              variation="neutral"
              type="outline"
              size="small"
            >
              {{ skill }}
            </lfx-tag>
          </div>
        </div>
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
