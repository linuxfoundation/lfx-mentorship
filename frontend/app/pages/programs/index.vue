<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <programs-view />
</template>

<script setup lang="ts">
import ProgramsView from '~/components/modules/programs/view/programs.vue';
import { FunnelEvent } from '~/composables/useFunnelAnalytics';

usePublicSeo({
  title: 'Programs',
  description: 'Browse LFX Mentorship programs. Find opportunities by status, skills, and mentors.',
});

useFunnelPageView(FunnelEvent.DirectoryViewed, { directory: 'programs' }, 'directory_viewed:programs');
</script>
