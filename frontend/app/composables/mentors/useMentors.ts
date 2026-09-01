// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { useInfiniteQuery } from '@tanstack/vue-query';
import type { MaybeRef } from 'vue';
import { computed, toValue } from 'vue';
import { MENTOR_PAGE_SIZE } from '~/components/modules/mentors/config/mentors-header.config';
import type { MentorsListResponse } from '~/types/mentor.types';

export function useMentors(filters: { search: MaybeRef<string>; skill: MaybeRef<string> }) {
  return useInfiniteQuery({
    queryKey: computed(() => ['mentors', toValue(filters.search), toValue(filters.skill)]),
    queryFn: ({ pageParam }) =>
      $fetch<MentorsListResponse>('/api/mentors', {
        query: {
          search: toValue(filters.search) || undefined,
          skill: toValue(filters.skill),
          limit: MENTOR_PAGE_SIZE,
          offset: pageParam,
        },
      }),
    initialPageParam: 0,
    getNextPageParam: (lastPage, allPages) => {
      const loaded = allPages.reduce((sum, page) => sum + page.data.length, 0);
      if (loaded >= lastPage.total) return undefined;
      return loaded;
    },
  });
}
