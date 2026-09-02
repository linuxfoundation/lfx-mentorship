// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { useQuery } from '@tanstack/vue-query';
import type { LandingSummaryResponse } from '~/types/landing.types';

/**
 * Fetches the aggregated marketing counts shown on the landing page.
 * The response is treated as static for the session — the underlying
 * numbers move slowly and the endpoint is cheap.
 */
export function useLandingSummary() {
  return useQuery<LandingSummaryResponse>({
    queryKey: ['landing-summary'],
    queryFn: () => $fetch<LandingSummaryResponse>('/api/summary'),
    staleTime: Infinity,
  });
}
