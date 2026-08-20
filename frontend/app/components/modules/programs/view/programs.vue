<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div>
    <programs-header
      v-model:search-term="searchTerm"
      v-model:active-status="activeStatus"
      v-model:term-name="termName"
      v-model:term-from="termFrom"
      v-model:term-to="termTo"
      v-model:skill="skill"
      v-model:sort-by="sortBy"
      :skill-options="skillOptions"
      :program-count="programCount"
      :foundation-count="foundationCount"
    />
    <div
      class="transition-all ease-linear"
      :class="{ 'pt-8': isScrolled }"
    >
      <programs-grid
        :programs="programs"
        :visible-count="visibleCount"
        :is-loading="isLoading"
        :error="programError"
        @load-more="loadMore"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import ProgramsHeader from '../components/programs-header.vue';
import ProgramsGrid from '../components/programs-grid.vue';
import {
  ALL_SKILLS_OPTION,
  DEFAULT_PROGRAM_FILTER,
  DEFAULT_PROGRAM_SORT,
  PROGRAM_PAGE_SIZE,
} from '../config/programs-header.config';
import { SKILL_LIST } from '~/config/skills';
import { usePrograms } from '~/composables/programs/usePrograms';
import type { ProgramSortBy, ProgramStatusFilter } from '~/types/program.types';
import useScroll from '~/utils/scroll';

const searchTerm = ref('');
const activeStatus = ref<ProgramStatusFilter>(DEFAULT_PROGRAM_FILTER);
const termName = ref('');
const termFrom = ref('');
const termTo = ref('');
const skill = ref<string>(ALL_SKILLS_OPTION.value);
const sortBy = ref<ProgramSortBy>(DEFAULT_PROGRAM_SORT.value);
const visibleCount = ref(PROGRAM_PAGE_SIZE);

const { data, isLoading, error } = usePrograms({
  search: searchTerm,
  status: activeStatus,
  termName,
  termFrom,
  termTo,
  skill,
  sortBy,
});

const programs = computed(() => data.value?.data ?? []);
const skillOptions = SKILL_LIST;
const programCount = computed(() => data.value?.programCount);
const foundationCount = computed(() => data.value?.foundationCount);
const programError = computed(() => (error.value as Error | null) ?? null);

watch([searchTerm, activeStatus, termName, termFrom, termTo, skill, sortBy], () => {
  visibleCount.value = PROGRAM_PAGE_SIZE;
});

function loadMore() {
  visibleCount.value += PROGRAM_PAGE_SIZE;
}

const { scrollTop } = useScroll();
const isScrolled = computed(() => scrollTop.value > 10);
</script>

<script lang="ts">
export default {
  name: 'ProgramsView',
};
</script>
