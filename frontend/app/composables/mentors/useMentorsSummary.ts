// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { useQuery } from '@tanstack/vue-query';
import type { MentorsSummaryResponse } from '~/types/mentor.types';

export function useMentorsSummary() {
  return useQuery<MentorsSummaryResponse>({
    queryKey: ['mentors-summary'],
    queryFn: () => $fetch<MentorsSummaryResponse>('/api/mentors/summary'),
    staleTime: Infinity,
  });
}
