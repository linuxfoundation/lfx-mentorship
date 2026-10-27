// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { readFile } from 'node:fs/promises';
import { createError, type H3Event } from 'h3';
import { marked } from 'marked';
import DOMPurify from 'isomorphic-dompurify';
import { beforeEach, describe, expect, it, vi } from 'vitest';

type Handler = (event: H3Event) => unknown;
type FakeEvent = { params?: Record<string, string> };

const APP_URL = 'https://mentorship.linuxfoundation.org';

vi.stubGlobal('createError', createError);
vi.stubGlobal('defineEventHandler', (fn: Handler) => fn);
vi.stubGlobal('getRouterParam', (event: FakeEvent, name: string) => event.params?.[name]);
vi.stubGlobal('useRuntimeConfig', () => ({ public: { appUrl: APP_URL } }));

// Spy on the real fs so "rejected before any disk access" is an assertion, not a comment.
vi.mock('node:fs/promises', async (importOriginal) => {
  const actual = await importOriginal<typeof import('node:fs/promises')>();
  return { ...actual, readFile: vi.fn(actual.readFile) };
});

const load = async (path: string) => (await import(path)).default as Handler;
const call = (handler: Handler, slug?: string) =>
  handler({ params: slug === undefined ? {} : { slug } } as unknown as H3Event);

// Renders through the same pipeline the handler uses, so a renderer regression fails here.
const render = async (markdown: string, slugDir = 'mentees') => {
  const { buildRenderer } = await import('./[...slug].get');
  const html = await marked.parse(markdown, {
    renderer: buildRenderer(slugDir, APP_URL),
    async: true,
  });
  return DOMPurify.sanitize(html, { ADD_ATTR: ['target'] });
};

beforeEach(() => {
  vi.mocked(readFile).mockClear();
});

describe('docs article handler', () => {
  it('renders a real article from docs/user', async () => {
    const article = (await call(await load('./[...slug].get'), 'getting-started')) as Record<
      string,
      unknown
    >;

    expect(article.slug).toBe('getting-started');
    expect(article.title).toBeTruthy();
    expect(String(article.bodyHtml).length).toBeGreaterThan(0);
    expect(article.intercomCollection).toBe('LFX Mentorship');
  });

  it('resolves a nested article through its index.md', async () => {
    const article = (await call(
      await load('./[...slug].get'),
      'mentees/apply-to-a-program',
    )) as Record<string, unknown>;
    expect(article.slug).toBe('mentees/apply-to-a-program');
  });

  it('rejects a missing slug with 400', async () => {
    await expect(call(await load('./[...slug].get'))).rejects.toMatchObject({ statusCode: 400 });
  });

  it.each(['../../package', '../../../etc/passwd', 'mentees/../../../secrets'])(
    'rejects traversal %s with 400 before touching the disk',
    async (slug) => {
      await expect(call(await load('./[...slug].get'), slug)).rejects.toMatchObject({
        statusCode: 400,
      });
      expect(readFile).not.toHaveBeenCalled();
    },
  );

  it('returns 404 for an article that does not exist', async () => {
    await expect(call(await load('./[...slug].get'), 'mentees/no-such-page')).rejects.toMatchObject(
      {
        statusCode: 404,
      },
    );
  });
});

// marked is pinned to v4, whose renderer hooks take positional arguments. Written against
// the v5+ options-object shape these silently produce `undefined` link text and empty
// images, and nothing else in the build catches it — hence these tests.
describe('markdown renderer (marked v4 hooks)', () => {
  it('keeps link text and rewrites a sibling doc link', async () => {
    const html = await render('[Apply to a Program](./apply-to-a-program/)');
    expect(html).toContain('>Apply to a Program</a>');
    expect(html).toContain('href="/docs/mentees/apply-to-a-program"');
    expect(html).not.toContain('undefined');
  });

  it('walks up for a ../ link', async () => {
    const html = await render('[Programs](../programs/)', 'mentees/apply-to-a-program');
    expect(html).toContain('href="/docs/mentees/programs"');
  });

  it('strips the .md and /index.md suffixes', async () => {
    expect(await render('[A](./history.md)')).toContain('href="/docs/mentees/history"');
    expect(await render('[B](./nested/index.md)')).toContain('href="/docs/mentees/nested"');
  });

  it('leaves anchors and root-relative links alone', async () => {
    expect(await render('[A](#section)')).toContain('href="#section"');
    expect(await render('[B](/programs)')).toContain('href="/programs"');
  });

  it('opens external links in a new tab with a safe rel', async () => {
    const html = await render('[LF](https://linuxfoundation.org)');
    expect(html).toContain('target="_blank"');
    expect(html).toContain('rel="noopener noreferrer"');
  });

  it('makes a root-relative image absolute and keeps its alt text', async () => {
    const html = await render(
      '![Browse programs](/images/docs/screenshots/programs/programs-browse.png)',
    );
    expect(html).toContain(`src="${APP_URL}/images/docs/screenshots/programs/programs-browse.png"`);
    expect(html).toContain('alt="Browse programs"');
  });

  it('leaves an already-absolute image URL untouched', async () => {
    const html = await render('![X](https://cdn.example.com/x.png)');
    expect(html).toContain('src="https://cdn.example.com/x.png"');
  });

  it('escapes quotes in a link title rather than breaking out of the attribute', async () => {
    const html = await render('[A](/x "a \\" title")');
    expect(html).not.toContain('title="a " title"');
  });

  it('sanitizes embedded HTML', async () => {
    const html = await render('Hello <script>alert(1)</script> world');
    expect(html).not.toContain('<script>');
  });

  it('strips an onerror handler from an inline image', async () => {
    const html = await render('<img src="x" onerror="alert(1)">');
    expect(html).not.toContain('onerror');
  });
});
