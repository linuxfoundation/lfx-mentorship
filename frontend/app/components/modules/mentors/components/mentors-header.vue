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
            name="user-tie"
            type="light"
            :size="16"
          />
          <span class="text-lg font-medium leading-7 text-accent-800">Mentors</span>
        </div>
        <h1 class="font-secondary font-light md:text-5xl text-4xl leading-normal text-neutral-900">
          Mentors
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
        placeholder="Search mentors by name, skill or project"
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
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue';
import { formatMentorsCatalogSummary } from '../config/mentors-header.config';
import LfxIcon from '~/components/uikit/icon/icon.vue';
import LfxInput from '~/components/uikit/input/input.vue';
import SkillFilterSelect from '~/components/shared/skill-filter-select.vue';
import useScroll from '~/utils/scroll';

const props = defineProps<{
  searchTerm: string;
  skill: string;
  skillOptions: string[];
  mentorCount?: number;
  projectCount?: number;
}>();

const catalogSummary = computed(() => {
  if (props.mentorCount == null || props.projectCount == null) {
    return '';
  }

  return formatMentorsCatalogSummary(props.mentorCount, props.projectCount);
});

defineEmits<{
  (e: 'update:searchTerm' | 'update:skill', value: string): void;
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
  name: 'MentorsHeader',
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
