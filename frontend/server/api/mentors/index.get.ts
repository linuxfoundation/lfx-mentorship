// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { MentorsListResponse } from '../../../app/types/mentor.types';
import { fetchMentors, toMentorsListResponse } from '../../utils/mentor';

function toOptionalInt(value: unknown): number | undefined {
  if (value == null || value === '') return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}

export default defineEventHandler(async (event): Promise<MentorsListResponse> => {
  const query = getQuery(event);
  const page = await fetchMentors({
    search: String(query.search ?? '').trim(),
    skill: String(query.skill ?? ''),
    limit: toOptionalInt(query.limit),
    offset: toOptionalInt(query.offset),
  });
  return toMentorsListResponse(page);
});
