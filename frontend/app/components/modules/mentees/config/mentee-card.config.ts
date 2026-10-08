// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { TagStyle } from '~/components/uikit/tag/types/tag.types';
import type { MenteeProfileStatus } from '~/types/mentee.types';

export interface MenteeStatusConfig {
  label: string;
  variation: TagStyle;
}

export const MENTEE_STATUS_CONFIG: Record<MenteeProfileStatus, MenteeStatusConfig> = {
  active: { label: 'Current', variation: 'info' },
  graduated: { label: 'Graduated', variation: 'positive' },
  pending: { label: 'Applied', variation: 'warning' },
  declined: { label: 'Declined', variation: 'neutral' },
  withdrawn: { label: 'Withdrawn', variation: 'neutral' },
};

export const MENTEE_SKILLS_VISIBLE_COUNT = 3;
export const MENTEE_MENTORS_VISIBLE_COUNT = 3;
