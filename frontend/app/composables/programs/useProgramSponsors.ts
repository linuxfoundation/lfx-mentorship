// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { useQuery } from '@tanstack/vue-query';
import type { MaybeRef } from 'vue';
import { computed, toValue } from 'vue';
import type { ProgramSponsorsResponse } from '~/types/program.types';

export function useProgramSponsors(programId: MaybeRef<string>, enabled: MaybeRef<boolean> = true) {
  return useQuery<ProgramSponsorsResponse>({
    queryKey: computed(() => ['program-sponsors', toValue(programId)]),
    queryFn: () =>
      $fetch<ProgramSponsorsResponse>(
        `/api/programs/${encodeURIComponent(toValue(programId))}/sponsors`,
      ),
    enabled: computed(() => Boolean(toValue(programId)) && Boolean(toValue(enabled))),
  });
}
