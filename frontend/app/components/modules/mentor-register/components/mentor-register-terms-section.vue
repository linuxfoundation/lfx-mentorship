<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8 space-y-4">
    <div>
      <h2 class="text-lg font-semibold text-neutral-900">Terms and Conditions</h2>
      <p class="mt-2 text-sm text-neutral-600 leading-5">
        {{ MENTOR_REGISTER_TERMS_INTRO }}
      </p>
    </div>

    <lfx-checkbox
      class="!flex !flex-nowrap !items-start !gap-3 w-full"
      :model-value="modelValue"
      @update:model-value="$emit('update:modelValue', $event)"
    >
      <span class="min-w-0 flex-1 text-sm text-neutral-800 leading-5">
        <span class="text-negative-500">*</span>
        I agree to the
        <a
          :href="policyHref('LFX Platform Use Agreement')"
          target="_blank"
          rel="noopener noreferrer"
          class="text-brand-600 hover:text-brand-700 font-medium"
          @click.stop
        >
          LFX Platform Use Agreement
        </a>
        and all terms incorporated therein, including the
        <a
          :href="policyHref('Service-Specific Use Terms')"
          target="_blank"
          rel="noopener noreferrer"
          class="text-brand-600 hover:text-brand-700 font-medium"
          @click.stop
        >
          Service-Specific Use Terms
        </a>
        , the
        <a
          :href="policyHref('Acceptable Use Policy')"
          target="_blank"
          rel="noopener noreferrer"
          class="text-brand-600 hover:text-brand-700 font-medium"
          @click.stop
        >
          Acceptable Use Policy
        </a>
        and the
        <a
          :href="policyHref('Privacy Policy')"
          target="_blank"
          rel="noopener noreferrer"
          class="text-brand-600 hover:text-brand-700 font-medium"
          @click.stop
        >
          Privacy Policy
        </a>
        .
      </span>
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
  MENTOR_REGISTER_POLICY_LINKS,
  MENTOR_REGISTER_TERMS_INTRO,
} from '../config/mentor-register.config';
import LfxCheckbox from '~/components/uikit/checkbox/checkbox.vue';
import LfxFieldMessage from '~/components/uikit/field/field-message.vue';

defineProps<{
  modelValue: boolean;
  error?: string;
}>();
defineEmits<{ (e: 'update:modelValue', value: boolean): void }>();

function policyHref(label: string): string {
  return MENTOR_REGISTER_POLICY_LINKS.find((link) => link.label === label)?.href ?? '#';
}
</script>

<script lang="ts">
export default {
  name: 'MentorRegisterTermsSection',
};
</script>
