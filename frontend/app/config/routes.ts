// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

export enum AppRoute {
  Home = '/',
  FindProgram = '/programs',
  Mentees = '/mentees',
  Mentors = '/mentors',
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
