// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

/**
 * Build script: scans frontend/docs/user/ and writes a search index to
 * public/assets/docs/search-index.json for use by the client-side search component.
 *
 * The sources live inside frontend/ on purpose: both image workflows build with
 * `context: frontend/`, so anything at the repo root is outside the build context and
 * never reaches the builder. A missing directory is a build failure rather than an
 * empty index, since an empty index is indistinguishable from a working portal with no
 * articles until someone opens /docs in a deployed environment.
 *
 * Run via: pnpm docs:build
 * Auto-runs before: pnpm dev, pnpm build
 */

import { readdirSync, readFileSync, writeFileSync, existsSync, mkdirSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { load as parseYaml } from 'js-yaml';

const DOCS_DIR = resolve(import.meta.dirname, '../docs/user');
const OUT_FILE = resolve(import.meta.dirname, '../public/assets/docs/search-index.json');

function parseFrontmatter(raw) {
  const match = raw.match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n?([\s\S]*)$/);
  if (!match) return { data: {}, content: raw };
  let data = /** @type {Record<string, unknown>} */ ({});
  try {
    data = /** @type {Record<string, unknown>} */ (parseYaml(match[1]) ?? {});
  } catch {
    // malformed frontmatter — fall back to empty metadata
  }
  return { data, content: match[2] };
}

function stripMarkdown(md) {
  return md
    .replace(/!\[.*?\]\(.*?\)/g, '') // images
    .replace(/\[([^\]]+)\]\(.*?\)/g, '$1') // links → text only
    .replace(/`{1,3}[^`\n]*`{1,3}/g, '') // inline code
    .replace(/^```[\s\S]*?^```/gm, '') // fenced code blocks
    .replace(/^#+\s+/gm, '') // headings
    .replace(/[*_~]{1,3}([^*_~\n]+)[*_~]{1,3}/g, '$1') // bold / italic / strikethrough
    .replace(/^\s*[-*+>|]\s*/gm, '') // lists, blockquotes, table pipes
    .replace(/\n{2,}/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
}

/**
 * @param {string} dir
 * @param {string} parentSlug
 * @returns {Array<{slug: string, title: string, description: string, content: string}>}
 */
function walk(dir, parentSlug = '') {
  const docs = [];

  if (!existsSync(dir)) return docs;

  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;

    const slug = parentSlug ? `${parentSlug}/${entry.name}` : entry.name;
    const indexPath = join(dir, entry.name, 'index.md');

    if (!existsSync(indexPath)) continue;

    const raw = readFileSync(indexPath, 'utf-8');
    const { data: fm, content } = parseFrontmatter(raw);

    docs.push({
      slug,
      title: String(fm.title ?? entry.name),
      description: String(fm.description ?? ''),
      content: stripMarkdown(content),
    });

    // recurse into sub-sections
    docs.push(...walk(join(dir, entry.name), slug));
  }

  return docs;
}

if (!existsSync(DOCS_DIR)) {
  process.stderr.write(`docs:build — source directory not found: ${DOCS_DIR}\n`);
  process.exit(1);
}

const docs = walk(DOCS_DIR);

if (docs.length === 0) {
  process.stderr.write(`docs:build — no documents found under ${DOCS_DIR}\n`);
  process.exit(1);
}

const outDir = resolve(OUT_FILE, '..');
if (!existsSync(outDir)) mkdirSync(outDir, { recursive: true });

writeFileSync(OUT_FILE, JSON.stringify(docs));

process.stdout.write(
  `✓ docs:build — ${docs.length} documents → public/assets/docs/search-index.json\n`,
);
