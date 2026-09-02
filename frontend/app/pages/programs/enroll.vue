<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <enroll-program-view />
</template>

<script setup lang="ts">
import EnrollProgramView from '~/components/modules/enroll-program/view/enroll-program.vue';

const title = 'Enroll a Program';
const description =
  'Program Admins can enroll a mentorship program for a Linux Foundation project on LFX. Learn what enrollment asks for and how to prepare.';

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
