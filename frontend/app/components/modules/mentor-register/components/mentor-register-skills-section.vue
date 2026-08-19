<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8 space-y-6">
    <div>
      <h2 class="text-lg font-semibold text-neutral-900">Skills</h2>
      <p class="mt-2 text-sm text-neutral-600 leading-5">
        {{ MENTOR_REGISTER_SKILLS_INTRO }}
      </p>
    </div>

    <lfx-field
      label="Which of your skills should we feature?"
      required
    >
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center">
        <lfx-select
          :model-value="draftSkill"
          placeholder="Select a skill"
          class="flex-1"
          @update:model-value="draftSkill = $event"
        >
          <lfx-dropdown-item
            v-for="skill in availableSkills"
            :key="skill"
            :value="skill"
            :label="skill"
          />
        </lfx-select>
        <lfx-button
          label="Add Skill"
          icon="plus"
          type="primary"
          button-style="pill"
          class="shrink-0 justify-center"
          :disabled="!draftSkill"
          @click="addSkill"
        />
      </div>

      <div
        v-if="modelValue.length"
        class="mt-3 flex flex-wrap gap-2 rounded-xl border border-neutral-200 p-3"
      >
        <lfx-chip
          v-for="skill in modelValue"
          :key="skill"
          type="bordered"
          size="default"
          removable
          @dismissed="$emit('remove-skill', skill)"
        >
          {{ skill }}
        </lfx-chip>
      </div>
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
import { computed, ref } from 'vue';
import {
  MENTOR_REGISTER_SKILLS_INTRO,
} from '../config/mentor-register.config';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxChip from '~/components/uikit/chip/chip.vue';
import LfxDropdownItem from '~/components/uikit/dropdown/dropdown-item.vue';
import LfxField from '~/components/uikit/field/field.vue';
import LfxFieldMessage from '~/components/uikit/field/field-message.vue';
import LfxSelect from '~/components/uikit/select/select.vue';
import { SKILL_OPTIONS } from '~/config/skills';

const props = defineProps<{
  modelValue: string[];
  error?: string;
}>();

const emit = defineEmits<{
  (e: 'add-skill', skill: string): void;
  (e: 'remove-skill', skill: string): void;
}>();

const draftSkill = ref('');

const selectedSet = computed(() => new Set(props.modelValue.map((skill) => skill.toLowerCase())));

const availableSkills = computed(() =>
  SKILL_OPTIONS.filter((skill) => !selectedSet.value.has(skill.toLowerCase())),
);

function addSkill() {
  const value = draftSkill.value.trim();
  if (!value) return;
  emit('add-skill', value);
  draftSkill.value = '';
}
</script>

<script lang="ts">
export default {
  name: 'MentorRegisterSkillsSection',
};
</script>
