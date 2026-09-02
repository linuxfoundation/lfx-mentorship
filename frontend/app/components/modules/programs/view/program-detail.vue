<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="container max-w-full overflow-hidden px-5 py-6 md:px-10 md:py-8 space-y-6">
    <div
      v-if="isLoading"
      class="flex items-center gap-2 text-neutral-500 py-16 justify-center"
    >
      <lfx-spinner />
      <span>Loading program…</span>
    </div>

    <div
      v-else-if="error || !program"
      class="rounded-2xl border border-neutral-200 bg-white p-10 text-center text-negative-600"
    >
      Program not found.
    </div>

    <template v-else>
      <program-detail-header
        :program="program"
        @open-repository="openRepository"
        @donate="openDonate"
        @apply="onApply"
      />

      <sign-in-to-apply-modal
        v-model="isSignInModalOpen"
        :program-name="program.name"
        :term-name="applyTerm?.name"
        :redirect-to="applyRedirectTo"
      />

      <section class="border border-neutral-200 rounded-lg bg-white overflow-hidden">
        <div class="border-b border-neutral-200 px-4 md:px-6">
          <div
            class="flex flex-wrap gap-1"
            role="tablist"
            aria-label="Program sections"
          >
            <lfx-button
              v-for="tab in PROGRAM_DETAIL_TABS"
              :key="tab.value"
              :label="tab.label"
              type="transparent"
              size="small"
              class="!rounded-none !px-3 !py-3 border-b-2"
              :class="
                activeTab === tab.value
                  ? '!border-brand-500 !text-brand-700 !font-semibold'
                  : '!border-transparent !text-neutral-500 hover:!text-neutral-800'
              "
              role="tab"
              :aria-selected="activeTab === tab.value"
              @click="activeTab = tab.value"
            />
          </div>
        </div>

        <div class="p-6 md:p-8">
          <program-detail-overview
            v-if="activeTab === 'overview'"
            :program="program"
          />
          <program-detail-terms
            v-else-if="activeTab === 'terms'"
            :terms="program.terms"
          />
          <program-detail-mentors
            v-else-if="activeTab === 'mentors'"
            :mentors="program.mentors"
            empty-label="Mentors"
          />
          <program-detail-mentees
            v-else-if="activeTab === 'mentees'"
            :current-mentees="currentMentees"
            :graduated-mentees="graduatedMentees"
            :is-loading="isMenteesLoading"
            :load-failed="Boolean(menteesError)"
          />
          <program-detail-sponsors
            v-else
            :sponsors="program.sponsors"
          />
        </div>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import ProgramDetailHeader from '../components/program-detail-header.vue';
import ProgramDetailMentors from '../components/program-detail-mentors.vue';
import ProgramDetailMentees from '../components/program-detail-mentees.vue';
import ProgramDetailOverview from '../components/program-detail-overview.vue';
import ProgramDetailSponsors from '../components/program-detail-sponsors.vue';
import ProgramDetailTerms from '../components/program-detail-terms.vue';
import SignInToApplyModal from '../components/sign-in-to-apply-modal.vue';
import { DEFAULT_PROGRAM_DETAIL_TAB, PROGRAM_DETAIL_TABS } from '../config/program-detail.config';
import { useAuth } from '~/composables/useAuth';
import { useProgram } from '~/composables/programs/useProgram';
import { useProgramMentees } from '~/composables/programs/useProgramMentees';
import { programPath } from '~/config/routes';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxSpinner from '~/components/uikit/spinner/spinner.vue';
import { ToastTypesEnum } from '~/components/uikit/toast/types/toast.types';
import useToastService from '~/components/uikit/toast/toast.service';
import type { ProgramTerm } from '~/types/program.types';
import { getFetchErrorMessage } from '~/utils/fetch-error';

const props = defineProps<{
  programId: string;
}>();

const programId = computed(() => props.programId);
const { data: program, isLoading, error } = useProgram(programId);
const { showToast } = useToastService();
const {
  public: { crowdfundingUrl },
} = useRuntimeConfig();

watch(error, (err) => {
  if (!import.meta.client || !err) return;
  const statusCode =
    typeof err === 'object' && err !== null && 'statusCode' in err
      ? Number((err as { statusCode?: number }).statusCode)
      : 0;
  const fallback = statusCode === 404 ? 'Program not found.' : 'Failed to load program. Please try again.';
  showToast(getFetchErrorMessage(err, fallback), ToastTypesEnum.negative);
});

const activeTab = ref(DEFAULT_PROGRAM_DETAIL_TAB);
const menteesEnabled = computed(() => activeTab.value === 'mentees');
const {
  data: mentees,
  isLoading: isMenteesLoading,
  error: menteesError,
} = useProgramMentees(programId, menteesEnabled);

watch(menteesError, (err) => {
  if (!import.meta.client || !err) return;
  showToast(getFetchErrorMessage(err, 'Failed to load mentees. Please try again.'), ToastTypesEnum.negative);
});

const currentMentees = computed(() => mentees.value?.filter((mentee) => mentee.status === 'active') ?? []);
const graduatedMentees = computed(() => mentees.value?.filter((mentee) => mentee.status === 'graduated') ?? []);

watch(programId, () => {
  activeTab.value = DEFAULT_PROGRAM_DETAIL_TAB;
});

function openRepository() {
  const url = program.value?.repositoryUrl;
  if (!url || !import.meta.client) return;
  window.open(url, '_blank', 'noopener,noreferrer');
}

function openDonate() {
  const programSlug = program.value?.slug;
  if (!programSlug || !import.meta.client) return;

  const base = String(crowdfundingUrl).replace(/\/$/, '');
  window.open(`${base}/initiatives/${programSlug}`, '_blank', 'noopener,noreferrer');
}

const isSignInModalOpen = ref(false);
const applyTerm = ref<ProgramTerm | null>(null);
const { isAuthenticated } = useAuth();

const applyRedirectTo = computed(() => {
  const id = program.value?.slug || program.value?.id || programId.value;
  const path = programPath(id);
  return applyTerm.value ? `${path}?apply=${applyTerm.value.id}` : path;
});

function onApply(term: ProgramTerm) {
  applyTerm.value = term;
  if (isAuthenticated.value) {
    return;
  }
  isSignInModalOpen.value = true;
}

useHead({
  title: computed(() => program.value?.name ?? 'Program'),
});
</script>

<script lang="ts">
export default {
  name: 'ProgramDetailView',
};
</script>
