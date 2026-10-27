// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { readFile } from 'node:fs/promises';
import { resolve, join, sep } from 'node:path';
import { marked, Renderer } from 'marked';
import DOMPurify from 'isomorphic-dompurify';
import {
  getDocsDir,
  parseFrontmatter,
  rewriteDocLink,
  escapeAttr,
  formatDate,
  toTitleCase,
} from '../../utils/doc-utils';
import type { DocArticle } from '../../../app/types/documentation.types';

// marked is pinned to v4 in this repo (the Quill markdown-paste helper depends on it),
// so the renderer hooks take POSITIONAL arguments — `link(href, title, text)` — not the
// single options object that marked v5+ passes. Writing these against the newer shape
// silently yields `undefined` link text and empty images.
export function buildRenderer(slugDir: string, baseUrl: string): Renderer {
  const renderer = new Renderer();
  renderer.link = (href: string | null, title: string | null, text: string) => {
    const rewritten = rewriteDocLink(href ?? '', slugDir);
    const isExternal = rewritten.startsWith('http://') || rewritten.startsWith('https://');
    const attrs = [
      `href="${escapeAttr(rewritten)}"`,
      title ? `title="${escapeAttr(title)}"` : '',
      isExternal ? 'target="_blank" rel="noopener noreferrer"' : '',
    ]
      .filter(Boolean)
      .join(' ');
    return `<a ${attrs}>${text}</a>`;
  };
  renderer.image = (href: string | null, title: string | null, text: string) => {
    const src = href?.startsWith('/') ? `${baseUrl.replace(/\/$/, '')}${href}` : (href ?? '');
    const titleAttr = title ? ` title="${escapeAttr(title)}"` : '';
    return `<img src="${escapeAttr(src)}" alt="${escapeAttr(text ?? '')}"${titleAttr}>`;
  };
  return renderer;
}

export default defineEventHandler(async (event): Promise<DocArticle> => {
  const slugParts = getRouterParam(event, 'slug');

  if (!slugParts) {
    throw createError({ statusCode: 400, message: 'Missing slug' });
  }

  const normalised = slugParts.toLowerCase().replace(/^\/|\/$/g, '');
  const safeDocsDir = resolve(await getDocsDir());

  // Build candidate paths then guard against path traversal before any disk access
  const candidates = [
    resolve(join(safeDocsDir, `${normalised}.md`)),
    resolve(join(safeDocsDir, normalised, 'index.md')),
  ];

  if (candidates.some((c) => !c.startsWith(safeDocsDir + sep))) {
    throw createError({ statusCode: 400, message: 'Invalid slug' });
  }

  let raw: string | null = null;
  let isIndex = false;
  for (const [i, candidate] of candidates.entries()) {
    try {
      raw = await readFile(candidate, 'utf-8');
      isIndex = i === 1;
      break;
    } catch {
      // try next candidate
    }
  }

  if (!raw) {
    throw createError({ statusCode: 404, message: `Documentation page not found: ${normalised}` });
  }

  // Directory context for resolving relative links:
  // index.md files (slug = 'mentees') → slugDir = 'mentees'
  // direct .md files (slug = 'mentees/history') → slugDir = 'mentees'
  const slugDir = isIndex ? normalised : normalised.split('/').slice(0, -1).join('/');

  const {
    public: { appUrl },
  } = useRuntimeConfig();
  const { data: fm, content } = parseFrontmatter(raw);
  const renderedHtml = await marked.parse(content, {
    renderer: buildRenderer(slugDir, appUrl),
    async: true,
  });
  // DOMPurify drops `target` by default, which would silently undo the renderer's
  // target="_blank" on external links. It is allowed back explicitly; the same hook
  // always pairs it with rel="noopener noreferrer", so the opener stays severed.
  const bodyHtml = DOMPurify.sanitize(renderedHtml, { ADD_ATTR: ['target'] });

  return {
    slug: normalised,
    title:
      (fm.title as string | undefined) ?? toTitleCase(normalised.split('/').pop() ?? normalised),
    description: (fm.description as string | undefined) ?? '',
    bodyHtml,
    tags: (fm.tags as string[] | undefined) ?? [],
    lastUpdated: fm.last_updated != null ? formatDate(fm.last_updated) : null,
    intercomCollection: (fm.intercom_collection as string | undefined) ?? null,
  };
});
