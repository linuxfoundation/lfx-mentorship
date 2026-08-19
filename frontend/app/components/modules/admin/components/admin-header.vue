<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <header class="space-y-6">
    <div class="space-y-2">
      <h1 class="font-secondary font-light text-3xl md:text-4xl text-neutral-900">
        {{ ADMIN_TITLE }}
      </h1>
      <p class="text-sm text-neutral-600 leading-5 max-w-3xl">
        {{ ADMIN_SUBTITLE }}
      </p>
    </div>

    <div
      class="flex flex-wrap gap-1 border-b border-neutral-200"
      role="tablist"
      aria-label="Admin sections"
    >
      <lfx-button
        v-for="tab in tabs"
        :key="tab.value"
        :label="tab.label"
        type="transparent"
        size="small"
        class="!rounded-none !px-3 !py-3 border-b-2"
        :class="
          modelValue === tab.value
            ? '!border-brand-500 !text-brand-700 !font-semibold'
            : '!border-transparent !text-neutral-500 hover:!text-neutral-800'
        "
        role="tab"
        :aria-selected="modelValue === tab.value"
        @click="$emit('update:modelValue', tab.value)"
      />
    </div>
  </header>
</template>

<script setup lang="ts">
import { ADMIN_SUBTITLE, ADMIN_TITLE } from '../config/admin.config';
import type { Tab } from '~/components/uikit/tabs/types/tab.types';
import LfxButton from '~/components/uikit/button/button.vue';

defineProps<{
  modelValue: string;
  tabs: Tab[];
}>();

defineEmits<{ (e: 'update:modelValue', value: string): void }>();
</script>

<script lang="ts">
export default {
  name: 'AdminHeader',
};
</script>
