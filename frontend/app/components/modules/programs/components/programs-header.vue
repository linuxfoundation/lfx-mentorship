<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section
    class="container pt-6 pb-10 flex flex-col sticky gap-8 top-8 bg-white z-10"
    :class="{ 'border-b border-neutral-200 !pb-5': isScrolled }"
  >
    <Transition name="header-eyebrow">
      <div
        v-show="!isScrolled"
        class="flex flex-col gap-6"
      >
        <div class="flex items-center gap-2 text-primary-600">
          <lfx-icon
            name="chalkboard-user"
            type="light"
            :size="16"
          />
          <span class="text-lg font-medium leading-7 text-accent-800">Programs</span>
        </div>
        <h1 class="font-secondary font-light md:text-5xl text-4xl leading-normal text-neutral-900">
          Find a mentorship program
          <p
            v-if="catalogSummary"
            class="text-sm text-neutral-600"
          >
            {{ catalogSummary }}
          </p>
        </h1>
      </div>
    </Transition>

    <lfx-input
      :model-value="searchTerm"
      class="!rounded-full"
      placeholder="Search programs..."
      @update:model-value="$emit('update:searchTerm', String($event))"
    >
      <template #prefix>
        <lfx-icon
          name="magnifying-glass"
          type="light"
          :size="16"
          class="text-neutral-400"
        />
      </template>
    </lfx-input>

    <div class="flex flex-wrap items-center gap-3">
      <div class="hidden md:block shrink-0">
        <lfx-tabs
          :model-value="activeStatus"
          :tabs="PROGRAM_FILTER_TABS"
          tab-style="pill"
          @update:model-value="$emit('update:activeStatus', $event as ProgramStatusFilter)"
        />
      </div>
      <div class="md:hidden block">
        <lfx-dropdown-select
          :model-value="activeStatus"
          width="200px"
          placement="bottom-end"
          @update:model-value="$emit('update:activeStatus', $event as ProgramStatusFilter)"
        >
          <template #trigger="{ selectedOption }">
            <lfx-button
              :label="selectedOption?.label ?? 'All programs'"
              type="outline"
              button-style="pill"
              icon="arrow-up-arrow-down"
            />
          </template>
          <lfx-dropdown-item
            v-for="tab in PROGRAM_FILTER_TABS"
            :key="tab.value"
            :value="tab.value"
            :label="tab.value === 'all' ? 'All programs' : tab.label"
          />
        </lfx-dropdown-select>
      </div>

      <div class="flex flex-wrap items-center gap-3 xl:ml-auto xl:flex-nowrap">

        <skill-filter-select
          :model-value="skill"
          :skill-options="skillOptions"
          @update:model-value="$emit('update:skill', $event)"
        />

        <lfx-dropdown-select
          :model-value="sortBy"
          width="220px"
          placement="bottom-end"
          @update:model-value="$emit('update:sortBy', $event as ProgramSortBy)"
        >
          <template #trigger="{ selectedOption }">
            <lfx-button
              :label="selectedOption?.label ?? DEFAULT_PROGRAM_SORT.label"
              type="outline"
              button-style="pill"
              icon="arrow-down-wide-short"
            />
          </template>
          <lfx-dropdown-item
            v-for="option in PROGRAM_SORT_OPTIONS"
            :key="option.value"
            :value="option.value"
            :label="option.label"
          />
        </lfx-dropdown-select>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue';
import {
  DEFAULT_PROGRAM_SORT,
  PROGRAM_FILTER_TABS,
  PROGRAM_SORT_OPTIONS,
  formatProgramsCatalogSummary,
} from '../config/programs-header.config';
import LfxIcon from '~/components/uikit/icon/icon.vue';
import LfxInput from '~/components/uikit/input/input.vue';
import LfxTabs from '~/components/uikit/tabs/tabs.vue';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxDropdownSelect from '~/components/uikit/dropdown/dropdown-select.vue';
import LfxDropdownItem from '~/components/uikit/dropdown/dropdown-item.vue';
import SkillFilterSelect from '~/components/shared/skill-filter-select.vue';
import useScroll from '~/utils/scroll';
import type { ProgramSortBy, ProgramStatusFilter } from '~/types/program.types';

const props = defineProps<{
  searchTerm: string;
  activeStatus: ProgramStatusFilter;
  skill: string;
  sortBy: ProgramSortBy;
  skillOptions: string[];
  programCount?: number;
  foundationCount?: number;
}>();

const catalogSummary = computed(() => {
  if (props.programCount == null || props.foundationCount == null) {
    return '';
  }

  return formatProgramsCatalogSummary(props.programCount, props.foundationCount);
});

const emit = defineEmits<{
  (e: 'update:searchTerm', value: string): void;
  (e: 'update:activeStatus', value: ProgramStatusFilter): void;
  (e: 'update:skill', value: string): void;
  (e: 'update:sortBy', value: ProgramSortBy): void;
}>();

let prevOverflowAnchor = '';
onMounted(() => {
  prevOverflowAnchor = document.documentElement.style.overflowAnchor;
  document.documentElement.style.overflowAnchor = 'none';
});
onUnmounted(() => {
  document.documentElement.style.overflowAnchor = prevOverflowAnchor;
});

const { scrollTop } = useScroll();
const isScrolled = computed(() => scrollTop.value > 10);
</script>

<script lang="ts">
export default {
  name: 'ProgramsHeader',
};
</script>

<style scoped>
.header-eyebrow-enter-active,
.header-eyebrow-leave-active {
  transition:
    opacity 0.2s ease,
    transform 0.2s ease,
    max-height 0.25s ease;
  max-height: 200px;
  overflow: hidden;
}

.header-eyebrow-enter-from,
.header-eyebrow-leave-to {
  opacity: 0;
  transform: translateY(-8px);
  max-height: 0;
}

</style>
