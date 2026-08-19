<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="space-y-8">
    <admin-header
      v-model="activeTab"
      :tabs="tabs"
    />

    <admin-programs-tab
      v-if="activeTab === 'my-programs'"
      v-model:status-filter="statusFilter"
      v-model:search="search"
      :programs="programs"
      @enroll="goToEnroll"
      @open="onOpen"
      @edit="onEdit"
      @hide="onHide"
    />

    <admin-enroll-tab
      v-else
      :step="enrollStep"
      :form="enrollForm"
      :errors="stepErrors"
      :show-errors="showStepErrors"
      @update:form="onFormUpdate"
      @cancel="cancelEnroll"
      @back="goBack"
      @next="goNext"
      @edit-term="onEditTerm"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import AdminEnrollTab from '../components/admin-enroll-tab.vue';
import AdminHeader from '../components/admin-header.vue';
import AdminProgramsTab from '../components/admin-programs-tab.vue';
import {
  ADMIN_ENROLL_STEPS_ORDER,
  ADMIN_MOCK_PROGRAMS,
  adminTabItems,
  createEmptyAdminEnrollForm,
  getAdminEnrollStepErrors,
} from '../config/admin.config';
import type {
  AdminEnrollForm,
  AdminEnrollStep,
  AdminProgram,
  AdminTab,
} from '~/types/admin.types';
import { adminProgramPath } from '~/config/routes';
import useToastService from '~/components/uikit/toast/toast.service';
import { ToastTypesEnum } from '~/components/uikit/toast/types/toast.types';

const router = useRouter();
const activeTab = ref<AdminTab>('my-programs');
const statusFilter = ref('all');
const search = ref('');
const programs = ref<AdminProgram[]>(ADMIN_MOCK_PROGRAMS.map((item) => ({ ...item })));
const enrollStep = ref<AdminEnrollStep>('details');
const enrollForm = reactive(createEmptyAdminEnrollForm());
const showStepErrors = ref(false);
const { showToast } = useToastService();

const stepErrors = computed(() => getAdminEnrollStepErrors(enrollStep.value, enrollForm));

const visibleProgramCount = computed(
  () => programs.value.filter((program) => !program.hidden).length,
);

const tabs = computed(() => adminTabItems(visibleProgramCount.value));

function goToEnroll() {
  activeTab.value = 'enroll';
  enrollStep.value = 'details';
  showStepErrors.value = false;
}

function cancelEnroll() {
  if (enrollForm.logoPreviewUrl.startsWith('blob:')) {
    URL.revokeObjectURL(enrollForm.logoPreviewUrl);
  }
  activeTab.value = 'my-programs';
  enrollStep.value = 'details';
  showStepErrors.value = false;
  Object.assign(enrollForm, createEmptyAdminEnrollForm());
}

function onFormUpdate(value: AdminEnrollForm) {
  Object.assign(enrollForm, value);
}

function goBack() {
  showStepErrors.value = false;
  const index = ADMIN_ENROLL_STEPS_ORDER.indexOf(enrollStep.value);
  if (index > 0) {
    enrollStep.value = ADMIN_ENROLL_STEPS_ORDER[index - 1]!;
  }
}

function goNext() {
  const errors = getAdminEnrollStepErrors(enrollStep.value, enrollForm);
  const firstError = Object.values(errors)[0];
  if (firstError) {
    showStepErrors.value = true;
    showToast(firstError, ToastTypesEnum.warning);
    return;
  }

  if (enrollStep.value === 'prerequisites') {
    showToast('Program enrollment submitted (mock).', ToastTypesEnum.positive);
    cancelEnroll();
    return;
  }

  showStepErrors.value = false;
  const index = ADMIN_ENROLL_STEPS_ORDER.indexOf(enrollStep.value);
  const next = ADMIN_ENROLL_STEPS_ORDER[index + 1];
  if (next) enrollStep.value = next;
}

function onOpen(id: string) {
  void router.push(adminProgramPath(id));
}

function onEdit(id: string) {
  void router.push(adminProgramPath(id));
}

function onHide(id: string) {
  programs.value = programs.value.map((program) =>
    program.id === id ? { ...program, hidden: true } : program,
  );
  showToast('Program hidden from your admin list (mock).', ToastTypesEnum.info);
}

function onEditTerm(id: string) {
  showToast(`Edit term ${id} is not wired yet.`, ToastTypesEnum.info);
}

useHead({ title: 'Admin' });
</script>

<script lang="ts">
export default {
  name: 'AdminView',
};
</script>
