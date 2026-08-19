<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="flex h-screen flex-col overflow-hidden bg-neutral-50">
    <mentorship-header />
    <div class="flex min-h-0 flex-1 overflow-hidden">
      <slot />
    </div>
    <share-modal-global />
    <lfx-toast />
  </div>
</template>

<script setup lang="ts">
import MentorshipHeader from '~/components/shared/layout/header.vue';
import ShareModalGlobal from '~/components/shared/components/share/share-modal-global.vue';
import LfxToast from '~/components/uikit/toast/toast.vue';
</script>
