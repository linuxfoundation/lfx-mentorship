<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="h-full min-h-0 w-full overflow-y-auto bg-neutral-50">
    <div class="border-b border-neutral-200/80 py-3 md:hidden">
      <div class="container flex gap-2 overflow-x-auto">
        <NuxtLink
          v-for="item in SELF_SERVE_NAV_ITEMS"
          :key="item.id"
          :to="item.to"
          class="shrink-0 rounded-lg px-3 py-2 text-sm"
          :class="
            isActive(item.to)
              ? 'bg-brand-50 font-medium text-brand-600'
              : 'text-neutral-600'
          "
        >
          {{ item.label }}
        </NuxtLink>
      </div>
    </div>

    <div class="container flex gap-8 py-6 md:gap-10 md:py-8">
      <self-serve-sidebar class="hidden md:flex" />

      <div class="min-w-0 flex-1">
        <slot />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import SelfServeSidebar from './self-serve-sidebar.vue';
import { SELF_SERVE_NAV_ITEMS } from '../config/self-serve.config';

const route = useRoute();

function isActive(to: string): boolean {
  return route.path === to || route.path.startsWith(`${to}/`);
}
</script>

<script lang="ts">
export default {
  name: 'SelfServeShell',
};
</script>
