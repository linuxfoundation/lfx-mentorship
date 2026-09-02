<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <landing-view />
</template>

<script setup lang="ts">
import LandingView from '~/components/modules/landing/view/landing.vue';

const title = 'LFX Mentorship';
const description =
  'Learn open source by building it. LFX Mentorship pairs contributors with maintainers for 12-week terms with a stipend.';

useHead({ title });
useSeoMeta({
  description,
  ogTitle: title,
  ogDescription: description,
  ogType: 'website',
  twitterCard: 'summary_large_image',
  twitterTitle: title,
  twitterDescription: description,
});
</script>
