<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <div class="min-h-screen flex flex-col">
    <mentorship-header />
    <main class="flex-grow flex items-center justify-center px-5 py-16">
      <div class="max-w-md text-center">
        <p class="text-sm font-semibold text-brand-700">{{ statusCode }}</p>
        <h1 class="mt-2 text-2xl font-semibold text-neutral-900">{{ title }}</h1>
        <p class="mt-2 text-neutral-600">{{ description }}</p>
        <button
          type="button"
          class="mt-6 inline-block text-brand-700 hover:underline"
          @click="handleHome"
        >
          Back to home
        </button>
      </div>
    </main>
    <mentorship-footer />
  </div>
</template>

<script setup lang="ts">
import MentorshipHeader from '~/components/shared/layout/header.vue';
import MentorshipFooter from '~/components/shared/layout/footer.vue';
import { AppRoute } from '~/config/routes';

const error = useError();
const statusCode = computed(() => Number(error.value?.statusCode) || 500);
const isNotFound = computed(() => statusCode.value === 404);
const title = computed(() => (isNotFound.value ? 'Page not found' : 'Something went wrong'));
const description = computed(() =>
  isNotFound.value
    ? 'This page is unavailable or is not published.'
    : 'An unexpected error occurred. Please try again.',
);

useHead({
  title,
  titleTemplate: '%s | LFX Mentorship',
  meta: [{ name: 'robots', content: 'noindex, nofollow' }],
});

function handleHome() {
  clearError({ redirect: AppRoute.Home });
}
</script>
