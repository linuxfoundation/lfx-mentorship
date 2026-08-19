// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { useQuery } from '@tanstack/vue-query';
import type { MaybeRef } from 'vue';
import { computed, toValue } from 'vue';
import type { MenteeStatusFilter, MenteesListResponse } from '~/types/mentee.types';

export function useMentees(filters: {
  search: MaybeRef<string>;
  status: MaybeRef<MenteeStatusFilter>;
  skill: MaybeRef<string>;
}) {
  return useQuery<MenteesListResponse>({
    queryKey: computed(() => [
      'mentees',
      toValue(filters.search),
      toValue(filters.status),
      toValue(filters.skill),
    ]),
    queryFn: () =>
      $fetch<MenteesListResponse>('/api/mentees', {
        query: {
          search: toValue(filters.search) || undefined,
          status: toValue(filters.status),
          skill: toValue(filters.skill),
        },
      }),
  });
}
