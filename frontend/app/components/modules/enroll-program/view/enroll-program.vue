<!--
Copyright The Linux Foundation and each contributor to LFX.
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
import { selfServeEnrollProgramUrl } from '~/config/routes';
import EnrollProgramCta from '../components/enroll-program-cta.vue';
import EnrollProgramGuidance from '../components/enroll-program-guidance.vue';
import EnrollProgramHero from '../components/enroll-program-hero.vue';
import EnrollProgramHow from '../components/enroll-program-how.vue';
import EnrollProgramWhy from '../components/enroll-program-why.vue';

const {
  public: { selfServeUrl },
} = useRuntimeConfig();
const ctaTo = selfServeEnrollProgramUrl(selfServeUrl);
</script>

<script lang="ts">
export default {
  name: 'EnrollProgramView',
};
</script>
