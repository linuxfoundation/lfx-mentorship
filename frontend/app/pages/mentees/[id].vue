<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <mentee-detail-view :mentee-id="menteeId" />
</template>

<script setup lang="ts">
import { computed } from 'vue';
import MenteeDetailView from '~/components/modules/mentees/view/mentee-detail.vue';
import type { MenteeDetail } from '~/types/mentee.types';

const route = useRoute();
const menteeId = computed(() => String(route.params.id ?? ''));

const { data: mentee, error } = await useAsyncData<MenteeDetail>(
  `mentee-seo-${menteeId.value}`,
  () => $fetch<MenteeDetail>(`/api/mentees/${menteeId.value}`),
  { lazy: false },
);

if (error.value) {
  throw createError(error.value);
}

usePublicSeo({
  title: computed(() => mentee.value?.name ?? 'Mentee'),
  description: computed(
    () => mentee.value?.introduction ?? 'Explore this mentee profile on LFX Mentorship.',
  ),
  type: 'profile',
});
</script>
