<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <program-detail-view :program-id="programId" />
</template>

<script setup lang="ts">
import { computed } from 'vue';
import ProgramDetailView from '~/components/modules/programs/view/program-detail.vue';
import { FunnelEvent, trackFunnelEvent } from '~/composables/useFunnelAnalytics';
import { programPath } from '~/config/routes';
import type { Program } from '~/types/program.types';
import { courseJsonLd, truncateMetaDescription } from '~/utils/seo';

const route = useRoute();
const programId = computed(() => String(route.params.id ?? ''));

const { data: program, error } = await useAsyncData<Program>(
  `program-seo-${programId.value}`,
  () => $fetch<Program>(`/api/programs/${programId.value}`),
  { lazy: false },
);

if (error.value) {
  throw createError(error.value);
}

const title = computed(() => program.value?.name ?? 'Program');
const description = computed(
  () => program.value?.description ?? 'Explore this mentorship program on LFX Mentorship.',
);
const canonicalPath = computed(() => programPath(program.value?.slug || program.value?.id || programId.value));

const { canonical, image } = usePublicSeo({
  title,
  description,
  path: canonicalPath,
  image: computed(() => program.value?.logoUrl),
});

useJsonLd(
  computed(() => {
    if (!program.value || !canonical.value) return null;
    return courseJsonLd({
      name: program.value.name,
      description: truncateMetaDescription(
        program.value.description,
        'Explore this mentorship program on LFX Mentorship.',
      ),
      url: canonical.value,
      image: image.value,
    });
  }),
  'json-ld-course',
);

onMounted(() => {
  if (!program.value) return;
  trackFunnelEvent(
    FunnelEvent.ProgramDetailViewed,
    {
      program_id: program.value.id,
      program_slug: program.value.slug,
    },
    `program_detail_viewed:${program.value.id}`,
  );
});
</script>
