// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { getAdminProgramDetail } from '../../../mock-data/admin-programs';
import type { AdminProgramDetail } from '../../../app/types/admin.types';

export default defineEventHandler((event): AdminProgramDetail => {
  const id = getRouterParam(event, 'id');
  const program = id ? getAdminProgramDetail(id) : undefined;

  if (!program) {
    throw createError({ statusCode: 404, statusMessage: 'Admin program not found' });
  }

  return program;
});
