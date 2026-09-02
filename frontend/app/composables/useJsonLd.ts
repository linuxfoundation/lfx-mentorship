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
          // Escape `<` so program-controlled text cannot break out of the script tag.
          innerHTML: JSON.stringify(value).replace(/</g, '\\u003c'),
        },
      ];
    }),
  });
}
