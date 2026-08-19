<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8 space-y-4">
    <h2 class="text-lg font-semibold text-neutral-900">Compliance Confirmation</h2>

    <lfx-checkbox
      class="!flex !flex-nowrap !items-start !gap-3 w-full"
      :model-value="modelValue"
      @update:model-value="$emit('update:modelValue', $event)"
    >
      <div class="min-w-0 flex-1">
        <span class="text-sm text-neutral-800 leading-5">
          <span class="text-negative-500">*</span>
          {{ MENTOR_REGISTER_COMPLIANCE_LEAD }}
        </span>
        <ul class="mt-2 list-disc pl-5 space-y-2 text-sm text-neutral-600 leading-5">
          <li
            v-for="item in MENTOR_REGISTER_COMPLIANCE_ITEMS"
            :key="item"
          >
            {{ item }}
          </li>
        </ul>
      </div>
    </lfx-checkbox>
    <lfx-field-message
      v-if="error"
      class="mt-1"
    >
      {{ error }}
    </lfx-field-message>
  </section>
</template>

<script setup lang="ts">
import {
  MENTOR_REGISTER_COMPLIANCE_ITEMS,
  MENTOR_REGISTER_COMPLIANCE_LEAD,
} from '../config/mentor-register.config';
import LfxCheckbox from '~/components/uikit/checkbox/checkbox.vue';
import LfxFieldMessage from '~/components/uikit/field/field-message.vue';

defineProps<{
  modelValue: boolean;
  error?: string;
}>();
defineEmits<{ (e: 'update:modelValue', value: boolean): void }>();
</script>

<script lang="ts">
export default {
  name: 'MentorRegisterComplianceSection',
};
</script>
