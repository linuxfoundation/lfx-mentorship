// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

export enum AppRoute {
  Home = '/',
  FindProgram = '/programs',
  Mentees = '/mentees',
  Mentors = '/mentors',
  EnrollProgram = '/enroll-program',
  SelfServe = '/self-serve',
  SelfServeMentee = '/self-serve/mentee',
  SelfServeMentor = '/self-serve/mentor',
  SelfServeAdmin = '/self-serve/admin',
  /** @deprecated Use SelfServeMentor */
  MentorRegister = '/self-serve/mentor',
  /** User mentorship hub (self-serve). */
  MyMentorship = '/self-serve',
  Docs = '/docs',
  About = '/about',
  Contact = '/contact',
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

export function adminProgramPath(id: string): string {
  return `${AppRoute.SelfServeAdmin}/${id}`;
}
