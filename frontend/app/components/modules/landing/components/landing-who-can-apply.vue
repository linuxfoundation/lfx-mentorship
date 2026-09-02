<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="rounded-2xl border border-neutral-200 bg-white p-6 md:p-8 h-full flex flex-col">
    <h2 class="font-secondary text-xl md:text-2xl font-normal text-neutral-900 mb-6">Who Can Apply</h2>

    <ul class="flex flex-col gap-4 flex-1">
      <li
        v-for="item in LANDING_ELIGIBILITY_ITEMS"
        :key="item.id"
        class="flex items-start gap-3"
      >
        <lfx-icon
          name="check"
          type="solid"
          :size="12"
          class="text-positive-600 shrink-0 mt-0.5"
        />
        <p class="text-sm text-neutral-700 leading-5">
          <template v-if="item.parts?.length">
            <template
              v-for="(part, i) in item.parts"
              :key="i"
            >
              <strong
                v-if="part.bold"
                class="font-semibold text-neutral-900"
              >
                {{ part.text }}
              </strong>
              <template v-else>{{ part.text }}</template>
            </template>
          </template>
          <template v-else>{{ item.text }}</template>
        </p>
      </li>
    </ul>

    <p class="mt-6 pt-4 border-t border-neutral-100 text-xs text-neutral-500 leading-5">
      {{ LANDING_ELIGIBILITY_FOOTER }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { LANDING_ELIGIBILITY_FOOTER, LANDING_ELIGIBILITY_ITEMS } from '../config/landing.config';
import LfxIcon from '~/components/uikit/icon/icon.vue';
</script>

<script lang="ts">
export default {
  name: 'LandingWhoCanApply',
};
</script>
