// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { useQuery } from '@tanstack/vue-query';
import type { MaybeRef } from 'vue';
import { computed, toValue } from 'vue';
import type { MentorsListResponse } from '~/types/mentor.types';

export function useMentors(filters: { search: MaybeRef<string>; skill: MaybeRef<string> }) {
  return useQuery<MentorsListResponse>({
    queryKey: computed(() => ['mentors', toValue(filters.search), toValue(filters.skill)]),
    queryFn: () =>
      $fetch<MentorsListResponse>('/api/mentors', {
        query: {
          search: toValue(filters.search) || undefined,
          skill: toValue(filters.skill),
        },
      }),
  });
}
