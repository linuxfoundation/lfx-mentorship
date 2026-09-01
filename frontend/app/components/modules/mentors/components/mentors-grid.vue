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
      v-else-if="!mentors.length"
      class="flex flex-col items-center justify-center gap-4 py-24 text-neutral-500"
    >
      <lfx-icon
        :name="loadFailed ? 'circle-exclamation' : 'user-tie'"
        type="light"
        :size="40"
      />
      <p class="text-base">
        {{ loadFailed ? 'Unable to load mentors. Please try again.' : 'No mentors found.' }}
      </p>
    </div>

    <template v-else>
      <div class="grid grid-cols-1 gap-8 sm:grid-cols-2 lg:grid-cols-3">
        <mentor-card
          v-for="mentor in mentors"
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
          :loading="isLoadingMore"
          @click="$emit('load-more')"
        />
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import MentorCard from './mentor-card.vue';
import MentorCardLoading from './mentor-card-loading.vue';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxIcon from '~/components/uikit/icon/icon.vue';
import type { Mentor } from '~/types/mentor.types';

defineProps<{
  mentors: Mentor[];
  hasMore: boolean;
  isLoading: boolean;
  isLoadingMore: boolean;
  loadFailed: boolean;
}>();

defineEmits<{ (e: 'load-more'): void }>();
</script>

<script lang="ts">
export default {
  name: 'MentorsGrid',
};
</script>
