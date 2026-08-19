<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="space-y-6">
    <admin-enroll-stepper :current="step" />

    <admin-enroll-details-step
      v-if="step === 'details'"
      :form="form"
      :errors="visibleErrors"
      @update:form="$emit('update:form', $event)"
    />
    <admin-enroll-setup-step
      v-else-if="step === 'setup'"
      :form="form"
      :errors="visibleErrors"
      @update:form="$emit('update:form', $event)"
      @edit-term="$emit('edit-term', $event)"
    />
    <admin-enroll-prerequisites-step
      v-else
      :form="form"
      :errors="visibleErrors"
      @update:form="$emit('update:form', $event)"
    />

    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between pt-2">
      <lfx-button
        v-if="step === 'details'"
        label="Cancel"
        type="transparent"
        button-style="pill"
        @click="$emit('cancel')"
      />
      <lfx-button
        v-else
        :label="backLabel"
        icon="arrow-left"
        type="transparent"
        button-style="pill"
        @click="$emit('back')"
      />

      <lfx-button
        :label="nextLabel"
        type="primary"
        button-style="pill"
        class="justify-center"
        @click="$emit('next')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import AdminEnrollDetailsStep from './admin-enroll-details-step.vue';
import AdminEnrollPrerequisitesStep from './admin-enroll-prerequisites-step.vue';
import AdminEnrollSetupStep from './admin-enroll-setup-step.vue';
import AdminEnrollStepper from './admin-enroll-stepper.vue';
import { ADMIN_ENROLL_STEP_LABELS } from '../config/admin.config';
import type { AdminEnrollFieldErrors } from '../config/admin.config';
import type { AdminEnrollForm, AdminEnrollStep } from '~/types/admin.types';
import LfxButton from '~/components/uikit/button/button.vue';

const props = withDefaults(
  defineProps<{
    step: AdminEnrollStep;
    form: AdminEnrollForm;
    errors?: AdminEnrollFieldErrors;
    showErrors?: boolean;
  }>(),
  {
    errors: () => ({}),
    showErrors: false,
  },
);

const visibleErrors = computed(() => (props.showErrors ? props.errors : {}));

defineEmits<{
  (e: 'update:form', value: AdminEnrollForm): void;
  (e: 'cancel'): void;
  (e: 'back'): void;
  (e: 'next'): void;
  (e: 'edit-term', id: string): void;
}>();

const backLabel = computed(() => {
  if (props.step === 'setup') return `Back: ${ADMIN_ENROLL_STEP_LABELS.details}`;
  if (props.step === 'prerequisites') return `Back: ${ADMIN_ENROLL_STEP_LABELS.setup}`;
  return 'Back';
});

const nextLabel = computed(() => {
  if (props.step === 'details') return `Next: ${ADMIN_ENROLL_STEP_LABELS.setup}`;
  if (props.step === 'setup') return `Next: ${ADMIN_ENROLL_STEP_LABELS.prerequisites}`;
  return 'Submit';
});
</script>

<script lang="ts">
export default {
  name: 'AdminEnrollTab',
};
</script>
