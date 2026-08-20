<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="container pb-16">
    <div
      v-if="isLoading"
      class="grid grid-cols-1 gap-8 sm:grid-cols-2 lg:grid-cols-3"
    >
      <mentor-card-loading
        v-for="n in 6"
        :key="n"
      />
    </div>

    <div
      v-else-if="error"
      class="flex items-center gap-2 text-negative-600"
    >
      <lfx-icon
        name="circle-exclamation"
        type="solid"
        :size="16"
      />
      <span class="text-body-1">Failed to load mentors. Please try again.</span>
    </div>

    <div
      v-else-if="!mentors.length"
      class="flex flex-col items-center justify-center gap-4 py-24 text-neutral-500"
    >
      <lfx-icon
        name="user-tie"
        type="light"
        :size="40"
      />
      <p class="text-base">No mentors found.</p>
    </div>

    <template v-else>
      <div class="grid grid-cols-1 gap-8 sm:grid-cols-2 lg:grid-cols-3">
        <mentor-card
          v-for="mentor in visibleMentors"
          :key="mentor.id"
          :mentor="mentor"
        />
      </div>

      <div
        v-if="hasMore"
        class="flex justify-center mt-10"
      >
        <lfx-button
          label="Load More"
          type="tertiary"
          button-style="pill"
          @click="$emit('load-more')"
        />
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import MentorCard from './mentor-card.vue';
import MentorCardLoading from './mentor-card-loading.vue';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxIcon from '~/components/uikit/icon/icon.vue';
import type { Mentor } from '~/types/mentor.types';

const props = defineProps<{
  mentors: Mentor[];
  visibleCount: number;
  isLoading: boolean;
  error: Error | null;
}>();

defineEmits<{ (e: 'load-more'): void }>();

const visibleMentors = computed(() => props.mentors.slice(0, props.visibleCount));
const hasMore = computed(() => props.visibleCount < props.mentors.length);
</script>

<script lang="ts">
export default {
  name: 'MentorsGrid',
};
</script>
