<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="pb-4">
    <enroll-program-hero :cta-to="ctaTo" />
    <enroll-program-why />
    <enroll-program-how />
    <enroll-program-guidance />
    <enroll-program-cta :cta-to="ctaTo" />
  </div>
</template>

<script setup lang="ts">
import EnrollProgramCta from '../components/enroll-program-cta.vue';
import EnrollProgramGuidance from '../components/enroll-program-guidance.vue';
import EnrollProgramHero from '../components/enroll-program-hero.vue';
import EnrollProgramHow from '../components/enroll-program-how.vue';
import EnrollProgramWhy from '../components/enroll-program-why.vue';

const ctaTo = '#'; // TODO: Add the actual link to the program
</script>

<script lang="ts">
export default {
  name: 'EnrollProgramView',
};
</script>
