// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { Tab } from '~/components/uikit/tabs/types/tab.types';

export const PROGRAM_DETAIL_TABS: Tab[] = [
  { value: 'overview', label: 'Overview' },
  { value: 'mentors', label: 'Mentors' },
  { value: 'mentees', label: 'Mentees' },
  { value: 'sponsors', label: 'Sponsors' },
];

export const DEFAULT_PROGRAM_DETAIL_TAB = 'overview';

export const PROGRAM_CURRENT_MENTEES_HEADING = 'Current Mentees';
export const PROGRAM_GRADUATED_MENTEES_HEADING = 'Graduated Mentees';

export const PROGRAM_SPONSORS_INTRO =
  'Stipends for this program are funded by the organizations below. Contributions are managed through the LFX funding platform.';
