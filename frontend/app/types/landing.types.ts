// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

export interface LandingHeroFeature {
  icon: string;
  label: string;
}

export interface LandingStat {
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
