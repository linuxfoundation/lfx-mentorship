// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { MentorsSummaryResponse } from '../../../app/types/mentor.types';
import { fetchMentorSummary } from '../../utils/mentor';

export default defineEventHandler((): Promise<MentorsSummaryResponse> => {
  return fetchMentorSummary();
});
