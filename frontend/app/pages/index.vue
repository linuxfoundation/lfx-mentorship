<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <landing-view />
</template>

<script setup lang="ts">
import LandingView from '~/components/modules/landing/view/landing.vue';
import { SITE_NAME } from '~/utils/seo';

usePublicSeo({
  title: SITE_NAME,
  description:
    'Learn open source by building it. LFX Mentorship pairs contributors with mentors for 12-week terms with a stipend.',
  appendSiteName: false,
});
</script>
