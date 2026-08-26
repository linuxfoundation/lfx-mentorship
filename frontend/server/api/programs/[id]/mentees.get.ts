// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { fetchProgramMentees, mapCatalogMentee } from '../../../utils/program-catalog';

export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id');

  if (!id) {
    throw createError({ statusCode: 400, message: 'Missing program id' });
  }

  const response = await fetchProgramMentees(id);
  return (response.data ?? []).map(mapCatalogMentee);
});
