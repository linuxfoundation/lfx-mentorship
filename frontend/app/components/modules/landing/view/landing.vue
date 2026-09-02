<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="pb-4">
    <landing-hero :summary="summary" />
    <landing-stats :summary="summary" />
    <landing-how-it-works />

    <section class="container px-5 pb-10 md:px-10 md:pb-12">
      <landing-who-can-apply />
    </section>

    <landing-benefits />
    <landing-faq />
    <landing-cta :accepting-program-count="summary.acceptingProgramCount" />
  </div>
</template>

<script setup lang="ts">
import LandingBenefits from '../components/landing-benefits.vue';
import LandingCta from '../components/landing-cta.vue';
import LandingFaq from '../components/landing-faq.vue';
import LandingHero from '../components/landing-hero.vue';
import LandingHowItWorks from '../components/landing-how-it-works.vue';
import LandingStats from '../components/landing-stats.vue';
import LandingWhoCanApply from '../components/landing-who-can-apply.vue';
import { useLandingSummary } from '~/composables/useLandingSummary';

const { data } = useLandingSummary();

const summary = computed(() => {
  return (
    data.value ?? {
      programCount: 0,
      acceptingProgramCount: 0,
      mentorCount: 0,
      graduatedMenteeCount: 0,
      foundationCount: 0,
      stipendsPaid: 0,
      graduatedMentees: [],
    }
  );
});
</script>

<script lang="ts">
export default {
  name: 'LandingView',
};
</script>
