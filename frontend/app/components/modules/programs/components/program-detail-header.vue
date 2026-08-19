<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <section class="border border-neutral-200 rounded-2xl bg-white p-6 md:p-8">
    <detail-back-link
      :to="AppRoute.FindProgram"
      label="Programs"
      class="mb-6"
    />
    <div
      class="flex flex-col gap-8 lg:flex-row lg:items-start lg:justify-between"
    >
      <div
        class="flex min-w-0 flex-1 flex-col gap-6 md:flex-row md:items-start"
      >
        <lfx-avatar
          :src="program.logoUrl"
          type="organization"
          size="xlarge"
          class="!rounded-xl shrink-0"
        />

        <div class="flex min-w-0 flex-1 flex-col gap-4">
          <div class="flex flex-wrap items-center gap-3">
            <span class="text-sm text-neutral-500">{{
              program.foundation.name
            }}</span>
            <lfx-tag
              :variation="statusConfig.variation"
              size="small"
              type="solid"
            >
              {{ statusConfig.label }}
            </lfx-tag>
          </div>

          <h1
            class="font-secondary text-2xl md:text-3xl font-light text-neutral-900 leading-tight break-words"
          >
            {{ program.name }}
          </h1>

          <div v-if="program.skills.length" class="flex flex-wrap gap-2">
            <lfx-chip
              v-for="skill in program.skills"
              :key="skill"
              type="bordered"
              size="xsmall"
            >
              {{ skill }}
            </lfx-chip>
          </div>

          <div class="flex flex-wrap items-center gap-3 pt-1">
            <lfx-button
              v-if="program.status === 'acceptance'"
              label="Apply to This Program"
              icon="paper-plane"
              type="primary"
              button-style="pill"
              @click="$emit('apply')"
            />
            <lfx-button
              v-if="program.repositoryUrl"
              label="Repository"
              icon="code-branch"
              type="outline"
              button-style="pill"
              @click="$emit('open-repository')"
            />
            <lfx-button
              v-if="program.crowdfundingInitiativeId"
              label="Donate"
              icon="heart"
              type="transparent"
              button-style="pill"
              @click="$emit('donate')"
            />
          </div>
        </div>
      </div>

      <aside
        class="w-full shrink-0 rounded-xl bg-neutral-50 border border-neutral-100 p-5 lg:w-72"
      >
        <p
          class="text-xs font-semibold uppercase tracking-wide text-neutral-500 mb-4"
        >
          Term details
        </p>
        <dl class="flex flex-col gap-4">
          <div class="flex flex-col gap-1">
            <dt class="text-xs text-neutral-500">Term</dt>
            <dd class="text-sm font-medium text-neutral-900">
              {{ termLabel }}
            </dd>
          </div>
          <div v-if="applicationsCloseLabel" class="flex flex-col gap-1">
            <dt class="text-xs text-neutral-500">Applications close</dt>
            <dd class="text-sm font-medium text-neutral-900">
              {{ applicationsCloseLabel }}
            </dd>
          </div>
          <div class="flex flex-col gap-1">
            <dt class="text-xs text-neutral-500">Stipend</dt>
            <dd class="text-sm font-medium text-neutral-900">
              Amount determined by mentee location
            </dd>
          </div>
        </dl>
      </aside>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { PROGRAM_STATUS_CONFIG } from "../config/program-card.config";
import { AppRoute } from "~/config/routes";
import DetailBackLink from "~/components/shared/detail-back-link.vue";
import type { Program } from "~/types/program.types";
import LfxAvatar from "~/components/uikit/avatar/avatar.vue";
import LfxButton from "~/components/uikit/button/button.vue";
import LfxChip from "~/components/uikit/chip/chip.vue";
import LfxTag from "~/components/uikit/tag/tag.vue";
import { formatProgramDate } from "~/utils/date";

const props = defineProps<{ program: Program }>();

defineEmits<{
  (e: "apply"): void;
  (e: "open-repository"): void;
  (e: "donate"): void;
}>();

const statusConfig = computed(
  () => PROGRAM_STATUS_CONFIG[props.program.status],
);

const termLabel = computed(() => {
  const { name, dateRangeLabel } = props.program.activeTerm;
  return dateRangeLabel ? `${name} · ${dateRangeLabel}` : name;
});

const applicationsCloseLabel = computed(() => {
  const iso = props.program.activeTerm.applicationsCloseAt;
  return iso ? formatProgramDate(iso) : null;
});
</script>

<script lang="ts">
export default {
  name: "ProgramDetailHeader",
};
</script>
