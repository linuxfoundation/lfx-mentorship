<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="space-y-6">
    <section class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8 space-y-6">
      <div>
        <h2 class="text-lg font-semibold text-neutral-900">Program Details</h2>
        <p class="mt-2 text-sm text-neutral-600">{{ ADMIN_DETAILS_INTRO }}</p>
      </div>

      <lfx-field label="Import from existing program">
        <lfx-select
          :model-value="form.importProgramId"
          placeholder="Select Program..."
          @update:model-value="onImportProgram"
        >
          <lfx-dropdown-item
            value=""
            label="None"
          />
          <lfx-dropdown-item
            v-for="option in ADMIN_IMPORT_PROGRAM_OPTIONS"
            :key="option.value"
            :value="option.value"
            :label="option.label"
          />
        </lfx-select>
        <p class="mt-2 text-xs text-neutral-500">
          Optionally start from a program you already administer.
        </p>
      </lfx-field>

      <lfx-field
        label="Program Name"
        required
        class="relative"
      >
        <div class="absolute top-0 right-0 text-xs text-neutral-400">
          {{ form.name.length }} / {{ ADMIN_NAME_MAX }}
        </div>
        <lfx-input
          :model-value="form.name"
          :maxlength="ADMIN_NAME_MAX"
          placeholder="Enter program name"
          @update:model-value="patch({ name: String($event).slice(0, ADMIN_NAME_MAX) })"
        />
        <lfx-field-message
          v-if="errors.name"
          class="mt-1"
        >
          {{ errors.name }}
        </lfx-field-message>
      </lfx-field>

      <lfx-field
        label="Linux Foundation Open Source Project"
        required
      >
        <lfx-select
          :model-value="form.projectId"
          placeholder="Select a project"
          @update:model-value="patch({ projectId: $event })"
        >
          <lfx-dropdown-item
            v-for="option in ADMIN_PROJECT_OPTIONS"
            :key="option.value"
            :value="option.value"
            :label="option.label"
          />
        </lfx-select>
        <lfx-field-message
          v-if="errors.projectId"
          class="mt-1"
        >
          {{ errors.projectId }}
        </lfx-field-message>
      </lfx-field>

      <lfx-field
        label="Technologies"
        required
      >
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center">
          <lfx-select
            v-model="draftTechnology"
            placeholder="Select a technology"
            class="flex-1"
          >
            <lfx-dropdown-item
              v-for="tech in availableTechnologies"
              :key="tech"
              :value="tech"
              :label="tech"
            />
          </lfx-select>
          <lfx-button
            label="Add Technology"
            icon="plus"
            type="primary"
            button-style="pill"
            class="shrink-0 justify-center"
            :disabled="!draftTechnology"
            @click="addTechnology"
          />
        </div>
        <div
          v-if="form.technologies.length"
          class="mt-3 flex flex-wrap gap-2 rounded-xl border border-neutral-200 p-3"
        >
          <lfx-chip
            v-for="tech in form.technologies"
            :key="tech"
            type="bordered"
            removable
            @dismissed="removeTechnology(tech)"
          >
            {{ tech }}
          </lfx-chip>
        </div>
        <lfx-field-message
          v-if="errors.technologies"
          class="mt-1"
        >
          {{ errors.technologies }}
        </lfx-field-message>
      </lfx-field>

      <lfx-field
        label="Program Description"
        required
      >
        <lfx-editor
          :model-value="form.description"
          placeholder="Describe the mentorship program"
          height="220px"
          :max-length="ADMIN_DESCRIPTION_MAX"
          @update:model-value="onDescriptionUpdate"
        />
        <p class="mt-2 text-xs text-neutral-400 text-right">
          {{ descriptionLength }} / {{ ADMIN_DESCRIPTION_MAX }}
        </p>
        <lfx-field-message
          v-if="errors.description"
          class="mt-1"
        >
          {{ errors.description }}
        </lfx-field-message>
      </lfx-field>

      <lfx-field
        label="Repository URL"
        required
      >
        <lfx-input
          :model-value="form.repositoryUrl"
          placeholder="https://github.com/org/repo"
          @update:model-value="patch({ repositoryUrl: String($event) })"
        />
        <p class="mt-2 text-xs text-neutral-500">
          Public repository mentees will contribute to during the program.
        </p>
        <lfx-field-message
          v-if="errors.repositoryUrl"
          class="mt-1"
        >
          {{ errors.repositoryUrl }}
        </lfx-field-message>
      </lfx-field>

      <lfx-field label="Website URL">
        <lfx-input
          :model-value="form.websiteUrl"
          placeholder="https://example.org"
          @update:model-value="patch({ websiteUrl: String($event) })"
        />
        <p class="mt-2 text-xs text-neutral-500">Optional project or program homepage.</p>
      </lfx-field>
    </section>

    <section class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8 space-y-4">
      <div>
        <h2 class="text-lg font-semibold text-neutral-900">
          Core Infrastructure Initiative Best Practices
        </h2>
        <p class="mt-2 text-sm text-neutral-600 leading-5">
          The CII Best Practices badge program helps open source projects demonstrate security and
          quality practices. Enter your project ID if you already participate.
        </p>
      </div>
      <lfx-field label="CII Project ID">
        <lfx-input
          :model-value="form.ciiProjectId"
          placeholder="1234"
          @update:model-value="patch({ ciiProjectId: String($event) })"
        >
          <template #prefix>
            <span class="text-neutral-400 text-sm">#</span>
          </template>
        </lfx-input>
      </lfx-field>
      <a
        href="https://bestpractices.coreinfrastructure.org/"
        target="_blank"
        rel="noopener noreferrer"
        class="inline-flex text-sm font-medium text-brand-600 hover:text-brand-700"
      >
        Apply for CII
      </a>
    </section>

    <section class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8 space-y-4">
      <div>
        <h2 class="text-lg font-semibold text-neutral-900">Code of Conduct</h2>
        <p class="mt-2 text-sm text-neutral-600 leading-5">
          A Code of Conduct sets expectations for respectful participation. Link to your project’s
          Code of Conduct, or start from a community template.
        </p>
      </div>
      <lfx-field label="Code of Conduct URL">
        <lfx-input
          :model-value="form.codeOfConductUrl"
          placeholder="https://example.org/code-of-conduct"
          @update:model-value="patch({ codeOfConductUrl: String($event) })"
        />
      </lfx-field>
      <a
        href="https://www.contributor-covenant.org/"
        target="_blank"
        rel="noopener noreferrer"
        class="inline-flex text-sm font-medium text-brand-600 hover:text-brand-700"
      >
        Start from a template
      </a>
    </section>

    <section class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8 space-y-4">
      <div>
        <h2 class="text-lg font-semibold text-neutral-900">Program Design</h2>
        <p class="mt-2 text-sm text-neutral-600">
          The logo appears on your program page and in search results.
        </p>
      </div>
      <div class="flex flex-col gap-4 md:flex-row md:items-start">
        <lfx-field
          label="Program Logo"
          required
          class="flex-1"
        >
          <div class="flex items-stretch overflow-hidden rounded-lg border border-neutral-200 bg-white">
            <div class="flex flex-1 items-center px-3 text-sm text-neutral-500 truncate min-w-0">
              {{ form.logoFileName || 'Choose file' }}
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
            :accept="ADMIN_LOGO_ACCEPT"
            @change="onLogoChange"
          />
          <p class="mt-2 text-xs text-neutral-500">{{ ADMIN_LOGO_HELPER }}</p>
          <p
            v-if="logoError"
            class="mt-1 text-xs text-negative-500"
          >
            {{ logoError }}
          </p>
          <lfx-field-message
            v-else-if="errors.logoFileName"
            class="mt-1"
          >
            {{ errors.logoFileName }}
          </lfx-field-message>
        </lfx-field>
        <div
          class="flex size-28 shrink-0 items-center justify-center overflow-hidden rounded-xl border bg-neutral-50"
          :class="
            form.logoPreviewUrl
              ? 'border-neutral-200'
              : 'border-dashed border-neutral-300'
          "
        >
          <img
            v-if="form.logoPreviewUrl"
            :src="form.logoPreviewUrl"
            :alt="form.logoFileName || 'Program logo'"
            class="size-full object-contain"
          />
          <lfx-icon
            v-else
            name="image"
            type="light"
            :size="28"
            class="text-neutral-300"
          />
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import {
  ADMIN_DESCRIPTION_MAX,
  ADMIN_DETAILS_INTRO,
  ADMIN_IMPORT_PROGRAM_OPTIONS,
  ADMIN_LOGO_ACCEPT,
  ADMIN_LOGO_HELPER,
  ADMIN_LOGO_MAX_BYTES,
  ADMIN_NAME_MAX,
  ADMIN_PROJECT_OPTIONS,
  formFromImportedProgram,
} from '../config/admin.config';
import { SKILL_OPTIONS } from '~/config/skills';
import type { AdminEnrollFieldErrors } from '../config/admin.config';
import type { AdminEnrollForm } from '~/types/admin.types';
import { plainTextLength } from '~/utils/html-text';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxChip from '~/components/uikit/chip/chip.vue';
import LfxDropdownItem from '~/components/uikit/dropdown/dropdown-item.vue';
import LfxEditor from '~/components/uikit/editor/editor.client.vue';
import LfxField from '~/components/uikit/field/field.vue';
import LfxFieldMessage from '~/components/uikit/field/field-message.vue';
import LfxIcon from '~/components/uikit/icon/icon.vue';
import LfxInput from '~/components/uikit/input/input.vue';
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
}>();

