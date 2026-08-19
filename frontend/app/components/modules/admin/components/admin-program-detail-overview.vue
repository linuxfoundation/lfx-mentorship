<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="flex flex-col gap-8">
    <section class="flex flex-col gap-2">
      <h2 class="text-lg font-semibold text-neutral-900">Program Description</h2>
      <p class="text-sm leading-6 text-neutral-600 whitespace-pre-line">
        {{ program.description }}
      </p>
    </section>

    <section
      v-if="program.about"
      class="flex flex-col gap-2"
    >
      <h2 class="text-lg font-semibold text-neutral-900">About This Project</h2>
      <p class="text-sm leading-6 text-neutral-600 whitespace-pre-line">
        {{ program.about }}
      </p>
    </section>
  </div>
</template>

<script setup lang="ts">
import type { AdminProgramDetail } from '~/types/admin.types';

defineProps<{ program: AdminProgramDetail }>();
</script>

<script lang="ts">
export default {
  name: 'AdminProgramDetailOverview',
};
</script>
