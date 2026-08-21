<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <programs-view />
</template>

<script setup lang="ts">
import ProgramsView from '~/components/modules/programs/view/programs.vue';

const title = 'Programs';
const description = 'Browse LFX Mentorship programs. Find opportunities by status, skills, and mentors.';

useHead({ title });
useSeoMeta({
  description,
  ogTitle: `${title} | LFX Mentorship`,
  ogDescription: description,
  ogType: 'website',
  twitterCard: 'summary_large_image',
  twitterTitle: `${title} | LFX Mentorship`,
  twitterDescription: description,
});
</script>
