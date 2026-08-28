<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section
    class="container pt-6 pb-10 flex flex-col sticky gap-8 top-[var(--lfx-header-height)] bg-white z-10"
    :class="{ 'border-b border-neutral-200 !pb-5': isScrolled }"
  >
    <Transition name="header-eyebrow">
      <div
        v-show="!isScrolled"
        class="flex flex-col gap-6"
      >
        <div class="flex items-center gap-2 text-primary-600">
          <lfx-icon
            name="user-graduate"
            type="light"
            :size="16"
          />
          <span class="text-lg font-medium leading-7 text-accent-800">Mentees</span>
        </div>
        <h1 class="font-secondary font-light md:text-4xl text-3xl leading-normal text-neutral-900">
          Mentees
          <p
            v-if="catalogSummary"
            class="text-sm text-neutral-600"
          >
            {{ catalogSummary }}
          </p>
        </h1>
      </div>
    </Transition>

    <div class="flex flex-col gap-4 lg:flex-row lg:items-center">
      <lfx-input
        :model-value="searchTerm"
        class="!rounded-full flex-1"
        placeholder="Search mentees by name, skill or project"
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

      <skill-filter-select
        :model-value="skill"
        :skill-options="skillOptions"
        @update:model-value="$emit('update:skill', $event)"
      />
    </div>

    <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
      <div class="hidden md:block">
        <lfx-tabs
          :model-value="activeStatus"
          :tabs="MENTEE_FILTER_TABS"
          tab-style="pill"
          @update:model-value="$emit('update:activeStatus', $event as MenteeStatusFilter)"
        />
      </div>
      <div class="md:hidden block">
        <lfx-dropdown-select
          :model-value="activeStatus"
          width="200px"
          placement="bottom-end"
          @update:model-value="$emit('update:activeStatus', $event as MenteeStatusFilter)"
        >
          <template #trigger="{ selectedOption }">
            <lfx-button
              :label="selectedOption?.label ?? 'All'"
              type="outline"
              button-style="pill"
              icon="arrow-up-arrow-down"
            />
          </template>
          <lfx-dropdown-item
            v-for="tab in MENTEE_FILTER_TABS"
            :key="tab.value"
            :value="tab.value"
            :label="tab.label"
          />
        </lfx-dropdown-select>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue';
import { MENTEE_FILTER_TABS, formatMenteesCatalogSummary } from '../config/mentees-header.config';
import LfxIcon from '~/components/uikit/icon/icon.vue';
import LfxInput from '~/components/uikit/input/input.vue';
import LfxTabs from '~/components/uikit/tabs/tabs.vue';
import LfxButton from '~/components/uikit/button/button.vue';
import LfxDropdownSelect from '~/components/uikit/dropdown/dropdown-select.vue';
import LfxDropdownItem from '~/components/uikit/dropdown/dropdown-item.vue';
import SkillFilterSelect from '~/components/shared/skill-filter-select.vue';
import useScroll from '~/utils/scroll';
import type { MenteeStatusFilter } from '~/types/mentee.types';

const props = defineProps<{
  searchTerm: string;
  activeStatus: MenteeStatusFilter;
  skill: string;
  skillOptions: string[];
  menteeCount?: number;
  projectCount?: number;
}>();

const catalogSummary = computed(() => {
  if (props.menteeCount == null || props.projectCount == null) {
    return '';
  }

  return formatMenteesCatalogSummary(props.menteeCount, props.projectCount);
});

defineEmits<{
  (e: 'update:searchTerm' | 'update:skill', value: string): void;
  (e: 'update:activeStatus', value: MenteeStatusFilter): void;
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
  name: 'MenteesHeader',
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
