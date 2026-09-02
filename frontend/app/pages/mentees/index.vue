<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <mentees-view />
</template>

<script setup lang="ts">
import MenteesView from '~/components/modules/mentees/view/mentees.vue';

const title = 'Mentees';
const description = 'Browse LFX Mentorship mentees. Find contributors by skill, project, and status.';

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
