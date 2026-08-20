<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div>
    <mentors-header
      v-model:search-term="searchTerm"
      v-model:skill="skill"
      :skill-options="skillOptions"
      :mentor-count="mentorCount"
      :project-count="projectCount"
    />
    <div
      class="transition-all ease-linear"
      :class="{ 'pt-8': isScrolled }"
    >
      <mentors-grid
        :mentors="mentors"
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
import MentorsHeader from '../components/mentors-header.vue';
import MentorsGrid from '../components/mentors-grid.vue';
import { ALL_SKILLS_OPTION, DIRECTORY_PAGE_SIZE } from '../config/mentors-header.config';
import { SKILL_LIST } from '~/config/skills';
import { useMentors } from '~/composables/mentors/useMentors';
import useScroll from '~/utils/scroll';

const searchTerm = ref('');
const skill = ref<string>(ALL_SKILLS_OPTION.value);
const visibleCount = ref(DIRECTORY_PAGE_SIZE);

const { data, isLoading, error } = useMentors({
  search: searchTerm,
  skill,
});

const mentors = computed(() => data.value?.data ?? []);
const skillOptions = SKILL_LIST;
const mentorCount = computed(() => data.value?.mentorCount);
const projectCount = computed(() => data.value?.projectCount);
const listError = computed(() => (error.value as Error | null) ?? null);

watch([searchTerm, skill], () => {
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
  name: 'MentorsView',
};
</script>
