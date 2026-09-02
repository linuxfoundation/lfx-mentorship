<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <div>
    <mentees-header
      v-model:search-term="searchTerm"
      v-model:active-status="activeStatus"
      v-model:skill="skill"
      :skill-options="skillOptions"
      :mentee-count="menteeCount"
      :program-count="programCount"
    />
    <div
      class="transition-all ease-linear"
      :class="{ 'pt-8': isScrolled }"
    >
      <mentees-grid
        :mentees="mentees"
        :has-more="hasMore"
        :is-loading="isLoading"
        :is-loading-more="isFetchingNextPage"
        :load-failed="Boolean(menteeError) && mentees.length === 0"
        @load-more="loadMore"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { refDebounced } from '@vueuse/core';
import MenteesHeader from '../components/mentees-header.vue';
import MenteesGrid from '../components/mentees-grid.vue';
import { ALL_SKILLS_OPTION, DEFAULT_MENTEE_FILTER, MENTEE_SEARCH_DEBOUNCE_MS } from '../config/mentees-header.config';
import { SKILL_LIST } from '~/config/skills';
import { useMentees } from '~/composables/mentees/useMentees';
import { useMenteesSummary } from '~/composables/mentees/useMenteesSummary';
import type { MenteeStatusFilter } from '~/types/mentee.types';
import useScroll from '~/utils/scroll';
import { getFetchErrorMessage } from '~/utils/fetch-error';
import { ToastTypesEnum } from '~/components/uikit/toast/types/toast.types';
import useToastService from '~/components/uikit/toast/toast.service';

const searchTerm = ref('');
const debouncedSearchTerm = refDebounced(searchTerm, MENTEE_SEARCH_DEBOUNCE_MS);
const activeStatus = ref<MenteeStatusFilter>(DEFAULT_MENTEE_FILTER);
const skill = ref<string>(ALL_SKILLS_OPTION.value);

const { data, isPending, isFetchingNextPage, isFetchNextPageError, hasNextPage, fetchNextPage, error } = useMentees({
  search: debouncedSearchTerm,
  status: activeStatus,
  skill,
});
const { data: summary } = useMenteesSummary();

const mentees = computed(() => data.value?.pages.flatMap((page) => page.data) ?? []);
const isLoading = computed(() => isPending.value && mentees.value.length === 0);
const hasMore = computed(() => Boolean(hasNextPage.value) || isFetchNextPageError.value);
const skillOptions = SKILL_LIST;
const menteeCount = computed(() => summary.value?.menteeCount);
const programCount = computed(() => summary.value?.programCount);
const menteeError = computed(() => (error.value as Error | null) ?? null);
const { showToast } = useToastService();

watch(error, (err) => {
  if (!import.meta.client || !err || mentees.value.length > 0) return;
  showToast(getFetchErrorMessage(err, 'Failed to load mentees. Please try again.'), ToastTypesEnum.negative);
});

async function loadMore() {
  if ((!hasNextPage.value && !isFetchNextPageError.value) || isFetchingNextPage.value) return;

  try {
    const result = await fetchNextPage();
    if (result.error) {
      showToast(
        getFetchErrorMessage(result.error, 'Failed to load more mentees. Please try again.'),
        ToastTypesEnum.negative,
      );
    }
  } catch (err) {
    showToast(getFetchErrorMessage(err, 'Failed to load more mentees. Please try again.'), ToastTypesEnum.negative);
  }
}

const { scrollTop } = useScroll();
const isScrolled = computed(() => scrollTop.value > 10);
</script>

<script lang="ts">
export default {
  name: 'MenteesView',
};
</script>
