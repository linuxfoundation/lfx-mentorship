<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
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
          <program-detail-members
            v-else-if="activeTab === 'mentors'"
            :members="program.mentors"
            empty-label="mentors"
          />
          <program-detail-mentees
            v-else-if="activeTab === 'mentees'"
            :current-mentees="currentMentees"
            :graduated-mentees="graduatedMentees"
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
import ProgramDetailMembers from '../components/program-detail-members.vue';
import ProgramDetailMentees from '../components/program-detail-mentees.vue';
import ProgramDetailOverview from '../components/program-detail-overview.vue';
import ProgramDetailSponsors from '../components/program-detail-sponsors.vue';
import { DEFAULT_PROGRAM_DETAIL_TAB, PROGRAM_DETAIL_TABS } from '../config/program-detail.config';
import { useProgram } from '~/composables/programs/useProgram';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxSpinner from '~/components/uikit/spinner/spinner.vue';

const props = defineProps<{
  programId: string;
}>();

const programId = computed(() => props.programId);
const { data: program, isLoading, error } = useProgram(programId);
const {
  public: { crowdfundingUrl },
} = useRuntimeConfig();

const activeTab = ref(DEFAULT_PROGRAM_DETAIL_TAB);

const currentMentees = computed(() => program.value?.mentees.filter((mentee) => mentee.status === 'active') ?? []);
const graduatedMentees = computed(() => program.value?.mentees.filter((mentee) => mentee.status === 'graduated') ?? []);

watch(programId, () => {
  activeTab.value = DEFAULT_PROGRAM_DETAIL_TAB;
});

function openRepository() {
  const url = program.value?.repositoryUrl;
  if (!url || !import.meta.client) return;
  window.open(url, '_blank', 'noopener,noreferrer');
}

function openDonate() {
  const initiativeId = program.value?.crowdfundingInitiativeId;
  if (!initiativeId || !import.meta.client) return;

  const base = String(crowdfundingUrl).replace(/\/$/, '');
  window.open(`${base}/initiatives/${initiativeId}`, '_blank', 'noopener,noreferrer');
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
