// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { TagStyle } from '~/components/uikit/tag/types/tag.types';
import type { ProgramStatus } from '~/types/program.types';

export interface ProgramStatusConfig {
  label: string;
  variation: TagStyle;
}

export const PROGRAM_STATUS_CONFIG: Record<ProgramStatus, ProgramStatusConfig> = {
  acceptance: { label: 'Accepting', variation: 'positive' },
  'open-soon': { label: 'Opens soon', variation: 'info' },
  'in-progress': { label: 'In progress', variation: 'warning' },
  completed: { label: 'Completed', variation: 'neutral' },
};

export const PROGRAM_SKILLS_VISIBLE_COUNT = 3;
