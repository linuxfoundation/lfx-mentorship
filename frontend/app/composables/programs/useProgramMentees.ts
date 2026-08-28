// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { useQuery } from '@tanstack/vue-query';
import type { MaybeRef } from 'vue';
import { computed, toValue } from 'vue';
import type { ProgramMentee } from '~/types/program.types';

export function useProgramMentees(programId: MaybeRef<string>, enabled: MaybeRef<boolean> = true) {
  return useQuery<ProgramMentee[]>({
    queryKey: computed(() => ['program-mentees', toValue(programId)]),
    queryFn: () => $fetch<ProgramMentee[]>(`/api/programs/${toValue(programId)}/mentees`),
    enabled: computed(() => Boolean(toValue(programId)) && Boolean(toValue(enabled))),
  });
}
