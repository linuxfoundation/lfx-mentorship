<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between text-sm text-neutral-500">
    <p>
      Page {{ page }} ({{ rangeStart }} – {{ rangeEnd }})
    </p>
    <div class="flex items-center gap-3">
      <span>Page Size: {{ pageSize }}</span>
      <lfx-button
        label="Previous"
        type="outline"
        button-style="rounded"
        size="small"
        :disabled="page <= 1"
        @click="$emit('update:page', page - 1)"
      />
      <lfx-button
        label="Next"
        type="outline"
        button-style="rounded"
        size="small"
        :disabled="page >= totalPages"
        @click="$emit('update:page', page + 1)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import LfxButton from '~/components/uikit/button/button.vue';

const props = defineProps<{
  page: number;
  pageSize: number;
  total: number;
}>();

defineEmits<{ (e: 'update:page', value: number): void }>();

const totalPages = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)));
const rangeStart = computed(() => (props.total === 0 ? 0 : (props.page - 1) * props.pageSize + 1));
const rangeEnd = computed(() => Math.min(props.page * props.pageSize, props.total));
</script>

<script lang="ts">
export default {
  name: 'AdminTablePagination',
};
</script>
