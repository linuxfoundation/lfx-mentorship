<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <mentee-detail-view :mentee-id="menteeId" />
</template>

<script setup lang="ts">
import { computed } from 'vue';
import MenteeDetailView from '~/components/modules/mentees/view/mentee-detail.vue';
import { menteePath } from '~/config/routes';
import type { MenteeDetail } from '~/types/mentee.types';

const route = useRoute();
const menteeId = computed(() => String(route.params.id ?? ''));
const config = useRuntimeConfig();

const { data: mentee, error } = await useAsyncData<MenteeDetail>(
  `mentee-seo-${menteeId.value}`,
  () => $fetch<MenteeDetail>(`/api/mentees/${menteeId.value}`),
  { lazy: false },
);

if (error.value) {
  throw createError(error.value);
}

const title = computed(() => mentee.value?.name ?? 'Mentee');
const description = computed(() => {
  const raw = mentee.value?.bio ?? '';
  return raw.length > 160
    ? `${raw.slice(0, 157)}...`
    : raw || 'Explore this mentee profile on LFX Mentorship.';
});
const baseUrl = (config.public.appUrl as string).replace(/\/$/, '');
const ogUrl = computed(() => `${baseUrl}${menteePath(menteeId.value)}`);

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
