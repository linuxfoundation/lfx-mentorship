// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

export const ADMIN_TABS = ['my-programs', 'enroll'] as const;
export type AdminTab = (typeof ADMIN_TABS)[number];

export const ADMIN_PROGRAM_STATUSES = ['published', 'pending-review'] as const;
export type AdminProgramStatus = (typeof ADMIN_PROGRAM_STATUSES)[number];

export const ADMIN_ENROLL_STEPS = ['details', 'setup', 'prerequisites'] as const;
export type AdminEnrollStep = (typeof ADMIN_ENROLL_STEPS)[number];

export const ADMIN_PROGRAM_DETAIL_TABS = [
  'overview',
  'current-mentees',
  'past-mentees',
  'mentors',
  'terms',
] as const;
export type AdminProgramDetailTab = (typeof ADMIN_PROGRAM_DETAIL_TABS)[number];

export const ADMIN_APPLICATION_STATUSES = [
  'pending',
  'accepted',
  'tasks-submitted',
  'graduated',
  'declined',
  'withdrawn',
] as const;
export type AdminApplicationStatus = (typeof ADMIN_APPLICATION_STATUSES)[number];

export const ADMIN_MENTOR_STATUSES = ['approved', 'pending'] as const;
export type AdminMentorStatus = (typeof ADMIN_MENTOR_STATUSES)[number];

export const ADMIN_TERM_STATUSES = ['open', 'closed'] as const;
export type AdminTermStatus = (typeof ADMIN_TERM_STATUSES)[number];

export interface AdminProgramStats {
  mentors: number;
  currentMentees: number;
  graduatedMentees: number;
  fundingToDateCents: number;
}

export interface AdminProgram {
  id: string;
  name: string;
  foundationName: string;
  termLabel: string;
  status: AdminProgramStatus;
  logoUrl?: string;
  stats: AdminProgramStats;
  hidden?: boolean;
}

export interface AdminProgramTerm {
  id: string;
  name: string;
  startsLabel: string;
  endsLabel: string;
}

export interface AdminPrerequisite {
  id: string;
  name: string;
  description: string;
  required: boolean;
  /** Optional URL field (e.g. coding challenge). */
  urlValue?: string;
  urlPlaceholder?: string;
}

export interface AdminEnrollForm {
  importProgramId: string;
  name: string;
  projectId: string;
  technologies: string[];
  description: string;
  repositoryUrl: string;
  websiteUrl: string;
  ciiProjectId: string;
  codeOfConductUrl: string;
  logoFileName: string;
  /** Object URL or remote URL for the logo preview. Kept on the form so it survives step changes. */
  logoPreviewUrl: string;
  skills: string[];
  terms: AdminProgramTerm[];
  prerequisites: AdminPrerequisite[];
  termsAccepted: boolean;
}

export interface AdminOtherApplication {
  programName: string;
  status: AdminApplicationStatus;
}

export interface AdminMenteeApplication {
  id: string;
  name: string;
  termLabel: string;
  status: AdminApplicationStatus;
  createdLabel: string;
  updatedLabel: string;
  otherApplications: AdminOtherApplication[];
}

export interface AdminProgramMentorRow {
  id: string;
  name: string;
  status: AdminMentorStatus;
  /** e.g. "Invited Apr 2, 2026" or "Applied May 18, 2026" */
  entryLabel: string;
  profileCreated: boolean;
}

export interface AdminManagedTerm {
  id: string;
  name: string;
  status: AdminTermStatus;
  pending: number;
  declined: number;
  accepted: number;
  graduated: number;
  startLabel: string;
  endLabel: string;
  applicationStartLabel: string;
  applicationEndLabel: string;
}

export interface AdminProgramDetail extends AdminProgram {
  description: string;
  skills: string[];
  repositoryUrl?: string;
  websiteUrl?: string;
  about?: string;
  termDetailsLabel: string;
  applicationsCloseLabel?: string;
  currentMentees: AdminMenteeApplication[];
  pastMentees: AdminMenteeApplication[];
  mentors: AdminProgramMentorRow[];
  managedTerms: AdminManagedTerm[];
}
