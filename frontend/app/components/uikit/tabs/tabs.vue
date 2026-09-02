<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <!-- Pill style: icon + label pill tabs matching Figma design -->
  <div
    v-if="props.tabStyle === 'pill' || props.tabStyle === 'rounded'"
    class="flex gap-4"
    role="tablist"
  >
    <button
      v-for="tab in tabs"
      :key="tab.value"
      type="button"
      role="tab"
      :disabled="tab.disabled"
      :aria-selected="modelValue === tab.value"
      class="flex items-center justify-center gap-1.5 h-9 px-3 py-1 text-sm shrink-0 overflow-hidden transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
      :class="[
        props.tabStyle === 'pill' ? 'rounded-full' : 'rounded-lg',
        modelValue === tab.value
          ? 'bg-accent-100 text-neutral-900 font-semibold'
          : 'text-neutral-900 font-medium hover:bg-neutral-50'
      ]"
      @click="emit('update:modelValue', tab.value)"
    >
      <lfx-icon
        v-if="tab.icon"
        :name="tab.icon"
        :type="modelValue === tab.value ? 'solid' : 'light'"
        :size="16"
        :class="modelValue === tab.value ? 'text-accent-500' : 'text-neutral-900'"
      />
      <span>{{ tab.label }}</span>
    </button>
  </div>

  <!-- Default style: PrimeVue SelectButton -->
  <pv-select-button
    v-else
    v-model="selectValue"
    :options="props.tabs"
    option-label="label"
    data-key="value"
    option-value="value"
    :allow-empty="false"
    :class="`tabs-width-${props.widthType} tabs-style-${props.tabStyle}`"
    option-disabled="disabled"
  >
    <template #option="slotProps">
      <slot
        name="slotItem"
        :option="slotProps.option"
      >
        <i
          v-if="slotProps.option.icon"
          :class="slotProps.option.icon"
        />
        <template v-else>{{ slotProps.option.label }}</template>
      </slot>
    </template>
  </pv-select-button>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { Tab } from './types/tab.types';
import LfxIcon from '~/components/uikit/icon/icon.vue';

const props = withDefaults(
  defineProps<{
    modelValue: string;
    tabs: Tab[];
    tabStyle?: 'pill' | 'default' | 'rounded';
    widthType?: 'full' | 'inline';
  }>(),
  {
    widthType: 'full',
    tabStyle: 'default',
  },
);
const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void;
}>();

const selectValue = computed({
  get() {
    return props.modelValue || '';
  },
  set(value: string) {
    emit('update:modelValue', value);
  },
});
</script>

<script lang="ts">
export default {
  name: 'LfxTabs',
};
</script>
