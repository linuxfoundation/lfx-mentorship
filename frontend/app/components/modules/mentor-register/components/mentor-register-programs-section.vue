<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8 space-y-6">
    <div>
      <h2 class="text-lg font-semibold text-neutral-900">Mentorship Program Details</h2>
      <p class="mt-2 text-sm text-neutral-600 leading-5">
        {{ MENTOR_REGISTER_PROGRAMS_INTRO }}
      </p>
    </div>

    <lfx-field
      label="Select LFX Mentorship"
      required
    >
      <lfx-select
        :model-value="selectedProgramId"
        placeholder="Select LFX Mentorship"
        @update:model-value="onSelectProgram"
      >
        <lfx-dropdown-item
          v-for="program in availablePrograms"
          :key="program.id"
          :value="program.id"
          :label="program.name"
        />
      </lfx-select>
      <p class="mt-2 text-xs text-neutral-500">{{ MENTOR_REGISTER_PROGRAMS_HELPER }}</p>
      <lfx-field-message
        v-if="error"
        class="mt-1"
      >
        {{ error }}
      </lfx-field-message>
    </lfx-field>

    <div
      v-if="requests.length"
      class="overflow-x-auto rounded-xl border border-neutral-200"
    >
      <table class="w-full min-w-[36rem] text-left">
        <thead>
          <tr class="border-b border-neutral-200">
            <th class="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-neutral-500">
              Mentorship Projects
            </th>
            <th class="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-neutral-500">
              Status
            </th>
            <th class="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-neutral-500">
              Actions
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="request in requests"
            :key="request.id"
            class="border-b border-neutral-100 last:border-b-0"
          >
            <td class="px-4 py-4">
              <NuxtLink
                :to="programPath(request.programId)"
                class="text-sm font-medium text-brand-600 hover:text-brand-700"
              >
                {{ request.programName }}
              </NuxtLink>
            </td>
            <td class="px-4 py-4">
              <lfx-tag
                :variation="MENTOR_REQUEST_STATUS_CONFIG[request.status].variation"
                size="small"
                type="solid"
              >
                {{ MENTOR_REQUEST_STATUS_CONFIG[request.status].label }}
              </lfx-tag>
            </td>
            <td class="px-4 py-4">
              <lfx-button
                label="Withdraw"
                type="transparent"
                size="small"
                class="!text-negative-600 !px-0"
                @click="$emit('withdraw', request.id)"
              />
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import {
  MENTOR_REGISTER_PROGRAMS_HELPER,
  MENTOR_REGISTER_PROGRAMS_INTRO,
  MENTOR_REQUEST_STATUS_CONFIG,
} from '../config/mentor-register.config';
import type { MentorProgramRequest } from '~/types/mentor-register.types';
import type { Program } from '~/types/program.types';
import { programPath } from '~/config/routes';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxDropdownItem from '~/components/uikit/dropdown/dropdown-item.vue';
import LfxField from '~/components/uikit/field/field.vue';
import LfxFieldMessage from '~/components/uikit/field/field-message.vue';
import LfxSelect from '~/components/uikit/select/select.vue';
import LfxTag from '~/components/uikit/tag/tag.vue';

const props = defineProps<{
  programs: Program[];
  requests: MentorProgramRequest[];
  error?: string;
}>();

const emit = defineEmits<{
  (e: 'add', program: Program): void;
  (e: 'withdraw', requestId: string): void;
}>();

const selectedProgramId = ref('');

const requestedIds = computed(() => new Set(props.requests.map((item) => item.programId)));

const availablePrograms = computed(() =>
  props.programs.filter((program) => !requestedIds.value.has(program.id)),
);

function onSelectProgram(programId: string) {
  selectedProgramId.value = '';
  const program = props.programs.find((item) => item.id === programId);
  if (program) emit('add', program);
}
</script>

<script lang="ts">
export default {
  name: 'MentorRegisterProgramsSection',
};
</script>
