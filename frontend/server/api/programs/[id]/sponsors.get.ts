// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { ProgramSponsorsResponse } from '../../../../app/types/program.types';
import { fetchProgramSponsors, mapCatalogSponsor } from '../../../utils/program-catalog';

export default defineEventHandler(async (event): Promise<ProgramSponsorsResponse> => {
  const id = getRouterParam(event, 'id');

  if (!id) {
    throw createError({ statusCode: 400, message: 'Missing program id' });
  }

  const response = await fetchProgramSponsors(id);
  return {
    sponsors: (response.data ?? []).map(mapCatalogSponsor),
    crowdfundingInitiative: response.crowdfunding_initiative ?? true,
  };
});
