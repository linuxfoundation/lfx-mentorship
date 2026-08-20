<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <NuxtLink
    :to="to"
    class="inline-flex items-center gap-1.5 text-sm font-medium text-brand-600 hover:text-brand-700"
  >
    <lfx-icon
      name="chevron-left"
      type="light"
      :size="12"
    />
    {{ label }}
  </NuxtLink>
</template>

<script setup lang="ts">
import LfxIcon from '~/components/uikit/icon/icon.vue';

defineProps<{
  to: string;
  label: string;
}>();
</script>

<script lang="ts">
export default {
  name: 'DetailBackLink',
};
</script>
