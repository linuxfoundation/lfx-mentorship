<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <mentors-view />
</template>

<script setup lang="ts">
import MentorsView from '~/components/modules/mentors/view/mentors.vue';
import { FunnelEvent } from '~/composables/useFunnelAnalytics';

usePublicSeo({
  title: 'Mentors',
  description: 'Browse LFX Mentorship mentors. Find mentors by name and skill.',
});

useFunnelPageView(FunnelEvent.DirectoryViewed, { directory: 'mentors' }, 'directory_viewed:mentors');
</script>