const draftTechnology = ref('');
const fileInput = ref<HTMLInputElement | null>(null);
const logoError = ref('');

function revokeLogoPreview() {
  if (props.form.logoPreviewUrl.startsWith('blob:')) {
    URL.revokeObjectURL(props.form.logoPreviewUrl);
  }
}

const descriptionLength = computed(() => plainTextLength(props.form.description));

const selectedTech = computed(
  () => new Set(props.form.technologies.map((item) => item.toLowerCase())),
);

const availableTechnologies = computed(() =>
  SKILL_OPTIONS.filter((tech) => !selectedTech.value.has(tech.toLowerCase())),
);

function patch(partial: Partial<AdminEnrollForm>) {
  emit('update:form', { ...props.form, ...partial });
}

function onImportProgram(programId: string) {
  revokeLogoPreview();
  if (fileInput.value) fileInput.value.value = '';
  emit('update:form', formFromImportedProgram(programId));
}

function onDescriptionUpdate(html: string) {
  if (plainTextLength(html) > ADMIN_DESCRIPTION_MAX) return;
  patch({ description: html });
}

function addTechnology() {
  const value = draftTechnology.value.trim();
  if (!value) return;
  if (selectedTech.value.has(value.toLowerCase())) return;
  patch({ technologies: [...props.form.technologies, value] });
  draftTechnology.value = '';
}

function removeTechnology(tech: string) {
  patch({ technologies: props.form.technologies.filter((item) => item !== tech) });
}

function onLogoChange(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  logoError.value = '';
  if (!file) {
    revokeLogoPreview();
    patch({ logoFileName: '', logoPreviewUrl: '' });
    return;
  }
  if (file.size > ADMIN_LOGO_MAX_BYTES) {
    logoError.value = 'File must be 2 MB or smaller.';
    input.value = '';
    revokeLogoPreview();
    patch({ logoFileName: '', logoPreviewUrl: '' });
    return;
  }
  revokeLogoPreview();
  patch({
    logoFileName: file.name,
    logoPreviewUrl: URL.createObjectURL(file),
  });
}
</script>

<script lang="ts">
export default {
  name: 'AdminEnrollDetailsStep',
};
</script>
