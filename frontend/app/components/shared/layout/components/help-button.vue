<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="fixed bottom-6 right-6 z-40 rounded-full shadow-lg">
    <lfx-button
      label="Help"
      icon="circle-question"
      type="primary"
      button-style="pill"
      @click="isOpen = true"
    />
    <contact-support-modal v-model="isOpen" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import ContactSupportModal from '~/components/shared/layout/components/contact-support-modal.vue';
import LfxButton from '~/components/uikit/button/button.vue';

const isOpen = ref(false);
</script>

<script lang="ts">
export default {
  name: 'HelpButton',
};
</script>
