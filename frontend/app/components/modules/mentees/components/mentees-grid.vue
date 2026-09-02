<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="container pb-16">
    <div
      v-if="isLoading"
      class="grid grid-cols-1 gap-8 sm:grid-cols-2 lg:grid-cols-3"
    >
      <mentee-card-loading
        v-for="n in 6"
        :key="n"
      />
    </div>

    <div
      v-else-if="!mentees.length"
      class="flex flex-col items-center justify-center gap-4 py-24 text-neutral-500"
    >
      <lfx-icon
        :name="loadFailed ? 'circle-exclamation' : 'user-graduate'"
        type="light"
        :size="40"
      />
      <p class="text-base">
        {{ loadFailed ? 'Unable to load mentees. Please try again.' : 'No mentees found.' }}
      </p>
    </div>

    <template v-else>
      <div class="grid grid-cols-1 gap-8 sm:grid-cols-2 lg:grid-cols-3">
        <mentee-card
          v-for="mentee in mentees"
          :key="mentee.id"
          :mentee="mentee"
        />
      </div>

      <div
        v-if="hasMore"
        class="flex justify-center mt-10"
      >
        <lfx-button
          label="Load More"
          type="tertiary"
          button-style="rounded"
          :loading="isLoadingMore"
          @click="$emit('load-more')"
        />
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import MenteeCard from './mentee-card.vue';
import MenteeCardLoading from './mentee-card-loading.vue';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxIcon from '~/components/uikit/icon/icon.vue';
import type { Mentee } from '~/types/mentee.types';

defineProps<{
  mentees: Mentee[];
  hasMore: boolean;
  isLoading: boolean;
  isLoadingMore: boolean;
  loadFailed: boolean;
}>();

defineEmits<{ (e: 'load-more'): void }>();
</script>

<script lang="ts">
export default {
  name: 'MenteesGrid',
};
</script>
