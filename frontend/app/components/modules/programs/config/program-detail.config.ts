// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { Tab } from '~/components/uikit/tabs/types/tab.types';
import type { TagStyle } from '~/components/uikit/tag/types/tag.types';
import type { ProgramTermDisplayStatus } from '~/utils/program-terms';

export const PROGRAM_DETAIL_TABS: Tab[] = [
  { value: 'overview', label: 'Overview' },
  { value: 'terms', label: 'Terms' },
  { value: 'mentors', label: 'Mentors' },
  { value: 'mentees', label: 'Mentees' },
  { value: 'sponsors', label: 'Sponsors' },
];

export const DEFAULT_PROGRAM_DETAIL_TAB = 'overview';

export const PROGRAM_CURRENT_MENTEES_HEADING = 'Current Mentees';
export const PROGRAM_GRADUATED_MENTEES_HEADING = 'Graduated Mentees';

export const PROGRAM_TERMS_INTRO =
  'This program runs in the terms below. Each term has its own application window and mentee cohort.';

export const PROGRAM_TERM_STATUS_CONFIG: Record<
  ProgramTermDisplayStatus,
  { label: string; variation: TagStyle }
> = {
  'opens-soon': { label: 'Opens Soon', variation: 'warning' },
  accepting: { label: 'Accepting', variation: 'positive' },
  completed: { label: 'Completed', variation: 'neutral' },
};

export const PROGRAM_SPONSORS_INTRO =
  'Stipends for this program are funded by the organizations below. Contributions are managed through the LFX funding platform.';

export const SIGN_IN_TO_APPLY_TITLE = 'Sign in to apply';

export const SIGN_IN_TO_APPLY_BODY =
  'Applications are submitted through your LFX account. Sign in or create one, and we will take you straight to the application for this program.';

export const SIGN_IN_TO_APPLY_LABEL = 'Applying to';
