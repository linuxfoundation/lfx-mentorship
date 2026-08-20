// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { useQuery } from '@tanstack/vue-query';
import type { MaybeRef } from 'vue';
import { computed, toValue } from 'vue';
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
  const query = useQuery<ProgramsListResponse>({
    queryKey: computed(() => [
      'programs',
      toValue(filters.search),
      toValue(filters.status),
      toValue(filters.skill),
      toValue(filters.sortBy),
    ]),
    queryFn: () =>
      $fetch<ProgramsListResponse>('/api/programs', {
        query: {
          search: toValue(filters.search) || undefined,
          status: toValue(filters.status),
          skill: toValue(filters.skill),
          sortBy: toValue(filters.sortBy),
        },
      }),
  });

  return query;
}
