<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <mentors-view />
</template>

<script setup lang="ts">
import MentorsView from '~/components/modules/mentors/view/mentors.vue';

const title = 'Mentors';
const description =
  'Browse LFX Mentorship mentors. Find mentors by skill and project experience.';

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
