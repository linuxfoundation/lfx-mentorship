<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <docs-landing />
</template>

<script setup lang="ts">
import DocsLanding from '~/components/modules/documentation/view/docs-landing.vue';

const url = useRequestURL();

const description =
  'Learn how to use LFX Mentorship — find a program, apply as a mentee, mentor a project, and enroll a program.';

useHead({ title: 'Documentation' });
useSeoMeta({
  description,
  ogTitle: 'Documentation — LFX Mentorship',
  ogDescription: description,
  ogType: 'website',
  ogUrl: url.origin + url.pathname,
  twitterCard: 'summary',
  twitterTitle: 'Documentation — LFX Mentorship',
  twitterDescription: description,
});
useHead({ link: [{ rel: 'canonical', href: url.origin + url.pathname }] });
</script>
