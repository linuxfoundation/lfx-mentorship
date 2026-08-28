// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

export const MENTEE_STATUSES = ['active', 'graduated'] as const;

export type MenteeStatus = (typeof MENTEE_STATUSES)[number];

export type MenteeStatusFilter = 'all' | MenteeStatus;

export const PROFILE_PROGRAM_STATUSES = ['accepting', 'closed', 'graduated', 'active'] as const;

export type ProfileProgramStatus = (typeof PROFILE_PROGRAM_STATUSES)[number];

export interface DirectoryMentorRef {
  id: string;
  name: string;
  title?: string;
  avatarUrl?: string;
}

export interface DirectoryProjectRef {
  id: string;
  name: string;
  /** Short foundation / org label shown before the project name, e.g. "CNCF". */
  foundationLabel: string;
}

export interface ProfileProgramTerm {
  id: string;
  /** e.g. "Term 3: Sep-Nov" */
  label: string;
  /** e.g. "Sep 2023 - Nov 2023" */
  dateRangeLabel: string;
}

export interface ProfileProgram {
  id: string;
  title: string;
  description: string;
  foundationLabel: string;
  status: ProfileProgramStatus;
  skills: string[];
  terms: ProfileProgramTerm[];
  mentors?: DirectoryMentorRef[];
  /** Two-letter fallback when no logo, e.g. "TH". */
  logoInitials: string;
  logoUrl?: string;
}

export interface Mentee {
  id: string;
  name: string;
  bio: string;
  skills: string[];
  status: MenteeStatus;
  /** Display label, e.g. "Since Aug. 2023". */
  sinceLabel: string;
  /** ISO date used for sorting / filtering. */
  joinedAt: string;
  project: DirectoryProjectRef;
  mentors: DirectoryMentorRef[];
  avatarUrl?: string;
}

export interface MenteeStats {
  programs: number;
  termsCompleted: number;
  mentors: number;
}

export interface MenteeDetail extends Mentee {
  githubUrl?: string;
  linkedinUrl?: string;
  stats: MenteeStats;
  programs: ProfileProgram[];
}

export interface MenteesListResponse {
  data: Mentee[];
  total: number;
  /** Unfiltered catalog size for header summary. */
  menteeCount: number;
  /** Unique projects in the catalog for header summary. */
  projectCount: number;
}
