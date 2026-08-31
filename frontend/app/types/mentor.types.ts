// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { DirectoryMentorRef, ProfileProgram } from './mentee.types';

export interface Mentor {
  id: string;
  name: string;
  bio: string;
  skills: string[];
  /** Display label, e.g. "Since Apr. 2023". */
  sinceLabel: string;
  /** ISO date used for sorting. */
  joinedAt: string;
  /** Mock-only program names; live list search is name-only. */
  projects?: string[];
  avatarUrl?: string;
}

export interface MentorStats {
  programsMentoring: number;
  currentMentees: number;
  menteesGraduated: number;
}

export interface MentorMenteeSummary {
  id: string;
  name: string;
  bio: string;
  /** e.g. "Thanos · Term 3" */
  programLabel: string;
  avatarUrl?: string;
}

export interface MentorDetail extends Mentor {
  /** e.g. "LF Energy · GridFlow, CNCF · Thanos" */
  affiliationsLabel: string;
  githubUrl?: string;
  linkedinUrl?: string;
  stats: MentorStats;
  programs: ProfileProgram[];
  currentMentees: MentorMenteeSummary[];
  graduatedMentees: MentorMenteeSummary[];
  /** Unused on mentor detail but kept for shared typing with directory refs. */
  coMentors?: DirectoryMentorRef[];
}

export interface MentorsListResponse {
  data: Mentor[];
  total: number;
}

export interface MentorsSummaryResponse {
  mentorCount: number;
  programCount: number;
}
