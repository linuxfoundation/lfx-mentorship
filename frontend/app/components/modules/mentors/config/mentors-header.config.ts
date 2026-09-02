// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

export const ALL_SKILLS_OPTION = { value: 'all', label: 'All skills' } as const;

export const MENTOR_PAGE_SIZE = 6;
export const MENTOR_SEARCH_DEBOUNCE_MS = 300;

/** Builds the header catalog summary, e.g. "8 mentors across 7 programs". */
export function formatMentorsSummary(mentorCount: number, programCount: number): string {
  const mentorsLabel = mentorCount === 1 ? 'mentor' : 'mentors';
  const programsLabel = programCount === 1 ? 'program' : 'programs';

  return `${mentorCount.toLocaleString()} ${mentorsLabel} across ${programCount.toLocaleString()} ${programsLabel}`;
}
