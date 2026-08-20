<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
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
      :project-count="projectCount"
    />
    <div
      class="transition-all ease-linear"
      :class="{ 'pt-8': isScrolled }"
    >
      <mentees-grid
        :mentees="mentees"
        :visible-count="visibleCount"
        :is-loading="isLoading"
        :error="listError"
        @load-more="loadMore"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import MenteesHeader from '../components/mentees-header.vue';
import MenteesGrid from '../components/mentees-grid.vue';
import {
  ALL_SKILLS_OPTION,
  DEFAULT_MENTEE_FILTER,
  DIRECTORY_PAGE_SIZE,
} from '../config/mentees-header.config';
import { SKILL_LIST } from '~/config/skills';
import { useMentees } from '~/composables/mentees/useMentees';
import type { MenteeStatusFilter } from '~/types/mentee.types';
import useScroll from '~/utils/scroll';

const searchTerm = ref('');
const activeStatus = ref<MenteeStatusFilter>(DEFAULT_MENTEE_FILTER);
const skill = ref<string>(ALL_SKILLS_OPTION.value);
const visibleCount = ref(DIRECTORY_PAGE_SIZE);

const { data, isLoading, error } = useMentees({
  search: searchTerm,
  status: activeStatus,
  skill,
});

const mentees = computed(() => data.value?.data ?? []);
const skillOptions = SKILL_LIST;
const menteeCount = computed(() => data.value?.menteeCount);
const projectCount = computed(() => data.value?.projectCount);
const listError = computed(() => (error.value as Error | null) ?? null);

watch([searchTerm, activeStatus, skill], () => {
  visibleCount.value = DIRECTORY_PAGE_SIZE;
});

function loadMore() {
  visibleCount.value += DIRECTORY_PAGE_SIZE;
}

const { scrollTop } = useScroll();
const isScrolled = computed(() => scrollTop.value > 10);
</script>

<script lang="ts">
export default {
  name: 'MenteesView',
};
</script>
