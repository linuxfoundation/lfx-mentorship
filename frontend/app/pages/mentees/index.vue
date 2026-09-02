<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <mentees-view />
</template>

<script setup lang="ts">
import MenteesView from '~/components/modules/mentees/view/mentees.vue';
import { FunnelEvent } from '~/composables/useFunnelAnalytics';

usePublicSeo({
  title: 'Mentees',
  description: 'Browse LFX Mentorship mentees. Find contributors by skill, project, and status.',
});

useFunnelPageView(FunnelEvent.DirectoryViewed, { directory: 'mentees' }, 'directory_viewed:mentees');
</script>
