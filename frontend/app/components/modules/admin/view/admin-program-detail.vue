<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="space-y-6">
    <nav
      class="flex w-full max-w-full items-center gap-2 overflow-hidden text-sm text-neutral-500"
      aria-label="Breadcrumb"
    >
      <NuxtLink
        :to="AppRoute.SelfServeAdmin"
        class="shrink-0 text-brand-600 hover:text-brand-700 font-medium"
      >
        Admin
      </NuxtLink>
      <span
        class="shrink-0 text-neutral-400"
        aria-hidden="true"
      >
        /
      </span>
      <span
        class="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap text-neutral-800"
        :title="program?.name ?? 'Program'"
      >
        {{ program?.name ?? 'Program' }}
      </span>
    </nav>

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
      <admin-program-detail-header
        :program="program"
        @open-repository="openRepository"
        @edit="onEdit"
      />

      <section class="border border-neutral-200 rounded-2xl bg-white overflow-hidden">
        <div class="border-b border-neutral-200 px-4 md:px-6">
          <div
            class="flex flex-wrap gap-1"
            role="tablist"
            aria-label="Admin program sections"
          >
            <lfx-button
              v-for="tab in ADMIN_PROGRAM_DETAIL_TAB_ITEMS"
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
              @click="activeTab = tab.value as AdminProgramDetailTab"
            />
          </div>
        </div>

        <div class="p-6 md:p-8">
          <admin-program-detail-overview
            v-if="activeTab === 'overview'"
            :program="program"
          />
          <admin-program-detail-mentees
            v-else-if="activeTab === 'current-mentees'"
            variant="current"
            :rows="program.currentMentees"
            @add-task="toastSoon('Add Task')"
            @decline-by-term="toastSoon('Decline by Term')"
            @download="toastSoon('Download By Status')"
            @open-mentee="toastSoon('Open mentee')"
            @add-note="toastSoon('Add note')"
            @view-tasks="toastSoon('View Tasks')"
          />
          <admin-program-detail-mentees
            v-else-if="activeTab === 'past-mentees'"
            variant="past"
            :rows="program.pastMentees"
            @download="toastSoon('Download By Status')"
            @open-mentee="toastSoon('Open mentee')"
            @add-note="toastSoon('Add note')"
            @view-tasks="toastSoon('View Tasks')"
          />
          <admin-program-detail-mentors
            v-else-if="activeTab === 'mentors'"
            :rows="program.mentors"
            @invite="toastSoon('Invite mentor')"
            @open-mentor="toastSoon('Open mentor')"
            @remove="toastSoon('Remove mentor')"
          />
          <admin-program-detail-terms
            v-else
            :rows="program.managedTerms"
            @create-term="toastSoon('Create Term')"
            @open-term="toastSoon('Open term')"
            @term-actions="toastSoon('Term actions')"
          />
        </div>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import AdminProgramDetailHeader from '../components/admin-program-detail-header.vue';
import AdminProgramDetailMentees from '../components/admin-program-detail-mentees.vue';
import AdminProgramDetailMentors from '../components/admin-program-detail-mentors.vue';
import AdminProgramDetailOverview from '../components/admin-program-detail-overview.vue';
import AdminProgramDetailTerms from '../components/admin-program-detail-terms.vue';
import {
  ADMIN_PROGRAM_DETAIL_TAB_ITEMS,
  DEFAULT_ADMIN_PROGRAM_DETAIL_TAB,
} from '../config/admin-program-detail.config';
import { useAdminProgram } from '~/composables/admin/useAdminProgram';
import { AppRoute } from '~/config/routes';
import type { AdminProgramDetailTab } from '~/types/admin.types';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxSpinner from '~/components/uikit/spinner/spinner.vue';
import useToastService from '~/components/uikit/toast/toast.service';
import { ToastTypesEnum } from '~/components/uikit/toast/types/toast.types';

const props = defineProps<{ programId: string }>();

const programId = computed(() => props.programId);
const { data: program, isLoading, error } = useAdminProgram(programId);
const activeTab = ref<AdminProgramDetailTab>(DEFAULT_ADMIN_PROGRAM_DETAIL_TAB);
const { showToast } = useToastService();

watch(programId, () => {
  activeTab.value = DEFAULT_ADMIN_PROGRAM_DETAIL_TAB;
});

function openRepository() {
  const url = program.value?.repositoryUrl;
  if (!url || !import.meta.client) return;
  window.open(url, '_blank', 'noopener,noreferrer');
}

function onEdit() {
  showToast('Edit program is not wired yet.', ToastTypesEnum.info);
}

function toastSoon(action: string) {
  showToast(`${action} is not wired yet.`, ToastTypesEnum.info);
}

useHead({
  title: computed(() => program.value?.name ?? 'Admin program'),
});
</script>

<script lang="ts">
export default {
  name: 'AdminProgramDetailView',
};
</script>
