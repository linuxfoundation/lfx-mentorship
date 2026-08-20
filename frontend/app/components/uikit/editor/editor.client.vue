<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div
    class="c-editor"
    :class="{ 'is-invalid': props.invalid, 'is-disabled': props.disabled }"
  >
    <component
      :is="PrimeEditor"
      v-if="PrimeEditor"
      v-model="value"
      :placeholder="props.placeholder"
      :readonly="props.disabled"
      :editor-style="resolvedEditorStyle"
      @load="onEditorLoad"
    >
      <template #toolbar>
        <span class="ql-formats">
          <select class="ql-header">
            <option value="1" />
            <option value="2" />
            <option value="3" />
            <option selected />
          </select>
        </span>
        <span class="ql-formats">
          <button
            type="button"
            class="ql-bold"
          />
          <button
            type="button"
            class="ql-italic"
          />
          <button
            type="button"
            class="ql-underline"
          />
          <button
            type="button"
            class="ql-strike"
          />
        </span>
        <span class="ql-formats">
          <button
            type="button"
            class="ql-list"
            value="ordered"
          />
          <button
            type="button"
            class="ql-list"
            value="bullet"
          />
        </span>
        <span class="ql-formats">
          <button
            type="button"
            class="ql-code-block"
          />
          <button
            type="button"
            class="ql-blockquote"
          />
          <button
            type="button"
            class="ql-link"
          />
        </span>
      </template>
    </component>
    <div
      v-else
      class="c-editor__fallback"
      :style="resolvedEditorStyle"
    >
      Loading editor…
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, shallowRef, type Component } from 'vue';
import { configureQuillMarkdownEditor } from '~/utils/quill-markdown-paste';

const props = withDefaults(
  defineProps<{
    modelValue: string;
    placeholder?: string;
    disabled?: boolean;
    invalid?: boolean;
    /** CSS height for the editable area, e.g. "240px". */
    height?: string;
    /** Max plain-text characters (paste + matcher). */
    maxLength?: number;
  }>(),
  {
    placeholder: '',
    disabled: false,
    invalid: false,
    height: '240px',
    maxLength: 10_000,
  },
);

const emit = defineEmits<{ (e: 'update:modelValue', value: string): void }>();

const value = computed({
  get: () => props.modelValue,
  set: (next: string) => emit('update:modelValue', next ?? ''),
});

const resolvedEditorStyle = computed(() => `height: ${props.height}; min-height: ${props.height};`);

const PrimeEditor = shallowRef<Component | null>(null);
let markdownConfigured = false;

onMounted(async () => {
  await import('quill/dist/quill.snow.css');
  const { default: Quill } = await import('quill');
  (window as Window & { Quill?: typeof Quill }).Quill = Quill;
  const { default: Editor } = await import('primevue/editor');
  PrimeEditor.value = Editor;
});

function onEditorLoad(event: { instance?: Parameters<typeof configureQuillMarkdownEditor>[0] }) {
  if (!event?.instance || markdownConfigured) {
    return;
  }
  configureQuillMarkdownEditor(event.instance, props.maxLength);
  markdownConfigured = true;
}
</script>

<script lang="ts">
export default {
  name: 'LfxEditor',
};
</script>
