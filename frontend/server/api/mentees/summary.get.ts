// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { MenteesSummaryResponse } from '../../../app/types/mentee.types';
import { fetchMenteeSummary } from '../../utils/mentee';

export default defineEventHandler((): Promise<MenteesSummaryResponse> => {
  return fetchMenteeSummary();
});
