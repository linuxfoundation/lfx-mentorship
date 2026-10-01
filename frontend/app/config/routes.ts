// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

export enum AppRoute {
  Home = '/',
  FindProgram = '/programs',
  EnrollProgram = '/programs/enroll',
  Mentees = '/mentees',
  Mentors = '/mentors',
}

export function programPath(id: string): string {
  return `${AppRoute.FindProgram}/${id}`;
}

export function menteePath(id: string): string {
  return `${AppRoute.Mentees}/${id}`;
}

export function mentorPath(id: string): string {
  return `${AppRoute.Mentors}/${id}`;
}

/** Mentee apply flow in LFX Self Serve, which handles sign-in and account creation. */
export function selfServeMenteeApplyUrl(
  selfServeUrl: string,
  programId: string,
  programTermId: string,
): string {
  const base = selfServeUrl.replace(/\/$/, '');
  const query = new URLSearchParams({ programId, programTermId });
  return `${base}/mentorship/mentee/apply?${query.toString()}`;
}
