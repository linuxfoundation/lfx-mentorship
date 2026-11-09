<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <lfx-modal
    :model-value="modelValue"
    width="32rem"
    type="centered"
    aria-label="Contact support"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <section class="p-6 md:p-8">
      <div class="flex items-start justify-between gap-4">
        <h2 class="font-secondary text-2xl font-normal text-neutral-900 leading-tight">Contact support</h2>
        <lfx-icon-button
          icon="close"
          type="transparent"
          size="small"
          aria-label="Close"
          @click="emit('update:modelValue', false)"
        />
      </div>

      <p class="mt-3 text-sm text-neutral-500 leading-6">
        To chat with support, sign in to LFX Self Serve and use the chat button there.
      </p>

      <div class="mt-6">
        <lfx-button
          label="Go to LFX Self Serve"
          icon="arrow-right-to-bracket"
          type="primary"
          button-style="rounded"
          @click="goToSelfServe"
        />
      </div>
    </section>
  </lfx-modal>
</template>

<script setup lang="ts">
import { selfServeMenteeUrl } from '~/config/routes';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxIconButton from '~/components/uikit/icon-button/icon-button.vue';
import LfxModal from '~/components/uikit/modal/modal.vue';

defineProps<{ modelValue: boolean }>();

const emit = defineEmits<{ (e: 'update:modelValue', value: boolean): void }>();

const {
  public: { selfServeUrl },
} = useRuntimeConfig();

// Intercom chat is for signed-in users only, and sign-in lives in Self Serve.
function goToSelfServe() {
  window.open(selfServeMenteeUrl(selfServeUrl), '_blank', 'noopener,noreferrer');
  emit('update:modelValue', false);
}
</script>

<script lang="ts">
export default {
  name: 'ContactSupportModal',
};
</script>
