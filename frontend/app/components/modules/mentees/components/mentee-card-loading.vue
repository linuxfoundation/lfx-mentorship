<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="border border-neutral-200 rounded-2xl p-6 min-h-[360px] flex flex-col gap-6">
    <div class="flex items-center gap-3">
      <lfx-skeleton
        width="3rem"
        height="3rem"
        class="!rounded-full"
      />
      <div class="flex flex-col gap-2 flex-1">
        <lfx-skeleton
          width="50%"
          height="1.25rem"
        />
        <lfx-skeleton
          width="30%"
          height="0.875rem"
        />
      </div>
    </div>
    <div class="flex flex-col gap-3">
      <lfx-skeleton
        width="100%"
        height="1rem"
      />
      <lfx-skeleton
        width="95%"
        height="1rem"
      />
      <lfx-skeleton
        width="80%"
        height="1rem"
      />
    </div>
    <div class="flex gap-2">
      <lfx-skeleton
        width="4rem"
        height="1.5rem"
        class="!rounded-full"
      />
      <lfx-skeleton
        width="4rem"
        height="1.5rem"
        class="!rounded-full"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import LfxSkeleton from '~/components/uikit/skeleton/skeleton.vue';
</script>

<script lang="ts">
export default {
  name: 'MenteeCardLoading',
};
</script>
