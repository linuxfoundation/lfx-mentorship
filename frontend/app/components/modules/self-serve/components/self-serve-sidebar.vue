<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <aside class="sticky top-8 flex w-44 shrink-0 flex-col self-start bg-transparent">
    <p class="mb-3 px-3 text-[11px] font-semibold uppercase tracking-wider text-neutral-400">
      {{ SELF_SERVE_SECTION_LABEL }}
    </p>
    <nav
      class="flex flex-col gap-1"
      aria-label="Self serve mentorship"
    >
      <NuxtLink
        v-for="item in SELF_SERVE_NAV_ITEMS"
        :key="item.id"
        :to="item.to"
        class="flex items-center gap-2.5 rounded-lg px-3 py-2.5 text-sm transition-colors"
        :class="
          isActive(item.to)
            ? 'bg-brand-50 font-medium text-brand-600'
            : 'text-neutral-700 hover:text-neutral-900'
        "
      >
        <lfx-icon
          :name="item.icon"
          type="solid"
          :size="16"
          class="shrink-0 w-4 text-center"
          :class="isActive(item.to) ? 'text-brand-600' : 'text-neutral-500'"
        />
        <span>{{ item.label }}</span>
      </NuxtLink>
    </nav>
  </aside>
</template>

<script setup lang="ts">
import {
  SELF_SERVE_NAV_ITEMS,
  SELF_SERVE_SECTION_LABEL,
} from '../config/self-serve.config';
import LfxIcon from '~/components/uikit/icon/icon.vue';

const route = useRoute();

function isActive(to: string): boolean {
  return route.path === to || route.path.startsWith(`${to}/`);
}
</script>

<script lang="ts">
export default {
  name: 'SelfServeSidebar',
};
</script>
