// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { useInfiniteQuery } from '@tanstack/vue-query';
import type { MaybeRef } from 'vue';
import { computed, toValue } from 'vue';
import { PROGRAM_PAGE_SIZE } from '~/components/modules/programs/config/programs-header.config';
import type {
  ProgramSortBy,
  ProgramStatusFilter,
  ProgramsListResponse,
} from '~/types/program.types';

export function usePrograms(filters: {
  search: MaybeRef<string>;
  status: MaybeRef<ProgramStatusFilter>;
  skill: MaybeRef<string>;
  sortBy: MaybeRef<ProgramSortBy>;
}) {
  return useInfiniteQuery({
    queryKey: computed(() => [
      'programs',
      toValue(filters.search),
      toValue(filters.status),
      toValue(filters.skill),
      toValue(filters.sortBy),
    ]),
    queryFn: ({ pageParam }) =>
      $fetch<ProgramsListResponse>('/api/programs', {
        query: {
          search: toValue(filters.search) || undefined,
          status: toValue(filters.status),
          skill: toValue(filters.skill),
          sortBy: toValue(filters.sortBy),
          limit: PROGRAM_PAGE_SIZE,
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
