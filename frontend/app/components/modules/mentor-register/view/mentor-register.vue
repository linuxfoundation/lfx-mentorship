<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="space-y-6">
    <header class="space-y-2">
      <h1 class="font-secondary font-light text-3xl md:text-4xl text-neutral-900">
        {{ MENTOR_REGISTER_TITLE }}
      </h1>
      <p class="text-sm text-neutral-600 leading-5 max-w-3xl">
        {{ MENTOR_REGISTER_SUBTITLE }}
      </p>
    </header>

    <mentor-register-programs-section
      :programs="programs"
      :requests="requests"
      :error="visibleErrors.programs"
      @add="addProgramRequest"
      @withdraw="withdrawRequest"
    />

    <mentor-register-introduction-section
      v-model="form.introduction"
      :error="visibleErrors.introduction"
    />

    <mentor-register-skills-section
      :model-value="form.skills"
      :error="visibleErrors.skills"
      @add-skill="addSkill"
      @remove-skill="removeSkill"
    />

    <mentor-register-links-section
      v-model:linkedin-url="form.linkedinUrl"
      v-model:github-url="form.githubUrl"
      v-model:resume-file-name="form.resumeFileName"
    />

    <mentor-register-compliance-section
      v-model="form.complianceAccepted"
      :error="visibleErrors.complianceAccepted"
    />

    <mentor-register-terms-section
      v-model="form.termsAccepted"
      :error="visibleErrors.termsAccepted"
    />

    <div class="flex flex-col gap-4 md:flex-row md:items-end md:justify-between pt-2">
      <p class="text-xs text-neutral-500 leading-5 max-w-2xl">
        {{ MENTOR_REGISTER_EXPORT_DISCLAIMER }}
      </p>
      <div class="flex items-center justify-end gap-3 shrink-0">
        <lfx-button
          label="Cancel"
          type="transparent"
          button-style="pill"
          @click="onCancel"
        />
        <lfx-button
          label="Submit"
          type="primary"
          button-style="pill"
          @click="onSubmit"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import MentorRegisterComplianceSection from '../components/mentor-register-compliance-section.vue';
import MentorRegisterIntroductionSection from '../components/mentor-register-introduction-section.vue';
import MentorRegisterLinksSection from '../components/mentor-register-links-section.vue';
import MentorRegisterProgramsSection from '../components/mentor-register-programs-section.vue';
import MentorRegisterSkillsSection from '../components/mentor-register-skills-section.vue';
import MentorRegisterTermsSection from '../components/mentor-register-terms-section.vue';
import {
  applyProgramToMentorForm,
  createEmptyMentorRegisterForm,
  getMentorRegisterErrors,
  MENTOR_REGISTER_EXPORT_DISCLAIMER,
  MENTOR_REGISTER_SEED_REQUESTS,
  MENTOR_REGISTER_SUBTITLE,
  MENTOR_REGISTER_TITLE,
} from '../config/mentor-register.config';
import { usePrograms } from '~/composables/programs/usePrograms';
import { AppRoute } from '~/config/routes';
import type { MentorProgramRequest } from '~/types/mentor-register.types';
import type { Program } from '~/types/program.types';
import LfxButton from '~/components/uikit/button/button.vue';
import useToastService from '~/components/uikit/toast/toast.service';
import { ToastTypesEnum } from '~/components/uikit/toast/types/toast.types';
import { SKILL_OPTIONS } from '~/config/skills';

const { data } = usePrograms({
  search: '',
  status: 'all',
  skill: 'all',
  sortBy: 'name_asc',
});

const programs = computed(() => data.value?.data ?? []);
const form = reactive(createEmptyMentorRegisterForm());
const requests = ref<MentorProgramRequest[]>([...MENTOR_REGISTER_SEED_REQUESTS]);
const showErrors = ref(false);
const { showToast } = useToastService();
const router = useRouter();

const formErrors = computed(() => getMentorRegisterErrors(form, requests.value.length));
const visibleErrors = computed(() => (showErrors.value ? formErrors.value : {}));

function addProgramRequest(program: Program) {
  if (requests.value.some((item) => item.programId === program.id)) return;

  requests.value = [
    ...requests.value,
    {
      id: `req-${program.id}-${Date.now()}`,
      programId: program.id,
      programName: program.name,
      status: 'pending',
    },
  ];

  Object.assign(form, applyProgramToMentorForm(form, program));
}

function withdrawRequest(requestId: string) {
  requests.value = requests.value.filter((item) => item.id !== requestId);
}

function addSkill(skill: string) {
  const normalized = skill.trim();
  if (!normalized) return;
  const allowed = SKILL_OPTIONS.find(
    (option) => option.toLowerCase() === normalized.toLowerCase(),
  );
  if (!allowed) return;
  if (form.skills.some((item) => item.toLowerCase() === allowed.toLowerCase())) return;
  form.skills = [...form.skills, allowed];
}

function removeSkill(skill: string) {
  form.skills = form.skills.filter((item) => item !== skill);
}

function onCancel() {
  router.push(AppRoute.Home);
}

function onSubmit() {
  const firstError = Object.values(formErrors.value)[0];
  if (firstError) {
    showErrors.value = true;
    showToast(firstError, ToastTypesEnum.warning);
    return;
  }

  showToast('Mentor registration submitted (mock).', ToastTypesEnum.positive);
}
</script>

<script lang="ts">
export default {
  name: 'MentorRegisterView',
};
</script>
