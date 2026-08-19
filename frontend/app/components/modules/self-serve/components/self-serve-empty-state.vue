<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="p-0">
    <p class="text-sm text-neutral-500">{{ SELF_SERVE_EMPTY_COPY }}</p>
  </div>
</template>

<script setup lang="ts">
import { SELF_SERVE_EMPTY_COPY } from '../config/self-serve.config';
</script>

<script lang="ts">
export default {
  name: 'SelfServeEmptyState',
};
</script>
