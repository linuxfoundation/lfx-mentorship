// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { LandingSummaryResponse } from '../../app/types/landing.types';

// Raw shape returned by `GET /v1/summary` on the backend.
interface PlatformSummaryApi {
  program_count: number;
  accepting_program_count: number;
  mentor_count: number;
  graduated_mentee_count: number;
}

function fetchErrorStatus(error: unknown): number {
  if (typeof error === 'object' && error !== null && 'statusCode' in error) {
    const statusCode = Number((error as { statusCode?: number }).statusCode);
    if (Number.isFinite(statusCode) && statusCode > 0) return statusCode;
  }
  return 502;
}

/**
 * Fetch the aggregated landing counts from the mentorship API.
 * The backend hides errors as generic 5xx; we surface a stable message.
 */
export async function fetchPlatformSummary(): Promise<LandingSummaryResponse> {
  const config = useRuntimeConfig();
  try {
    const summary = await $fetch<PlatformSummaryApi>(`${config.apiBaseUrl}/v1/summary`);
    return {
      programCount: summary.program_count,
      acceptingProgramCount: summary.accepting_program_count,
      mentorCount: summary.mentor_count,
      graduatedMenteeCount: summary.graduated_mentee_count,
      foundationCount: 42, // TODO: Implement this
      stipendsPaid: 6100000, // TODO: Implement this
    };
  } catch (error) {
    throw createError({
      statusCode: fetchErrorStatus(error),
      message: 'Failed to load landing summary',
    });
  }
}
