// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { Tab } from '~/components/uikit/tabs/types/tab.types';
import type { ProgramSortBy, ProgramStatusFilter } from '~/types/program.types';

export const PROGRAM_FILTER_TABS: Tab[] = [
  { value: 'all', label: 'All', icon: 'grid-round-2' },
  { value: 'acceptance', label: 'Accepting', icon: 'inbox' },
  { value: 'in-progress', label: 'In progress', icon: 'play' },
  { value: 'completed', label: 'Completed', icon: 'circle-check' },
];

export const DEFAULT_PROGRAM_FILTER: ProgramStatusFilter = 'all';

export const ALL_SKILLS_OPTION = { value: 'all', label: 'All skills' } as const;

export const PROGRAM_PAGE_SIZE = 6;

export interface ProgramSortOption {
  value: ProgramSortBy;
  label: string;
}

export const DEFAULT_PROGRAM_SORT: ProgramSortOption = {
  value: 'accepting_first',
  label: 'Accepting First',
};

export const PROGRAM_SORT_OPTIONS: ProgramSortOption[] = [
  DEFAULT_PROGRAM_SORT,
  { value: 'completed_first', label: 'Completed first' },
  { value: 'name_asc', label: 'Name (A-Z)' },
  { value: 'name_desc', label: 'Name (Z-A)' },
  { value: 'updated_oldest', label: 'Updated (Oldest First)' },
  { value: 'updated_newest', label: 'Updated (Newest First)' },
];

export const PROGRAMS_HEADER_SUBTITLE =
  'All programs across 42 foundations, including terms already under way.';
