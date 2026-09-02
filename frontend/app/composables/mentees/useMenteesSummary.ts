// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { useQuery } from '@tanstack/vue-query';
import type { MenteesSummaryResponse } from '~/types/mentee.types';

export function useMenteesSummary() {
  return useQuery<MenteesSummaryResponse>({
    queryKey: ['mentees-summary'],
    queryFn: () => $fetch<MenteesSummaryResponse>('/api/mentees/summary'),
    staleTime: Infinity,
  });
}
