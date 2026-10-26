// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { access } from 'node:fs/promises';
import { resolve } from 'node:path';
import { load as parseYaml } from 'js-yaml';

async function pathExists(p: string): Promise<boolean> {
  try {
    await access(p);
    return true;
  } catch {
    return false;
  }
}

export async function getDocsDir(): Promise<string> {
  // The Markdown lives at frontend/docs/user so it sits inside the image build context
  // (both workflows build with `context: frontend/`). Try each launch point in turn:
  //   runtime image:                 process.cwd() = /app      → docs/user
  //   dev (pnpm dev from frontend/): process.cwd() = …/frontend → docs/user
  //   repo root / CI:                process.cwd() = …/repo     → frontend/docs/user
  const candidates = [
    resolve(process.cwd(), 'docs/user'),
    resolve(process.cwd(), 'frontend/docs/user'),
  ];
  for (const candidate of candidates) {
    if (await pathExists(candidate)) return candidate;
  }
  return candidates[0]!;
}

export { pathExists };

export function parseFrontmatter(raw: string): { data: Record<string, unknown>; content: string } {
  const match = raw.match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n?([\s\S]*)$/);
  if (!match) return { data: {}, content: raw };
  let data: Record<string, unknown> = {};
  try {
    data = (parseYaml(match[1]) ?? {}) as Record<string, unknown>;
  } catch {
    // malformed frontmatter — fall back to empty metadata
  }
  return { data, content: match[2] };
}

// Rewrite relative doc links to absolute /docs/* paths.
// slugDir is the directory context of the current article (set by the route handler):
//   - top-level index.md  (slug = 'mentees')                    → slugDir = 'mentees'
//   - nested index.md     (slug = 'mentees/apply-to-a-program')       → slugDir = 'mentees/apply-to-a-program'
//   - direct .md file     (slug = 'mentees/apply-to-a-program/guide') → slugDir = 'mentees/apply-to-a-program'
export function rewriteDocLink(href: string, slugDir: string): string {
  // Anchors and root-relative paths pass through unchanged
  if (href.startsWith('#') || href.startsWith('/')) {
    return href;
  }

  // Any URL scheme (http:, https:, mailto:, tel:, etc.) passes through unchanged
  if (/^[a-z][a-z0-9+.-]*:/i.test(href)) {
    return href;
  }

  let path = href;

  if (path.startsWith('../')) {
    // Walk up the directory stack for each ../ segment — handles multi-level
    const parts = slugDir ? slugDir.split('/') : [];
    while (path.startsWith('../')) {
      parts.pop();
      path = path.slice(3);
    }
    path = parts.length ? `${parts.join('/')}/${path}` : path;
  } else if (path.startsWith('./')) {
    path = slugDir ? `${slugDir}/${path.slice(2)}` : path.slice(2);
  } else if (slugDir) {
    path = `${slugDir}/${path}`;
  }

  const cleaned = path
    .replace(/\/index\.md$/, '')
    .replace(/\.md$/, '')
    .replace(/\/$/, '');

  return `/docs/${cleaned}`;
}

// Escape characters that are unsafe inside HTML attribute values.
// Handles both double-quoted and single-quoted attributes, and prevents
// tag injection via angle brackets.
export function escapeAttr(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

export function formatDate(val: unknown): string {
  if (val instanceof Date) return val.toISOString().slice(0, 10);
  return String(val).slice(0, 10);
}

export function toTitleCase(slug: string): string {
  return slug
    .split('-')
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(' ');
}
