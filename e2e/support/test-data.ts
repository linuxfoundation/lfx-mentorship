// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// DEV data created for the P1 run. Keep in sync with "Test data created on DEV"
// in docs/testing/p1-test-plan.md.
export const PROGRAM_A = {
  id: '97059425-e2cb-4b9a-b432-411b78678c47',
  slug: 'e2e-p1-program-2026-10-26',
  name: 'E2E P1 Program 2026-10-26',
  project: 'Academy Software Foundation (ASWF)',
  terms: {
    a: { id: 'f426493e-d188-456a-8ee2-6fa54f5e69cf', name: 'E2E Term A Nov 2026' },
    b: { id: '26a7efd4-a373-4ea4-b1b5-b77371ce6b05', name: 'E2E Term B Dec 2026' },
  },
} as const;

export function applyPath(programId: string, programTermId: string): string {
  return `/mentorship/mentee/apply?programId=${programId}&programTermId=${programTermId}`;
}

// Programs section 4 enrolls as adminA and section 5 reviews. Each run of 4.7 adds one of each, named
// `<prefix> <timestamp>`; section 5 looks them up by prefix and takes one still pending.
export const REVIEW_PROGRAMS = {
  approve: 'E2E P1 Approve',
  reject: 'E2E P1 Reject',
  race: 'E2E P1 Race',
  pending: 'E2E P1 Pending',
} as const;

export type ReviewDecision = 'approve' | 'reject';

export function reviewPath(programId: string, decision: ReviewDecision): string {
  return `/mentorship/program-review/${programId}?decision=${decision}`;
}

// LFX usernames, for calls that name a user (an admin inviting a mentor). The plan names these accounts; they are not secrets.
export const LFIDS = {
  adminA: 'sameh100',
} as const;
