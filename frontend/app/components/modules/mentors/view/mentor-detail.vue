<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="container max-w-full overflow-hidden px-5 py-6 md:px-10 md:py-8 space-y-6">
    <div
      v-if="isLoading"
      class="flex items-center gap-2 text-neutral-500 py-16 justify-center"
    >
      <lfx-spinner />
      <span>Loading mentor…</span>
    </div>

    <div
      v-else-if="error || !mentor"
      class="rounded-2xl border border-neutral-200 bg-white p-10 text-center text-negative-600"
    >
      Mentor not found.
    </div>

    <template v-else>
      <mentor-detail-header :mentor="mentor" />

      <section class="space-y-4">
        <h2 class="font-secondary text-xl md:text-2xl font-normal text-neutral-900">Programs</h2>
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <profile-program-card
            v-for="program in mentor.programs"
            :key="program.id"
            :program="program"
          />
        </div>
      </section>

      <section
        v-if="mentor.currentMentees.length"
        class="space-y-4"
      >
        <h2 class="font-secondary text-xl md:text-2xl font-normal text-neutral-900">Mentees</h2>
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-3">
          <profile-mentee-card
            v-for="mentee in mentor.currentMentees"
            :key="mentee.id"
            :mentee="mentee"
          />
        </div>
      </section>

      <section
        v-if="mentor.graduatedMentees.length"
        class="space-y-4"
      >
        <h2 class="font-secondary text-xl md:text-2xl font-normal text-neutral-900">Graduated Mentees</h2>
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-3">
          <profile-mentee-card
            v-for="mentee in mentor.graduatedMentees"
            :key="mentee.id"
            :mentee="mentee"
          />
        </div>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import MentorDetailHeader from '../components/mentor-detail-header.vue';
import ProfileMenteeCard from '~/components/shared/directory/profile-mentee-card.vue';
import ProfileProgramCard from '~/components/shared/directory/profile-program-card.vue';
import { useMentor } from '~/composables/mentors/useMentor';
import LfxSpinner from '~/components/uikit/spinner/spinner.vue';

const props = defineProps<{ mentorId: string }>();

const mentorId = computed(() => props.mentorId);
const { data: mentor, isLoading, error } = useMentor(mentorId);

useHead({
  title: computed(() => mentor.value?.name ?? 'Mentor'),
});
</script>

<script lang="ts">
export default {
  name: 'MentorDetailView',
};
</script>
