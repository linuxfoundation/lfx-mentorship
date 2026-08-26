// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { fetchProgramCatalogItem, mapCatalogItemToProgram } from '../../utils/program-catalog';

export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id');

  if (!id) {
    throw createError({ statusCode: 400, message: 'Missing program id' });
  }

  const item = await fetchProgramCatalogItem(id);
  return mapCatalogItemToProgram(item);
});
