<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <admin-program-detail-view :program-id="programId" />
</template>

<script setup lang="ts">
import { computed } from 'vue';
import AdminProgramDetailView from '~/components/modules/admin/view/admin-program-detail.vue';
import { adminProgramPath } from '~/config/routes';
import type { AdminProgramDetail } from '~/types/admin.types';

const route = useRoute();
const programId = computed(() => String(route.params.id ?? ''));
const config = useRuntimeConfig();

const { data: program } = await useAsyncData<AdminProgramDetail>(
  `admin-program-seo-${programId.value}`,
  () => $fetch<AdminProgramDetail>(`/api/admin/programs/${programId.value}`),
  { lazy: false },
);

const title = computed(() => program.value?.name ?? 'Admin program');
const description = computed(() => {
  const raw = program.value?.description ?? '';
  return raw.length > 160
    ? `${raw.slice(0, 157)}...`
    : raw || 'Administer this mentorship program on LFX Mentorship.';
});
const baseUrl = (config.public.appUrl as string).replace(/\/$/, '');
const ogUrl = computed(() => `${baseUrl}${adminProgramPath(programId.value)}`);

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
