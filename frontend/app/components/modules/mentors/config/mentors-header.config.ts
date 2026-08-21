// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

export const ALL_SKILLS_OPTION = { value: 'all', label: 'All skills' } as const;

export const DIRECTORY_PAGE_SIZE = 6;

/** Builds the header catalog summary, e.g. "8 mentors across 7 projects". */
export function formatMentorsCatalogSummary(mentorCount: number, projectCount: number): string {
  const mentorsLabel = mentorCount === 1 ? 'mentor' : 'mentors';
  const projectsLabel = projectCount === 1 ? 'project' : 'projects';

  return `${mentorCount.toLocaleString()} ${mentorsLabel} across ${projectCount.toLocaleString()} ${projectsLabel}`;
}
