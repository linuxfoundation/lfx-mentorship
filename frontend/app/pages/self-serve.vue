<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <self-serve-shell>
    <NuxtPage />
  </self-serve-shell>
</template>

<script setup lang="ts">
import SelfServeShell from '~/components/modules/self-serve/components/self-serve-shell.vue';

definePageMeta({
  layout: 'self-serve',
});

useHead({ title: 'Self Serve' });
</script>
