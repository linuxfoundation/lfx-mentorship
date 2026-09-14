// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { useQuery } from '@tanstack/vue-query';
import type { MaybeRef } from 'vue';
import { computed, toValue } from 'vue';
import type { ProgramSponsor } from '~/types/program.types';

export function useProgramSponsors(programId: MaybeRef<string>, enabled: MaybeRef<boolean> = true) {
  return useQuery<ProgramSponsor[]>({
    queryKey: computed(() => ['program-sponsors', toValue(programId)]),
    queryFn: () => $fetch<ProgramSponsor[]>(`/api/programs/${toValue(programId)}/sponsors`),
    enabled: computed(() => Boolean(toValue(programId)) && Boolean(toValue(enabled))),
  });
}
