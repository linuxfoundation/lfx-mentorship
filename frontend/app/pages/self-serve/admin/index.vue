<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <admin-view />
</template>

<script setup lang="ts">
import AdminView from '~/components/modules/admin/view/admin.vue';

const title = 'Admin';
const description =
  'Enroll and administer mentorship programs on LFX Mentorship.';

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
