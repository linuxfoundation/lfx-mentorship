<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <mentor-register-view />
</template>

<script setup lang="ts">
import MentorRegisterView from '~/components/modules/mentor-register/view/mentor-register.vue';

const title = 'Become a Mentor';
const description =
  'Register as a mentor and request to join the LFX Mentorship programs you want to support.';

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
