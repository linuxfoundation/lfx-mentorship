// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { MenteesListResponse } from '../../../app/types/mentee.types';
import { fetchMentees, toMenteesListResponse } from '../../utils/mentee';

function toOptionalInt(value: unknown): number | undefined {
  if (value == null || value === '') return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}

export default defineEventHandler(async (event): Promise<MenteesListResponse> => {
  const query = getQuery(event);
  const page = await fetchMentees({
    search: String(query.search ?? '').trim(),
    skill: String(query.skill ?? ''),
    status: String(query.status ?? ''),
    limit: toOptionalInt(query.limit),
    offset: toOptionalInt(query.offset),
  });
  return toMenteesListResponse(page);
});
