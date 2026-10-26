<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <docs-landing />
</template>

<script setup lang="ts">
import DocsLanding from '~/components/modules/documentation/view/docs-landing.vue';

const url = useRequestURL();

useHead({ title: 'Documentation' });
useSeoMeta({
  description:
    'Learn how to use LFX Mentorship: find a program, apply as a mentee, mentor, run a program, and get paid stipends.',
  ogTitle: 'Documentation — LFX Mentorship',
  ogDescription:
    'Learn how to use LFX Mentorship: find a program, apply as a mentee, mentor, run a program, and get paid stipends.',
  ogType: 'website',
  ogUrl: url.origin + url.pathname,
  twitterCard: 'summary',
  twitterTitle: 'Documentation — LFX Mentorship',
  twitterDescription:
    'Learn how to use LFX Mentorship: find a program, apply as a mentee, mentor, run a program, and get paid stipends.',
});
useHead({ link: [{ rel: 'canonical', href: url.origin + url.pathname }] });
</script>
