// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { LandingSummaryResponse } from '../../app/types/landing.types';
import { fetchPlatformSummary } from '../utils/platform-summary';

export default defineEventHandler((): Promise<LandingSummaryResponse> => {
  return fetchPlatformSummary();
});
