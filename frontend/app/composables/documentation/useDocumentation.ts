// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { onServerPrefetch } from 'vue';
import { useQuery } from '@tanstack/vue-query';
import type { FetchError } from 'ofetch';
import type { MaybeRefOrGetter } from 'vue';
import type { DocArticle } from '~/types/documentation.types';

export function useDocumentation(slug: MaybeRefOrGetter<string>) {
  const slugRef = toRef(slug);

  const query = useQuery<DocArticle>({
    queryKey: ['docs', 'article', slugRef] as const,
    queryFn: () => $fetch<DocArticle>(`/api/docs/${slugRef.value}`),
    staleTime: 5 * 60 * 1000,
    retry: false,
  });

  // Surface the API's status on the server so the response carries it. Swallowing here
  // would render the component's "Page not found" panel inside a 200, which is a soft
  // 404: crawlers index the miss, and a 500 from the API looks like a successful page.
  // The client keeps the in-place error panel, since there is no response left to set.
  //
  // `suspense()` only REJECTS when `throwOnError` says so; by default it resolves with
  // the settled result, error and all. So the error is read off the query afterwards
  // rather than caught — a try/catch around this await never fires.
  onServerPrefetch(async () => {
    await query.suspense().catch(() => undefined);

    const error = query.error.value as FetchError | null;
    if (!error) return;

    const status = error.statusCode ?? 500;
    throw createError({
      statusCode: status,
      statusMessage:
        status === 404 ? 'Documentation page not found' : 'Could not load this documentation page',
      fatal: true,
    });
  });

  return query;
}
