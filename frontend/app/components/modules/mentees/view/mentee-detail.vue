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
      <span>Loading mentee…</span>
    </div>

    <div
      v-else-if="error || !mentee"
      class="rounded-2xl border border-neutral-200 bg-white p-10 text-center text-negative-600"
    >
      Mentee not found.
    </div>

    <template v-else>
      <mentee-detail-header :mentee="mentee" />

      <section class="space-y-4">
        <h2 class="font-secondary text-xl md:text-2xl font-normal text-neutral-900">Programs</h2>
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <profile-program-card
            v-for="program in mentee.programs"
            :key="program.id"
            :program="program"
          />
        </div>
        <p
          v-if="!mentee.programs.length"
          class="text-sm text-neutral-500"
        >
          No programs yet.
        </p>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue';
import MenteeDetailHeader from '../components/mentee-detail-header.vue';
import ProfileProgramCard from '~/components/shared/directory/profile-program-card.vue';
import { useMentee } from '~/composables/mentees/useMentee';
import LfxSpinner from '~/components/uikit/spinner/spinner.vue';
import { ToastTypesEnum } from '~/components/uikit/toast/types/toast.types';
import useToastService from '~/components/uikit/toast/toast.service';
import { getFetchErrorMessage } from '~/utils/fetch-error';

const props = defineProps<{ menteeId: string }>();

const menteeId = computed(() => props.menteeId);
const { data: mentee, isLoading, error } = useMentee(menteeId);
const { showToast } = useToastService();

watch(error, (err) => {
  if (!import.meta.client || !err) return;
  const statusCode =
    typeof err === 'object' && err !== null && 'statusCode' in err
      ? Number((err as { statusCode?: number }).statusCode)
      : 0;
  const fallback = statusCode === 404 ? 'Mentee not found.' : 'Failed to load mentee. Please try again.';
  showToast(getFetchErrorMessage(err, fallback), ToastTypesEnum.negative);
});

useHead({
  title: computed(() => mentee.value?.name ?? 'Mentee'),
});
</script>

<script lang="ts">
export default {
  name: 'MenteeDetailView',
};
</script>
