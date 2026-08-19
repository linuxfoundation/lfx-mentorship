<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8 space-y-6">
    <div>
      <h2 class="text-lg font-semibold text-neutral-900">External Profile Links</h2>
      <p class="mt-2 text-sm text-neutral-600 leading-5">
        {{ MENTOR_REGISTER_LINKS_INTRO }}
      </p>
    </div>

    <lfx-field label="LinkedIn Profile">
      <lfx-input
        :model-value="linkedinUrl"
        placeholder="https://www.linkedin.com/in/username"
        @update:model-value="$emit('update:linkedinUrl', String($event))"
      />
    </lfx-field>

    <lfx-field label="GitHub Profile">
      <lfx-input
        :model-value="githubUrl"
        placeholder="https://github.com/username"
        @update:model-value="$emit('update:githubUrl', String($event))"
      />
    </lfx-field>

    <lfx-field label="Upload Resume">
      <div class="flex items-stretch overflow-hidden rounded-lg border border-neutral-200 bg-white">
        <div class="flex flex-1 items-center px-3 text-sm text-neutral-500 truncate min-w-0">
          {{ resumeFileName || 'Choose file' }}
        </div>
        <lfx-button
          label="Browse"
          type="primary"
          size="small"
          class="!rounded-none shrink-0"
          @click="fileInput?.click()"
        />
      </div>
      <input
        ref="fileInput"
        type="file"
        class="hidden"
        :accept="MENTOR_REGISTER_RESUME_ACCEPT"
        @change="onFileChange"
      />
      <p class="mt-2 text-xs text-neutral-500">
        {{ MENTOR_REGISTER_RESUME_HELPER }}
        <span class="mx-2">·</span>
        {{ MENTOR_REGISTER_RESUME_SIZE_HELPER }}
      </p>
      <p
        v-if="fileError"
        class="mt-1 text-xs text-negative-500"
      >
        {{ fileError }}
      </p>
    </lfx-field>
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import {
  MENTOR_REGISTER_LINKS_INTRO,
  MENTOR_REGISTER_RESUME_ACCEPT,
  MENTOR_REGISTER_RESUME_HELPER,
  MENTOR_REGISTER_RESUME_MAX_BYTES,
  MENTOR_REGISTER_RESUME_SIZE_HELPER,
} from '../config/mentor-register.config';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxField from '~/components/uikit/field/field.vue';
import LfxInput from '~/components/uikit/input/input.vue';

defineProps<{
  linkedinUrl: string;
  githubUrl: string;
  resumeFileName: string;
}>();

const emit = defineEmits<{
  (e: 'update:linkedinUrl', value: string): void;
  (e: 'update:githubUrl', value: string): void;
  (e: 'update:resumeFileName', value: string): void;
}>();

const fileInput = ref<HTMLInputElement | null>(null);
const fileError = ref('');

function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  fileError.value = '';

  if (!file) {
    emit('update:resumeFileName', '');
    return;
  }

  const lower = file.name.toLowerCase();
  const allowed =
    lower.endsWith('.pdf') || lower.endsWith('.doc') || lower.endsWith('.docx');

  if (!allowed) {
    fileError.value = 'Please upload a PDF, DOC, or DOCX file.';
    input.value = '';
    return;
  }

  if (file.size > MENTOR_REGISTER_RESUME_MAX_BYTES) {
    fileError.value = 'File must be 10 MB or smaller.';
    input.value = '';
    return;
  }

  emit('update:resumeFileName', file.name);
}
</script>

<script lang="ts">
export default {
  name: 'MentorRegisterLinksSection',
};
</script>
