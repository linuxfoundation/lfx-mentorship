<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <div>
    <mentors-header
      v-model:search-term="searchTerm"
      v-model:skill="skill"
      :skill-options="skillOptions"
      :mentor-count="mentorCount"
      :program-count="programCount"
    />
    <div
      class="transition-all ease-linear"
      :class="{ 'pt-8': isScrolled }"
    >
      <mentors-grid
        :mentors="mentors"
        :has-more="hasMore"
        :is-loading="isLoading"
        :is-loading-more="isFetchingNextPage"
        :load-failed="Boolean(mentorError) && mentors.length === 0"
        @load-more="loadMore"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { refDebounced } from '@vueuse/core';
import MentorsHeader from '../components/mentors-header.vue';
import MentorsGrid from '../components/mentors-grid.vue';
import { ALL_SKILLS_OPTION, MENTOR_SEARCH_DEBOUNCE_MS } from '../config/mentors-header.config';
import { SKILL_LIST } from '~/config/skills';
import { useMentors } from '~/composables/mentors/useMentors';
import { useMentorsSummary } from '~/composables/mentors/useMentorsSummary';
import useScroll from '~/utils/scroll';
import { getFetchErrorMessage } from '~/utils/fetch-error';
import { ToastTypesEnum } from '~/components/uikit/toast/types/toast.types';
import useToastService from '~/components/uikit/toast/toast.service';

const searchTerm = ref('');
const debouncedSearchTerm = refDebounced(searchTerm, MENTOR_SEARCH_DEBOUNCE_MS);
const skill = ref<string>(ALL_SKILLS_OPTION.value);

const { data, isPending, isFetchingNextPage, isFetchNextPageError, hasNextPage, fetchNextPage, error } = useMentors({
  search: debouncedSearchTerm,
  skill,
});
const { data: summary } = useMentorsSummary();

const mentors = computed(() => data.value?.pages.flatMap((page) => page.data) ?? []);
const isLoading = computed(() => isPending.value && mentors.value.length === 0);
const hasMore = computed(() => Boolean(hasNextPage.value) || isFetchNextPageError.value);
const skillOptions = SKILL_LIST;
const mentorCount = computed(() => summary.value?.mentorCount);
const programCount = computed(() => summary.value?.programCount);
const mentorError = computed(() => (error.value as Error | null) ?? null);
const { showToast } = useToastService();

watch(error, (err) => {
  if (!import.meta.client || !err || mentors.value.length > 0) return;
  showToast(getFetchErrorMessage(err, 'Failed to load mentors. Please try again.'), ToastTypesEnum.negative);
});

async function loadMore() {
  if ((!hasNextPage.value && !isFetchNextPageError.value) || isFetchingNextPage.value) return;

  try {
    const result = await fetchNextPage();
    if (result.error) {
      showToast(
        getFetchErrorMessage(result.error, 'Failed to load more mentors. Please try again.'),
        ToastTypesEnum.negative,
      );
    }
  } catch (err) {
    showToast(getFetchErrorMessage(err, 'Failed to load more mentors. Please try again.'), ToastTypesEnum.negative);
  }
}

const { scrollTop } = useScroll();
const isScrolled = computed(() => scrollTop.value > 10);
</script>

<script lang="ts">
export default {
  name: 'MentorsView',
};
</script>
