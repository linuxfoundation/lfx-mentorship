// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { MenteeDetail } from '../../../app/types/mentee.types';
import { fetchMentee, mapMenteeDetail } from '../../utils/mentee';

export default defineEventHandler(async (event): Promise<MenteeDetail> => {
  const id = getRouterParam(event, 'id');
  if (!id) {
    throw createError({ statusCode: 400, message: 'Missing mentee id' });
  }
  const detail = await fetchMentee(id);
  return mapMenteeDetail(detail);
});
