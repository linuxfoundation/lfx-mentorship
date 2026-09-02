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
import { mentorPath } from '~/config/routes';
import type { MentorDetail } from '~/types/mentor.types';

const route = useRoute();
const mentorId = computed(() => String(route.params.id ?? ''));
const config = useRuntimeConfig();

const { data: mentor, error } = await useAsyncData<MentorDetail>(
  `mentor-seo-${mentorId.value}`,
  () => $fetch<MentorDetail>(`/api/mentors/${mentorId.value}`),
  { lazy: false },
);

if (error.value) {
  throw createError(error.value);
}

const title = computed(() => mentor.value?.name ?? 'Mentor');
const description = computed(() => {
  const raw = mentor.value?.bio ?? '';
  return raw.length > 160 ? `${raw.slice(0, 157)}...` : raw || 'Explore this mentor profile on LFX Mentorship.';
});
const baseUrl = (config.public.appUrl as string).replace(/\/$/, '');
const ogUrl = computed(() => `${baseUrl}${mentorPath(mentorId.value)}`);

useHead({ title });
useSeoMeta({
  description,
  ogTitle: computed(() => `${title.value} | LFX Mentorship`),
  ogDescription: description,
  ogType: 'profile',
  ogUrl,
  twitterCard: 'summary_large_image',
  twitterTitle: computed(() => `${title.value} | LFX Mentorship`),
  twitterDescription: description,
});
</script>
