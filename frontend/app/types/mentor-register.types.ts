// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

export const MENTOR_REQUEST_STATUSES = ['pending', 'approved'] as const;

export type MentorRequestStatus = (typeof MENTOR_REQUEST_STATUSES)[number];

export interface MentorProgramRequest {
  id: string;
  programId: string;
  programName: string;
  status: MentorRequestStatus;
}

export interface MentorRegisterForm {
  introduction: string;
  skills: string[];
  linkedinUrl: string;
  githubUrl: string;
  resumeFileName: string;
  complianceAccepted: boolean;
  termsAccepted: boolean;
}

export interface MentorRegisterPolicyLink {
  label: string;
  href: string;
}
