<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <program-detail-view :program-id="programId" />
</template>

<script setup lang="ts">
import { computed } from 'vue';
import ProgramDetailView from '~/components/modules/programs/view/program-detail.vue';
import { programPath } from '~/config/routes';
import type { Program } from '~/types/program.types';

const route = useRoute();
const programId = computed(() => String(route.params.id ?? ''));
const config = useRuntimeConfig();

const { data: program, error } = await useAsyncData<Program>(
  `program-seo-${programId.value}`,
  () => $fetch<Program>(`/api/programs/${programId.value}`),
  { lazy: false },
);

if (error.value) {
  throw createError(error.value);
}

const title = computed(() => program.value?.name ?? 'Program');
const description = computed(() => {
  const raw = program.value?.description ?? '';
  return raw.length > 160 ? `${raw.slice(0, 157)}...` : raw || 'Explore this mentorship program on LFX Mentorship.';
});
const baseUrl = (config.public.appUrl as string).replace(/\/$/, '');
const ogUrl = computed(() => `${baseUrl}${programPath(programId.value)}`);

useHead({ title });
useSeoMeta({
  description,
  ogTitle: computed(() => `${title.value} | LFX Mentorship`),
  ogDescription: description,
  ogType: 'website',
  ogUrl,
  twitterCard: 'summary_large_image',
  twitterTitle: computed(() => `${title.value} | LFX Mentorship`),
  twitterDescription: description,
});
</script>
