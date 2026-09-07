// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { MenteeStatus } from './mentee.types';

export const PROGRAM_STATUSES = ['open-soon', 'acceptance', 'in-progress', 'completed'] as const;

export type ProgramStatus = (typeof PROGRAM_STATUSES)[number];

export type ProgramStatusFilter = 'all' | ProgramStatus;

export type ProgramSortBy =
  | 'accepting_first'
  | 'completed_first'
  | 'name_asc'
  | 'name_desc'
  | 'updated_oldest'
  | 'updated_newest';

export interface ProgramMember {
  /** User id used for profile links, not the membership or application row. */
  id: string;
  name: string;
  intro?: string;
  avatarUrl?: string;
}

/** Program mentee: currently in a term, or already graduated. */
export interface ProgramMentee extends ProgramMember {
  status: MenteeStatus;
  /** Term id used with `id` for list keys when a user appears on more than one term. */
  termId?: string;
  /** Display term, e.g. "Fall 2026". */
  termLabel: string;
}

/** Foundation owns many programs. */
export interface Foundation {
  id: string;
  name: string;
  slug: string;
}

export const TERM_STATUSES = ['open', 'closed', 'deleted'] as const;

export type TermStatus = (typeof TERM_STATUSES)[number];

/** Term belongs to a program; each program has at least one. */
export interface ProgramTerm {
  id: string;
  name: string;
  status: TermStatus;
  /** Display range, e.g. "Sep-Nov 2026". */
  dateRangeLabel?: string;
  /** Inclusive term start, ISO date `YYYY-MM-DD`. */
  startsAt: string;
  /** Inclusive term end, ISO date `YYYY-MM-DD`. */
  endsAt: string;
  /** ISO datetime when applications open for this term. */
  applicationsStartAt?: string;
  /** ISO datetime when applications close for this term. */
  applicationsCloseAt?: string;
}

export interface ProgramSponsor {
  id: string;
  name: string;
  logoUrl?: string;
  /** Contribution amount in cents. */
  amountCents: number;
}

export interface Program {
  id: string;
  slug: string;
  name: string;
  description: string;
  logoUrl?: string;
  skills: string[];
  status: ProgramStatus;
  foundation: Foundation;
  terms: ProgramTerm[];
  /**
   * Open terms whose application window includes now.
   * Computed from `terms` — not stored independently.
   */
  activeTerms: ProgramTerm[];
  updatedAt: string;
  repositoryUrl?: string;
  mentees: ProgramMentee[];
  mentors: ProgramMember[];
  sponsors: ProgramSponsor[];
  isPaid?: boolean;
}

export interface ProgramsListResponse {
  data: Program[];
  /** Total matching programs after filters (used for pagination). */
  total: number;
  /** Unique foundations in the catalog, used by the programs header summary. */
  foundationCount: number;
}
