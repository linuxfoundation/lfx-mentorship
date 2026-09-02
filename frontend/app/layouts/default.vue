<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="min-h-screen flex flex-col">
    <mentorship-header />
    <main class="flex-grow">
      <slot />
    </main>
    <mentorship-footer />
    <share-modal-global />
    <lfx-toast />
  </div>
</template>

<script setup lang="ts">
import MentorshipHeader from '~/components/shared/layout/header.vue';
import MentorshipFooter from '~/components/shared/layout/footer.vue';
import ShareModalGlobal from '~/components/shared/components/share/share-modal-global.vue';
import LfxToast from '~/components/uikit/toast/toast.vue';
</script>
