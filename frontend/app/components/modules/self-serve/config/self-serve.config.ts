// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { AppRoute } from '~/config/routes';
import type { SelfServeNavItem } from '~/types/self-serve.types';

export const SELF_SERVE_SECTION_LABEL = 'Mentorship';

export const SELF_SERVE_NAV_ITEMS: SelfServeNavItem[] = [
  {
    id: 'mentee',
    label: 'Mentee',
    icon: 'user-graduate',
    to: AppRoute.SelfServeMentee,
  },
  {
    id: 'mentor',
    label: 'Mentor',
    icon: 'user-tie',
    to: AppRoute.SelfServeMentor,
  },
  {
    id: 'admin',
    label: 'Admin',
    icon: 'shield-halved',
    to: AppRoute.SelfServeAdmin,
  },
];

export const SELF_SERVE_EMPTY_COPY = 'This section is coming soon.';
