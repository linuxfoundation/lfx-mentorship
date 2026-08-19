<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="space-y-6">
    <section class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8 space-y-6">
      <div>
        <h2 class="text-lg font-semibold text-neutral-900">Program Setup</h2>
        <p class="mt-2 text-sm text-neutral-600">{{ ADMIN_SETUP_INTRO }}</p>
      </div>

      <lfx-field
        label="Required and/or desirable skills and training"
        required
      >
        <p class="text-sm text-neutral-500 mb-3">{{ ADMIN_SETUP_SKILLS_HELPER }}</p>
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center">
          <lfx-select
            v-model="draftSkill"
            placeholder="Skill Name"
            class="flex-1"
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
          v-if="form.skills.length"
          class="mt-3 flex flex-wrap gap-2 rounded-xl border border-neutral-200 p-3"
        >
          <lfx-chip
            v-for="skill in form.skills"
            :key="skill"
            type="bordered"
            removable
            @dismissed="removeSkill(skill)"
          >
            {{ skill }}
          </lfx-chip>
        </div>
        <lfx-field-message
          v-if="errors.skills"
          class="mt-1"
        >
          {{ errors.skills }}
        </lfx-field-message>
      </lfx-field>

      <div
        class="flex items-start gap-3 rounded-xl border border-brand-100 bg-brand-50 px-4 py-3 text-sm text-brand-700"
      >
        <lfx-icon
          name="circle-info"
          type="solid"
          :size="16"
          class="shrink-0 mt-0.5"
        />
        <p>{{ ADMIN_SETUP_MENTOR_INFO }}</p>
      </div>
    </section>

    <section class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8 space-y-4">
      <div>
        <h2 class="text-lg font-semibold text-neutral-900">
          <span class="text-negative-500">*</span> Program Terms
        </h2>
        <p class="mt-2 text-sm text-neutral-600">{{ ADMIN_SETUP_TERMS_HELPER }}</p>
      </div>

      <p class="text-sm font-medium text-neutral-800">Current Terms</p>
      <div class="overflow-x-auto rounded-xl border border-neutral-200">
        <table class="w-full min-w-[28rem] text-left">
          <thead>
            <tr class="border-b border-neutral-200">
              <th class="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-neutral-500">
                Term
              </th>
              <th class="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-neutral-500">
                Starts
              </th>
              <th class="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-neutral-500">
                Ends
              </th>
              <th class="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-neutral-500">
                Actions
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="term in form.terms"
              :key="term.id"
              class="border-b border-neutral-100 last:border-b-0"
            >
              <td class="px-4 py-3 text-sm text-neutral-800">{{ term.name }}</td>
              <td class="px-4 py-3 text-sm text-neutral-600">{{ term.startsLabel }}</td>
              <td class="px-4 py-3 text-sm text-neutral-600">{{ term.endsLabel }}</td>
              <td class="px-4 py-3">
                <div class="flex items-center gap-3">
                  <lfx-button
                    label="Edit"
                    type="transparent"
                    size="small"
                    class="!text-brand-600 !px-0"
                    @click="$emit('edit-term', term.id)"
                  />
                  <lfx-button
                    label="Delete"
                    type="transparent"
                    size="small"
                    class="!text-negative-600 !px-0"
                    @click="deleteTerm(term.id)"
                  />
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <lfx-field-message
        v-if="errors.terms"
        class="mt-1"
      >
        {{ errors.terms }}
      </lfx-field-message>

      <lfx-button
        label="Add Term"
        icon="plus"
        type="primary"
        button-style="pill"
        @click="addTerm"
      />
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import {
  ADMIN_SETUP_INTRO,
  ADMIN_SETUP_MENTOR_INFO,
  ADMIN_SETUP_SKILLS_HELPER,
  ADMIN_SETUP_TERMS_HELPER,
} from '../config/admin.config';
import { SKILL_OPTIONS } from '~/config/skills';
import type { AdminEnrollFieldErrors } from '../config/admin.config';
import type { AdminEnrollForm } from '~/types/admin.types';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxChip from '~/components/uikit/chip/chip.vue';
import LfxDropdownItem from '~/components/uikit/dropdown/dropdown-item.vue';
import LfxField from '~/components/uikit/field/field.vue';
import LfxFieldMessage from '~/components/uikit/field/field-message.vue';
import LfxIcon from '~/components/uikit/icon/icon.vue';
import LfxSelect from '~/components/uikit/select/select.vue';

const props = withDefaults(
  defineProps<{
    form: AdminEnrollForm;
    errors?: AdminEnrollFieldErrors;
  }>(),
  {
    errors: () => ({}),
  },
);

const emit = defineEmits<{
  (e: 'update:form', value: AdminEnrollForm): void;
  (e: 'edit-term', id: string): void;
}>();

const draftSkill = ref('');

const selectedSkills = computed(
  () => new Set(props.form.skills.map((item) => item.toLowerCase())),
);

const availableSkills = computed(() =>
  SKILL_OPTIONS.filter((skill) => !selectedSkills.value.has(skill.toLowerCase())),
);

function patch(partial: Partial<AdminEnrollForm>) {
  emit('update:form', { ...props.form, ...partial });
}

function addSkill() {
  const value = draftSkill.value.trim();
  if (!value || selectedSkills.value.has(value.toLowerCase())) return;
  patch({ skills: [...props.form.skills, value] });
  draftSkill.value = '';
}

function removeSkill(skill: string) {
  patch({ skills: props.form.skills.filter((item) => item !== skill) });
}

function deleteTerm(id: string) {
  patch({ terms: props.form.terms.filter((term) => term.id !== id) });
}

function addTerm() {
  const next = props.form.terms.length + 1;
  patch({
    terms: [
      ...props.form.terms,
      {
        id: `term-new-${Date.now()}`,
        name: `Term ${next}`,
        startsLabel: 'TBD',
        endsLabel: 'TBD',
      },
    ],
  });
}
</script>

<script lang="ts">
export default {
  name: 'AdminEnrollSetupStep',
};
</script>
