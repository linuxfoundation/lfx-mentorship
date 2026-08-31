// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { useInfiniteQuery } from '@tanstack/vue-query';
import type { MaybeRef } from 'vue';
import { computed, toValue } from 'vue';
import { MENTEE_PAGE_SIZE } from '~/components/modules/mentees/config/mentees-header.config';
import type { MenteeStatusFilter, MenteesListResponse } from '~/types/mentee.types';

export function useMentees(filters: {
  search: MaybeRef<string>;
  status: MaybeRef<MenteeStatusFilter>;
  skill: MaybeRef<string>;
}) {
  return useInfiniteQuery({
    queryKey: computed(() => [
      'mentees',
      toValue(filters.search),
      toValue(filters.status),
      toValue(filters.skill),
    ]),
    queryFn: ({ pageParam }) =>
      $fetch<MenteesListResponse>('/api/mentees', {
        query: {
          search: toValue(filters.search) || undefined,
          status: toValue(filters.status),
          skill: toValue(filters.skill),
          limit: MENTEE_PAGE_SIZE,
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
