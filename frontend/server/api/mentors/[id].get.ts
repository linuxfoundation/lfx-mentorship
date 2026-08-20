// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { getMentorDetail } from '../../mock-data/directory-details';
import type { MentorDetail } from '../../../app/types/mentor.types';

export default defineEventHandler((event): MentorDetail => {
  const id = getRouterParam(event, 'id');
  const mentor = id ? getMentorDetail(id) : undefined;

  if (!mentor) {
    throw createError({ statusCode: 404, statusMessage: 'Mentor not found' });
  }

  return mentor;
});
