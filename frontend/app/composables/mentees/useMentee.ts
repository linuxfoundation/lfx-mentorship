// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { useQuery } from '@tanstack/vue-query';
import type { MaybeRef } from 'vue';
import { computed, toValue } from 'vue';
import type { MenteeDetail } from '~/types/mentee.types';

export function useMentee(menteeId: MaybeRef<string>) {
  return useQuery<MenteeDetail>({
    queryKey: computed(() => ['mentee', toValue(menteeId)]),
    queryFn: () => $fetch<MenteeDetail>(`/api/mentees/${toValue(menteeId)}`),
    enabled: computed(() => Boolean(toValue(menteeId))),
  });
}
