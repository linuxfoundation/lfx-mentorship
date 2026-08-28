// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { ProgramsListResponse } from '../../../app/types/program.types';
import { fetchProgramCatalog, mapCatalogItemToProgram } from '../../utils/program-catalog';

function toOptionalInt(value: unknown): number | undefined {
  if (value == null || value === '') return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}

export default defineEventHandler(async (event): Promise<ProgramsListResponse> => {
  const query = getQuery(event);
  const catalog = await fetchProgramCatalog({
    search: String(query.search ?? '').trim(),
    skill: String(query.skill ?? ''),
    status: String(query.status ?? ''),
    sortBy: String(query.sortBy ?? ''),
    limit: toOptionalInt(query.limit),
    offset: toOptionalInt(query.offset),
  });

  return {
    data: catalog.data.map(mapCatalogItemToProgram),
    total: catalog.meta.total,
    foundationCount: 0,
  };
});
