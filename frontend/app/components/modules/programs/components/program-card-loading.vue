<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="border border-neutral-200 rounded-xl p-6 min-h-[360px] flex flex-col gap-6">
    <lfx-skeleton
      width="3rem"
      height="3rem"
      class="!rounded-lg"
    />
    <div class="flex flex-col gap-3">
      <lfx-skeleton
        width="70%"
        height="1.25rem"
      />
      <lfx-skeleton
        width="100%"
        height="1rem"
      />
      <lfx-skeleton
        width="90%"
        height="1rem"
      />
    </div>
    <div class="flex gap-2 mt-auto">
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
  name: 'ProgramCardLoading',
};
</script>
