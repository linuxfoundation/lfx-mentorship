// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

export interface EnrollProgramBenefit {
  icon: string;
  title: string;
  description: string;
}

export interface EnrollProgramStep {
  step: number;
  title: string;
  description: string;
  checklist: string[];
}

export interface EnrollProgramGuidanceItem {
  text: string;
}
