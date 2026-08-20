<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div
    class="c-dropdown__item"
    :class="{ 'is-selected': isSelected }"
    role="option"
    :aria-selected="isSelected"
    tabindex="0"
    @click="handleClick"
    @keydown.enter.prevent="handleClick"
    @keydown.space.prevent="handleClick"
  >
    <slot>
      {{ props.label }}
    </slot>
    <div
      v-if="isSelected || props.checkmarkBefore"
      class="flex justify-end"
      :class="props.checkmarkBefore ? 'order-first min-w-4 w-4' : 'flex-grow'"
    >
      <lfx-icon
        v-if="isSelected"
        name="check"
        :size="16"
        class="!text-brand-500"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Ref, WritableComputedRef } from 'vue';

const props = defineProps<{
  value?: string;
  label?: string;
  checkmarkBefore?: boolean;
}>();

const attrs = useAttrs();
const selectedValue = inject<WritableComputedRef<string>>('selectedValue', ref('') as WritableComputedRef<string>);
const selectedOptionProps = inject<Ref<Record<string, unknown>>>(
  'selectedOptionProps',
  ref<Record<string, unknown>>({}),
);
const closeDropdown = inject<(() => void) | undefined>('closeDropdown', undefined);

const isSelected = computed(
  () => selectedValue != null && props.value !== undefined && selectedValue.value === props.value,
);

const handleClick = () => {
  if (props.value === undefined) return;
  if (selectedOptionProps) {
    selectedOptionProps.value = { ...props, ...attrs };
  }
  if (selectedValue) {
    selectedValue.value = props.value;
  }
  closeDropdown?.();
};
</script>

<script lang="ts">
export default {
  name: 'LfxDropdownItem',
};
</script>
