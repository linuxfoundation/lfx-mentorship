// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { MenteeStatus } from './mentee.types';

export const PROGRAM_STATUSES = ['acceptance', 'in-progress', 'completed'] as const;

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
  id: string;
  name: string;
  intro?: string;
  avatarUrl?: string;
}

/** Program mentee: currently in a term, or already graduated. */
export interface ProgramMentee extends ProgramMember {
  status: MenteeStatus;
}

/** Foundation owns many programs. */
export interface Foundation {
  id: string;
  name: string;
  slug: string;
}

/** Term belongs to a program; each program has at least one. */
export interface ProgramTerm {
  id: string;
  name: string;
  /** Display range, e.g. "Sep-Nov 2026". */
  dateRangeLabel?: string;
  /** Inclusive term start, ISO date `YYYY-MM-DD`. */
  startsAt: string;
  /** Inclusive term end, ISO date `YYYY-MM-DD`. */
  endsAt: string;
  /** ISO date when applications close for this term. */
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
  /** Active / current term shown on cards (must be one of `terms`). */
  activeTerm: ProgramTerm;
  updatedAt: string;
  repositoryUrl?: string;
  /**
   * Crowdfunding initiative id/slug used for Donate deep-link:
   * `${crowdfundingUrl}/initiatives/${crowdfundingInitiativeId}`
   */
  crowdfundingInitiativeId?: string;
  whatYouWillWorkOn?: string;
  prerequisites?: string;
  mentees: ProgramMentee[];
  mentors: ProgramMember[];
  sponsors: ProgramSponsor[];
}

export interface ProgramsListResponse {
  data: Program[];
  total: number;
  skills: string[];
  /** Unfiltered catalog size, used by the programs header summary. */
  programCount: number;
  /** Unique foundations in the catalog, used by the programs header summary. */
  foundationCount: number;
}
