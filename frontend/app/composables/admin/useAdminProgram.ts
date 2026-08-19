// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { useQuery } from '@tanstack/vue-query';
import type { MaybeRef } from 'vue';
import { computed, toValue } from 'vue';
import type { AdminProgramDetail } from '~/types/admin.types';

export function useAdminProgram(programId: MaybeRef<string>) {
  return useQuery<AdminProgramDetail>({
    queryKey: computed(() => ['admin-program', toValue(programId)]),
    queryFn: () => $fetch<AdminProgramDetail>(`/api/admin/programs/${toValue(programId)}`),
    enabled: computed(() => Boolean(toValue(programId))),
  });
}
