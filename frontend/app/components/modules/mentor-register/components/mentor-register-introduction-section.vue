<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8 space-y-6">
    <div>
      <h2 class="text-lg font-semibold text-neutral-900">Introduction</h2>
      <p class="mt-2 text-sm text-neutral-600 leading-5">
        {{ MENTOR_REGISTER_INTRODUCTION_INTRO }}
      </p>
    </div>

    <lfx-field
      label="Tell us a little bit about yourself."
      required
    >
      <lfx-editor
        :model-value="modelValue"
        :placeholder="MENTOR_REGISTER_INTRODUCTION_PLACEHOLDER"
        height="260px"
        @update:model-value="$emit('update:modelValue', $event)"
      />
      <lfx-field-message
        v-if="error"
        class="mt-1"
      >
        {{ error }}
      </lfx-field-message>
    </lfx-field>
  </section>
</template>

<script setup lang="ts">
import {
  MENTOR_REGISTER_INTRODUCTION_INTRO,
  MENTOR_REGISTER_INTRODUCTION_PLACEHOLDER,
} from '../config/mentor-register.config';
import LfxEditor from '~/components/uikit/editor/editor.client.vue';
import LfxField from '~/components/uikit/field/field.vue';
import LfxFieldMessage from '~/components/uikit/field/field-message.vue';

defineProps<{
  modelValue: string;
  error?: string;
}>();
defineEmits<{ (e: 'update:modelValue', value: string): void }>();
</script>

<script lang="ts">
export default {
  name: 'MentorRegisterIntroductionSection',
};
</script>
