<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <mentor-detail-view :mentor-id="mentorId" />
</template>

<script setup lang="ts">
import { computed } from 'vue';
import MentorDetailView from '~/components/modules/mentors/view/mentor-detail.vue';
import type { MentorDetail } from '~/types/mentor.types';

const route = useRoute();
const mentorId = computed(() => String(route.params.id ?? ''));

const { data: mentor, error } = await useAsyncData<MentorDetail>(
  `mentor-seo-${mentorId.value}`,
  () => $fetch<MentorDetail>(`/api/mentors/${mentorId.value}`),
  { lazy: false },
);

if (error.value) {
  throw createError(error.value);
}

usePublicSeo({
  title: computed(() => mentor.value?.name ?? 'Mentor'),
  description: computed(() => mentor.value?.bio ?? 'Explore this mentor profile on LFX Mentorship.'),
  type: 'profile',
});
</script>
