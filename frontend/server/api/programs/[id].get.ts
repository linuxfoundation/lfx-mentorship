// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { MOCK_PROGRAMS } from '../../mock-data/programs';

export default defineEventHandler((event) => {
  const id = getRouterParam(event, 'id');

  if (!id) {
    throw createError({ statusCode: 400, message: 'Missing program id' });
  }

  const program = MOCK_PROGRAMS.find((item) => item.id === id);

  if (!program) {
    throw createError({ statusCode: 404, message: 'Program not found' });
  }

  return program;
});
