<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <lfx-dropdown-select
    :model-value="modelValue"
    :width="width"
    :placement="placement"
    :visibility="isOpen"
    @update:model-value="$emit('update:modelValue', String($event))"
    @update:visibility="onVisibilityChange"
  >
    <template #trigger="{ selectedOption }">
      <lfx-button
        :label="triggerLabel(selectedOption)"
        type="outline"
        button-style="pill"
        icon="tags"
      />
    </template>

    <lfx-dropdown-search
      v-model="query"
      placeholder="Search skills"
    />

    <lfx-dropdown-item
      :value="allValue"
      :label="allLabel"
    />
    <lfx-dropdown-item
      v-for="option in filteredSkills"
      :key="option"
      :value="option"
      :label="option"
    />
    <p
      v-if="!filteredSkills.length"
      class="px-3 py-2 text-sm text-neutral-400"
    >
      No skills found
    </p>
  </lfx-dropdown-select>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxDropdownItem from '~/components/uikit/dropdown/dropdown-item.vue';
import LfxDropdownSearch from '~/components/uikit/dropdown/dropdown-search.vue';
import LfxDropdownSelect from '~/components/uikit/dropdown/dropdown-select.vue';
import type { Placement } from '@popperjs/core';

const props = withDefaults(
  defineProps<{
    modelValue: string;
    skillOptions: string[];
    allValue?: string;
    allLabel?: string;
    width?: string;
    placement?: Placement;
  }>(),
  {
    allValue: 'all',
    allLabel: 'All skills',
    width: '240px',
    placement: 'bottom-end',
  },
);

defineEmits<{
  (e: 'update:modelValue', value: string): void;
}>();

const query = ref('');
const isOpen = ref(false);

const filteredSkills = computed(() => {
  const term = query.value.trim().toLowerCase();
  if (!term) return props.skillOptions;
  return props.skillOptions.filter((skill) => skill.toLowerCase().includes(term));
});

function triggerLabel(selectedOption?: { value?: string; label?: string }) {
  if (!selectedOption?.value || selectedOption.value === props.allValue) {
    return props.allLabel;
  }
  return selectedOption.label || selectedOption.value;
}

function onVisibilityChange(value: boolean) {
  isOpen.value = value;
  if (!value) query.value = '';
}
</script>

<script lang="ts">
export default {
  name: 'SkillFilterSelect',
};
</script>
