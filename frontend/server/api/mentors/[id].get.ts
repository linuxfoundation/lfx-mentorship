// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { MentorDetail } from '../../../app/types/mentor.types';
import { fetchMentor, mapMentorDetail } from '../../utils/mentor';

export default defineEventHandler(async (event): Promise<MentorDetail> => {
  const id = getRouterParam(event, 'id');
  if (!id) {
    throw createError({ statusCode: 400, message: 'Missing mentor id' });
  }
  const detail = await fetchMentor(id);
  return mapMentorDetail(detail);
});
