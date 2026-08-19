<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="space-y-6">
    <section class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8 space-y-4">
      <div>
        <h2 class="text-lg font-semibold text-neutral-900">
          <span class="text-negative-500">*</span> Prerequisites
        </h2>
        <p class="mt-2 text-sm text-neutral-600">{{ ADMIN_PREREQ_INTRO }}</p>
        <lfx-field-message
          v-if="errors.prerequisites"
          class="mt-2"
        >
          {{ errors.prerequisites }}
        </lfx-field-message>
      </div>

      <div class="overflow-x-auto rounded-xl border border-neutral-200">
        <table class="w-full min-w-[40rem] text-left">
          <thead>
            <tr class="border-b border-neutral-200">
              <th class="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-neutral-500">
                Prerequisite Name
              </th>
              <th class="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-neutral-500">
                Description
              </th>
              <th class="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-neutral-500">
                Required
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="(item, index) in form.prerequisites"
              :key="item.id"
              class="border-b border-neutral-100 last:border-b-0 align-top"
            >
              <td class="px-4 py-4 text-sm font-medium text-neutral-900 whitespace-nowrap">
                {{ item.name }}
              </td>
              <td class="px-4 py-4 text-sm text-neutral-600">
                <p>{{ item.description }}</p>
                <lfx-input
                  v-if="item.urlPlaceholder !== undefined"
                  :model-value="item.urlValue ?? ''"
                  :placeholder="item.urlPlaceholder"
                  class="mt-2 max-w-md"
                  @update:model-value="updatePrereqUrl(index, String($event))"
                />
              </td>
              <td class="px-4 py-4">
                <lfx-checkbox
                  :model-value="item.required"
                  @update:model-value="toggleRequired(index, $event)"
                />
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="flex flex-col gap-2 sm:flex-row sm:items-center">
        <lfx-button
          label="Add Custom Prerequisite"
          icon="plus"
          type="primary"
          button-style="pill"
          @click="addCustomPrerequisite"
        />
        <p class="text-xs text-neutral-500">Add materials unique to your program.</p>
      </div>
    </section>

    <section class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8 space-y-4">
      <div>
        <h2 class="text-lg font-semibold text-neutral-900">Terms and Conditions</h2>
        <p class="mt-2 text-sm text-neutral-600">{{ ADMIN_TERMS_INTRO }}</p>
      </div>

      <lfx-checkbox
        class="!flex !flex-nowrap !items-start !gap-3 w-full"
        :model-value="form.termsAccepted"
        @update:model-value="patch({ termsAccepted: $event })"
      >
        <span class="min-w-0 flex-1 text-sm text-neutral-800 leading-5">
          <span class="text-negative-500">*</span>
          I agree to the
          <a
            :href="policyHref('LFX Platform Use Agreement')"
            target="_blank"
            rel="noopener noreferrer"
            class="text-brand-600 hover:text-brand-700 font-medium"
            @click.stop
          >
            LFX Platform Use Agreement
          </a>
          and all terms incorporated therein, including the
          <a
            :href="policyHref('Service-Specific Use Terms')"
            target="_blank"
            rel="noopener noreferrer"
            class="text-brand-600 hover:text-brand-700 font-medium"
            @click.stop
          >
            Service-Specific Use Terms
          </a>
          , the
          <a
            :href="policyHref('Acceptable Use Policy')"
            target="_blank"
            rel="noopener noreferrer"
            class="text-brand-600 hover:text-brand-700 font-medium"
            @click.stop
          >
            Acceptable Use Policy
          </a>
          and the
          <a
            :href="policyHref('Privacy Policy')"
            target="_blank"
            rel="noopener noreferrer"
            class="text-brand-600 hover:text-brand-700 font-medium"
            @click.stop
          >
            Privacy Policy
          </a>
          .
        </span>
      </lfx-checkbox>
      <lfx-field-message
        v-if="errors.termsAccepted"
        class="mt-1"
      >
        {{ errors.termsAccepted }}
      </lfx-field-message>
    </section>
  </div>
</template>

<script setup lang="ts">
import {
  ADMIN_POLICY_LINKS,
  ADMIN_PREREQ_INTRO,
  ADMIN_TERMS_INTRO,
} from '../config/admin.config';
import type { AdminEnrollFieldErrors } from '../config/admin.config';
import type { AdminEnrollForm } from '~/types/admin.types';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxCheckbox from '~/components/uikit/checkbox/checkbox.vue';
import LfxFieldMessage from '~/components/uikit/field/field-message.vue';
import LfxInput from '~/components/uikit/input/input.vue';

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
}>();

function patch(partial: Partial<AdminEnrollForm>) {
  emit('update:form', { ...props.form, ...partial });
}

function policyHref(label: string): string {
  return ADMIN_POLICY_LINKS.find((link) => link.label === label)?.href ?? '#';
}

function toggleRequired(index: number, required: boolean) {
  const prerequisites = props.form.prerequisites.map((item, i) =>
    i === index ? { ...item, required } : item,
  );
  patch({ prerequisites });
}

function updatePrereqUrl(index: number, urlValue: string) {
  const prerequisites = props.form.prerequisites.map((item, i) =>
    i === index ? { ...item, urlValue } : item,
  );
  patch({ prerequisites });
}

function addCustomPrerequisite() {
  patch({
    prerequisites: [
      ...props.form.prerequisites,
      {
        id: `prereq-custom-${Date.now()}`,
        name: 'Custom Prerequisite',
        description: 'Describe what applicants should provide.',
        required: false,
      },
    ],
  });
}
</script>

<script lang="ts">
export default {
  name: 'AdminEnrollPrerequisitesStep',
};
</script>
