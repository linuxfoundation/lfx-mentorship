// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { getMenteeDetail } from '../../mock-data/directory-details';
import type { MenteeDetail } from '../../../app/types/mentee.types';

export default defineEventHandler((event): MenteeDetail => {
  const id = getRouterParam(event, 'id');
  const mentee = id ? getMenteeDetail(id) : undefined;

  if (!mentee) {
    throw createError({ statusCode: 404, statusMessage: 'Mentee not found' });
  }

  return mentee;
});
