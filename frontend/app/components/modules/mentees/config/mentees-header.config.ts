// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { Tab } from '~/components/uikit/tabs/types/tab.types';
import type { MenteeStatusFilter } from '~/types/mentee.types';

export const MENTEE_FILTER_TABS: Tab[] = [
  { value: 'all', label: 'All', icon: 'grid-round-2' },
  { value: 'active', label: 'Active', icon: 'play' },
  { value: 'graduated', label: 'Graduated', icon: 'circle-check' },
];

export const DEFAULT_MENTEE_FILTER: MenteeStatusFilter = 'all';

export const ALL_SKILLS_OPTION = { value: 'all', label: 'All skills' } as const;

export const MENTEE_PAGE_SIZE = 6;
export const MENTEE_SEARCH_DEBOUNCE_MS = 300;

/** Builds the header summary, e.g. "8 mentees across 6 projects". */
export function formatMenteesSummary(menteeCount: number, projectCount: number): string {
  const menteesLabel = menteeCount === 1 ? 'mentee' : 'mentees';
  const projectsLabel = projectCount === 1 ? 'project' : 'projects';

  return `${menteeCount.toLocaleString()} ${menteesLabel} across ${projectCount.toLocaleString()} ${projectsLabel}`;
}
