// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { fetchProgramCatalog } from '../../../utils/program-catalog';

const PAGE_SIZE = 100;
const MAX_PAGES = 50;

export default defineSitemapEventHandler(async () => {
  const entries: Array<{ loc: string; lastmod?: string }> = [];
  let offset = 0;

  try {
    for (let page = 0; page < MAX_PAGES; page++) {
      const res = await fetchProgramCatalog({
        limit: PAGE_SIZE,
        offset,
        sortBy: 'name_asc',
      });

      for (const item of res.data) {
        if (!item.slug) continue;
        entries.push({
          loc: `/programs/${item.slug}`,
          lastmod: item.updated_on || undefined,
        });
      }

      offset += res.data.length;
      if (res.data.length === 0 || offset >= res.meta.total) break;
    }
  } catch {
    return entries;
  }

  return entries;
});
