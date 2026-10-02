// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { AppRoute, selfServeMentorUrl, selfServeMenteeUrl } from '~/config/routes';

export interface HeaderMenuChild {
  label: string;
  icon: string;
  to: string;
}

export interface HeaderMenuItem {
  label: string;
  icon: string;
  to?: string;
  children?: HeaderMenuChild[];
}

export interface HeaderCtaItem {
  label: string;
  to: string;
  type: 'primary' | 'outline';
}

export function lfxHeaderCtas(selfServeUrl: string): HeaderCtaItem[] {
  return [
    { label: 'Enroll a Program', to: AppRoute.EnrollProgram, type: 'outline' },
    { label: 'Become a Mentor', to: selfServeMentorUrl(selfServeUrl), type: 'outline' },
    { label: 'My Mentorship', to: selfServeMenteeUrl(selfServeUrl), type: 'primary' },
  ];
}

export const lfxHeaderMenu: HeaderMenuItem[] = [
  { label: 'Find a Program', icon: 'magnifying-glass', to: AppRoute.FindProgram },
  { label: 'Mentees', icon: 'user-graduate', to: AppRoute.Mentees },
  { label: 'Mentors', icon: 'user-tie', to: AppRoute.Mentors },
];
