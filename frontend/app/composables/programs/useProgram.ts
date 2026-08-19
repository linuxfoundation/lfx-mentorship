// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { useQuery } from '@tanstack/vue-query';
import type { MaybeRef } from 'vue';
import { computed, toValue } from 'vue';
import type { Program } from '~/types/program.types';

export function useProgram(id: MaybeRef<string>) {
  return useQuery<Program>({
    queryKey: computed(() => ['program', toValue(id)]),
    queryFn: () => $fetch<Program>(`/api/programs/${toValue(id)}`),
    enabled: computed(() => !!toValue(id)),
  });
}
