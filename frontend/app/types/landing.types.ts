// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

export interface LandingHeroFeature {
  icon: string;
  iconType?: 'solid' | 'regular' | 'light';
  label: string;
}

export interface LandingStat {
  key: keyof LandingSummaryResponse;
  value: string;
  label: string;
}

export interface LandingHowItWorksStep {
  step: number;
  title: string;
  description: string;
}

export interface LandingEligibilityItem {
  id: string;
  text: string;
  /** Optional segments for bold emphasis; if omitted, `text` is shown as-is. */
  parts?: Array<{ text: string; bold?: boolean }>;
}

export interface LandingBenefit {
  icon: string;
  title: string;
  description: string;
}

export interface LandingFaqItem {
  question: string;
  answer: string;
}

/**
 * Aggregated marketing counts served by the BFF `/api/summary` endpoint,
 * which proxies the backend `GET /v1/summary`.
 */
export interface LandingGraduatedMentee {
  name?: string;
  avatarUrl?: string;
}

export interface LandingSummaryResponse {
  programCount: number;
  acceptingProgramCount: number;
  mentorCount: number;
  graduatedMenteeCount: number;
  foundationCount: number;
  stipendsPaid: number;
  graduatedMentees: LandingGraduatedMentee[];
}
