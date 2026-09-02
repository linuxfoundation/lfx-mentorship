// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { computed, toValue, type MaybeRefOrGetter } from 'vue';

export function useJsonLd(
  schema: MaybeRefOrGetter<Record<string, unknown> | null | undefined>,
  key = 'json-ld',
) {
  useHead({
    script: computed(() => {
      const value = toValue(schema);
      if (!value) return [];
      return [
        {
          key,
          type: 'application/ld+json',
          innerHTML: JSON.stringify(value),
        },
      ];
    }),
  });
}
