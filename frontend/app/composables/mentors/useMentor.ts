// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { useQuery } from '@tanstack/vue-query';
import type { MaybeRef } from 'vue';
import { computed, toValue } from 'vue';
import type { MentorDetail } from '~/types/mentor.types';

export function useMentor(mentorId: MaybeRef<string>) {
  return useQuery<MentorDetail>({
    queryKey: computed(() => ['mentor', toValue(mentorId)]),
    queryFn: () => $fetch<MentorDetail>(`/api/mentors/${toValue(mentorId)}`),
    enabled: computed(() => Boolean(toValue(mentorId))),
  });
}
