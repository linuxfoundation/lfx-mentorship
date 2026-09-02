<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <div>
    <programs-header
      v-model:search-term="searchTerm"
      v-model:active-status="activeStatus"
      v-model:skill="skill"
      v-model:sort-by="sortBy"
      :skill-options="skillOptions"
    />
    <div
      class="transition-all ease-linear"
      :class="{ 'pt-8': isScrolled }"
    >
      <programs-grid
        :programs="programs"
        :has-more="hasMore"
        :is-loading="isLoading"
        :is-loading-more="isFetchingNextPage"
        :load-failed="Boolean(programError) && programs.length === 0"
        @load-more="loadMore"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { refDebounced } from '@vueuse/core';
import ProgramsHeader from '../components/programs-header.vue';
import ProgramsGrid from '../components/programs-grid.vue';
import {
  ALL_SKILLS_OPTION,
  DEFAULT_PROGRAM_FILTER,
  DEFAULT_PROGRAM_SORT,
  PROGRAM_SEARCH_DEBOUNCE_MS,
} from '../config/programs-header.config';
import { SKILL_LIST } from '~/config/skills';
import { usePrograms } from '~/composables/programs/usePrograms';
import type { ProgramSortBy, ProgramStatusFilter } from '~/types/program.types';
import useScroll from '~/utils/scroll';
import { getFetchErrorMessage } from '~/utils/fetch-error';
import { ToastTypesEnum } from '~/components/uikit/toast/types/toast.types';
import useToastService from '~/components/uikit/toast/toast.service';

const searchTerm = ref('');
const debouncedSearchTerm = refDebounced(searchTerm, PROGRAM_SEARCH_DEBOUNCE_MS);
const activeStatus = ref<ProgramStatusFilter>(DEFAULT_PROGRAM_FILTER);
const skill = ref<string>(ALL_SKILLS_OPTION.value);
const sortBy = ref<ProgramSortBy>(DEFAULT_PROGRAM_SORT.value);

const { data, isPending, isFetchingNextPage, isFetchNextPageError, hasNextPage, fetchNextPage, error } = usePrograms({
  search: debouncedSearchTerm,
  status: activeStatus,
  skill,
  sortBy,
});

const programs = computed(() => data.value?.pages.flatMap((page) => page.data) ?? []);
const isLoading = computed(() => isPending.value && programs.value.length === 0);
const hasMore = computed(() => Boolean(hasNextPage.value) || isFetchNextPageError.value);
const skillOptions = SKILL_LIST;
const programError = computed(() => (error.value as Error | null) ?? null);
const { showToast } = useToastService();

watch(error, (err) => {
  if (!import.meta.client || !err || programs.value.length > 0) return;
  showToast(getFetchErrorMessage(err, 'Failed to load programs. Please try again.'), ToastTypesEnum.negative);
});

async function loadMore() {
  if ((!hasNextPage.value && !isFetchNextPageError.value) || isFetchingNextPage.value) return;

  try {
    const result = await fetchNextPage();
    if (result.error) {
      showToast(
        getFetchErrorMessage(result.error, 'Failed to load more programs. Please try again.'),
        ToastTypesEnum.negative,
      );
    }
  } catch (err) {
    showToast(getFetchErrorMessage(err, 'Failed to load more programs. Please try again.'), ToastTypesEnum.negative);
  }
}

const { scrollTop } = useScroll();
const isScrolled = computed(() => scrollTop.value > 10);
</script>

<script lang="ts">
export default {
  name: 'ProgramsView',
};
</script>
