// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { AppRoute } from '~/config/routes';

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

export const lfxHeaderCtas: HeaderCtaItem[] = [];

export const lfxHeaderMenu: HeaderMenuItem[] = [
  { label: 'Find a Program', icon: 'magnifying-glass', to: AppRoute.FindProgram },
  { label: 'Mentees', icon: 'user-graduate', to: AppRoute.Mentees },
  { label: 'Mentors', icon: 'user-tie', to: AppRoute.Mentors },
  {
    label: 'More',
    icon: 'ellipsis',
    children: [
      { label: 'About', icon: 'circle-info', to: AppRoute.About },
      { label: 'Documentation', icon: 'book-open', to: AppRoute.Docs },
    ],
  },
];
