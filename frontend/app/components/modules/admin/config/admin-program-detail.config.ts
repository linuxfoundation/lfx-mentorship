// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { Tab } from '~/components/uikit/tabs/types/tab.types';
import type { TagStyle } from '~/components/uikit/tag/types/tag.types';
import type {
  AdminApplicationStatus,
  AdminMentorStatus,
  AdminProgramDetailTab,
  AdminTermStatus,
} from '~/types/admin.types';

export const ADMIN_PROGRAM_DETAIL_TAB_ITEMS: Tab[] = [
  { value: 'overview', label: 'Overview' },
  { value: 'current-mentees', label: 'Current Mentees' },
  { value: 'past-mentees', label: 'Past Mentees' },
  { value: 'mentors', label: 'Mentors' },
  { value: 'terms', label: 'Terms' },
];

export const DEFAULT_ADMIN_PROGRAM_DETAIL_TAB: AdminProgramDetailTab = 'overview';

export const ADMIN_APPLICATION_STATUS_CONFIG: Record<
  AdminApplicationStatus,
  { label: string; variation: TagStyle }
> = {
  pending: { label: 'Pending', variation: 'warning' },
  accepted: { label: 'Accepted', variation: 'positive' },
  'tasks-submitted': { label: 'Tasks Submitted', variation: 'info' },
  graduated: { label: 'Graduated', variation: 'positive' },
  declined: { label: 'Declined', variation: 'negative' },
  withdrawn: { label: 'Withdrawn', variation: 'neutral' },
};

export const ADMIN_MENTOR_STATUS_CONFIG: Record<
  AdminMentorStatus,
  { label: string; variation: TagStyle }
> = {
  approved: { label: 'Approved', variation: 'positive' },
  pending: { label: 'Pending', variation: 'warning' },
};

export const ADMIN_TERM_STATUS_LABEL: Record<AdminTermStatus, string> = {
  open: 'open',
  closed: 'closed',
};

export const ADMIN_MENTEE_STATUS_FILTER_OPTIONS = [
  { value: 'all', label: 'Filter By Status' },
  { value: 'pending', label: 'Pending' },
  { value: 'accepted', label: 'Accepted' },
  { value: 'tasks-submitted', label: 'Tasks Submitted' },
  { value: 'graduated', label: 'Graduated' },
  { value: 'declined', label: 'Declined' },
  { value: 'withdrawn', label: 'Withdrawn' },
] as const;

export const ADMIN_MENTEE_TERM_FILTER_OPTIONS = [
  { value: 'all', label: 'Filter by Term' },
  { value: 'Term 3 · 2026', label: 'Term 3 · 2026' },
  { value: 'Term 2 · 2026', label: 'Term 2 · 2026' },
  { value: 'Term 1 · 2026', label: 'Term 1 · 2026' },
] as const;

export const ADMIN_CURRENT_MENTEES_NOTE =
  'Application status stays "Pending" while a mentee works on prerequisite tasks. When all prerequisites are complete, "Tasks Submitted" appears above the status and the program admin is notified by email to review the submission and make the admission decision.';

export const ADMIN_TABLE_PAGE_SIZE = 10;

/** Initial page size for Mentors tab (Load More). */
export const ADMIN_MENTOR_PAGE_SIZE = 3;
