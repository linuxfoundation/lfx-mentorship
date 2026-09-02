<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <lfx-modal
    :model-value="modelValue"
    width="32rem"
    type="centered"
    :aria-label="SIGN_IN_TO_APPLY_TITLE"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <section class="p-6 md:p-8">
      <div class="flex items-start justify-between gap-4">
        <h2 class="font-secondary text-2xl font-normal text-neutral-900 leading-tight">
          {{ SIGN_IN_TO_APPLY_TITLE }}
        </h2>
        <lfx-icon-button
          icon="close"
          type="transparent"
          size="small"
          aria-label="Close"
          @click="emit('update:modelValue', false)"
        />
      </div>

      <p class="mt-3 text-sm text-neutral-500 leading-6">
        {{ SIGN_IN_TO_APPLY_BODY }}
      </p>

      <div class="mt-6 rounded-xl border border-neutral-200 bg-neutral-50 px-4 py-3.5">
        <p class="text-xxs font-semibold uppercase tracking-wide text-neutral-400">
          {{ SIGN_IN_TO_APPLY_LABEL }}
        </p>
        <p class="mt-1 text-sm font-semibold text-neutral-900">
          {{ programName }}
        </p>
        <p
          v-if="termName"
          class="mt-0.5 text-sm text-neutral-500"
        >
          {{ termName }}
        </p>
      </div>

      <div class="mt-6 flex flex-col gap-3 sm:flex-row">
        <lfx-button
          label="Sign In"
          icon="arrow-right-to-bracket"
          type="primary"
          button-style="rounded"
          :loading="isLoading"
          @click="onSignIn"
        />
        <lfx-button
          label="Create an Account"
          type="outline"
          button-style="rounded"
          :disabled="isLoading"
          @click="onCreateAccount"
        />
      </div>
    </section>
  </lfx-modal>
</template>

<script setup lang="ts">
import {
  SIGN_IN_TO_APPLY_BODY,
  SIGN_IN_TO_APPLY_LABEL,
  SIGN_IN_TO_APPLY_TITLE,
} from '../config/program-detail.config';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxIconButton from '~/components/uikit/icon-button/icon-button.vue';
import LfxModal from '~/components/uikit/modal/modal.vue';
import { useAuth } from '~/composables/useAuth';

const props = defineProps<{
  modelValue: boolean;
  programName: string;
  termName?: string;
  redirectTo?: string;
}>();

const emit = defineEmits<{ (e: 'update:modelValue', value: boolean): void }>();

const { login, isLoading } = useAuth();

function onSignIn() {
  void login(props.redirectTo);
}

function onCreateAccount() {
  void login(props.redirectTo, { screenHint: 'signup' });
}
</script>

<script lang="ts">
export default {
  name: 'SignInToApplyModal',
};
</script>
