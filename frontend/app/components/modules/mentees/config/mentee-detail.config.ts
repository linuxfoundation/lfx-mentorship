// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { TagStyle } from '~/components/uikit/tag/types/tag.types';
import type { ProfileProgramStatus } from '~/types/mentee.types';

export interface ProfileProgramStatusConfig {
  label: string;
  variation: TagStyle;
}

export const PROFILE_PROGRAM_STATUS_CONFIG: Record<
  ProfileProgramStatus,
  ProfileProgramStatusConfig
> = {
  accepting: { label: 'Accepting', variation: 'positive' },
  closed: { label: 'Closed', variation: 'neutral' },
  graduated: { label: 'Graduated', variation: 'positive' },
  active: { label: 'Active', variation: 'info' },
  acceptance: { label: 'Accepting', variation: 'positive' },
  'open-soon': { label: 'Opens soon', variation: 'info' },
  'in-progress': { label: 'In progress', variation: 'warning' },
  completed: { label: 'Completed', variation: 'neutral' },
};
