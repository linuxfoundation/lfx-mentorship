// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { beforeAll, describe, expect, it, vi } from 'vitest';
import Delta from 'quill-delta';
import {
  clipboardHtmlLooksRich,
  configureQuillMarkdownEditor,
  decodeMarkedEntities,
  looksLikeMarkdown,
  markdownToDelta,
  markdownToHtml,
  matchMarkdownBlockTrigger,
  trimTrailingUnformattedNewline,
} from './quill-markdown-paste';

const ops = (markdown: string) => markdownToDelta(markdown).ops;

describe('markdownToDelta', () => {
  it('returns an empty delta for empty input', () => {
    expect(ops('')).toEqual([]);
  });

  it('converts a heading', () => {
    expect(ops('## Title')).toEqual([
      { insert: 'Title' },
      { insert: '\n', attributes: { header: 2 } },
    ]);
  });

  it('converts inline formatting in a paragraph', () => {
    const result = ops('**bold** *em* `a<b` ~~gone~~ \\*literal');
    expect(result).toContainEqual({ insert: 'bold', attributes: { bold: true } });
    expect(result).toContainEqual({ insert: 'em', attributes: { italic: true } });
    expect(result).toContainEqual({ insert: 'a<b', attributes: { code: true } });
    expect(result).toContainEqual({ insert: 'gone', attributes: { strike: true } });
    expect(result).toContainEqual({ insert: '*' });
    expect(result.at(-1)).toEqual({ insert: '\n' });
  });

  it('keeps safe links and drops unsafe ones', () => {
    const result = ops(
      '[site](https://example.org) [mail](mailto:a@b.org) [rel](/programs) [bad](javascript:alert(1))',
    );
    expect(result).toContainEqual({ insert: 'site', attributes: { link: 'https://example.org' } });
    expect(result).toContainEqual({ insert: 'mail', attributes: { link: 'mailto:a@b.org' } });
    expect(result).toContainEqual({ insert: 'rel', attributes: { link: '/programs' } });
    expect(result).toContainEqual({ insert: 'bad' });
  });

  it('keeps an image as its alt text and inline HTML as plain text', () => {
    const result = ops('![diagram](https://example.org/d.png) <b>hi</b>');
    expect(result).toContainEqual({ insert: 'diagram' });
    expect(result.some((op) => typeof op.insert === 'string' && op.insert.includes('<'))).toBe(
      false,
    );
  });

  it('turns a soft line break into a newline', () => {
    expect(ops('one\ntwo')).toEqual([
      { insert: 'one' },
      { insert: '\n' },
      { insert: 'two' },
      { insert: '\n' },
    ]);
  });

  it('converts bullet, ordered, and nested lists', () => {
    expect(ops('- a\n- b\n  - c')).toEqual([
      { insert: 'a' },
      { insert: '\n', attributes: { list: 'bullet' } },
      { insert: 'b' },
      { insert: '\n', attributes: { list: 'bullet' } },
      { insert: 'c' },
      { insert: '\n', attributes: { list: 'bullet', indent: 1 } },
    ]);
    expect(ops('1. first')).toEqual([
      { insert: 'first' },
      { insert: '\n', attributes: { list: 'ordered' } },
    ]);
  });

  it('converts a heading inside a list item', () => {
    expect(ops('- # Big')).toEqual([
      { insert: 'Big' },
      { insert: '\n', attributes: { list: 'bullet' } },
    ]);
  });

  it('applies blockquote to every quoted line', () => {
    expect(ops('> quoted')).toEqual([
      { insert: 'quoted' },
      { insert: '\n', attributes: { blockquote: true } },
    ]);
  });

  it('converts a fenced code block', () => {
    expect(ops('```\nlet x = 1\n```')).toEqual([
      { insert: 'let x = 1\n', attributes: { 'code-block': true } },
    ]);
  });

  it('strips tags from an HTML block and drops scripts', () => {
    expect(ops('<div>hello</div>')).toEqual([{ insert: 'hello' }, { insert: '\n' }]);
    expect(ops('<script>alert(1)</script>')).toEqual([]);
  });
});

describe('matchMarkdownBlockTrigger', () => {
  it.each([
    ['#', { deleteCount: 1, format: 'header', value: 1 }],
    ['######', { deleteCount: 6, format: 'header', value: 6 }],
    ['-', { deleteCount: 1, format: 'list', value: 'bullet' }],
    ['*', { deleteCount: 1, format: 'list', value: 'bullet' }],
    ['+', { deleteCount: 1, format: 'list', value: 'bullet' }],
    ['12.', { deleteCount: 3, format: 'list', value: 'ordered' }],
    ['>', { deleteCount: 1, format: 'blockquote', value: true }],
    ['#######', null],
    ['text', null],
    ['', null],
  ])('%j', (prefix, want) => {
    expect(matchMarkdownBlockTrigger(prefix)).toEqual(want);
  });

  it('treats a non-breaking space like a space', () => {
    expect(matchMarkdownBlockTrigger('\u00a0')).toBeNull();
  });
});

describe('detection and sanitizing helpers', () => {
  it.each([
    ['# Heading', true],
    ['- item', true],
    ['1. item', true],
    ['> quote', true],
    ['```', true],
    ['some **bold** text', true],
    ['a [link](https://x.org)', true],
    ['plain sentence', false],
    ['   ', false],
  ])('looksLikeMarkdown(%j) = %s', (text, want) => {
    expect(looksLikeMarkdown(text)).toBe(want);
  });

  it('recognises rich clipboard HTML', () => {
    expect(clipboardHtmlLooksRich('<p><strong>x</strong></p>')).toBe(true);
    expect(clipboardHtmlLooksRich('<span>x</span>')).toBe(false);
    expect(clipboardHtmlLooksRich('')).toBe(false);
  });

  it('sanitizes rendered Markdown', () => {
    const html = markdownToHtml('**ok** <img src=x onerror=alert(1)> <script>alert(1)</script>');
    expect(html).toContain('<strong>ok</strong>');
    expect(html).not.toMatch(/<img|<script|onerror/);
  });

  it('decodes the entities marked escapes', () => {
    expect(decodeMarkedEntities('&amp;&lt;&gt;&quot;&#39;&copy;')).toBe(`&<>"'&copy;`);
    expect(decodeMarkedEntities('')).toBe('');
  });

  it('trims only an unformatted trailing newline', () => {
    expect(trimTrailingUnformattedNewline(new Delta([{ insert: 'a\n' }])).ops).toEqual([
      { insert: 'a' },
    ]);
    const formatted = new Delta([{ insert: 'a' }, { insert: '\n', attributes: { header: 1 } }]);
    expect(trimTrailingUnformattedNewline(formatted)).toBe(formatted);
    expect(trimTrailingUnformattedNewline(new Delta()).ops).toEqual([]);
  });
});

// ── editor wiring ────────────────────────────────────────────────────────────

type Range = { index: number; length: number };

/** A minimal Quill stand-in that records the handlers the module registers. */
function fakeQuill(doc: string) {
  let matcher: ((node: Node, delta: Delta) => Delta) | undefined;
  let spaceHandler: ((range: Range) => boolean) | undefined;
  let textChange: ((delta: Delta, old: Delta, source: string) => void) | undefined;
  let paste: ((event: ClipboardEvent) => void) | undefined;
  const quill = {
    clipboard: { addMatcher: (_type: number, fn: typeof matcher) => (matcher = fn) },
    keyboard: {
      addBinding: (_key: unknown, _ctx: unknown, fn: typeof spaceHandler) => (spaceHandler = fn),
    },
    root: {
      addEventListener: (_type: string, fn: typeof paste) => (paste = fn),
    } as unknown as HTMLElement,
    getLength: () => doc.length + 1,
    getLine: vi.fn((index: number): [unknown, number] => [{}, index]),
    getText: (index: number, length: number) => doc.slice(index, index + length),
    getSelection: vi.fn((): Range | null => ({ index: doc.length, length: 0 })),
    deleteText: vi.fn(),
    formatLine: vi.fn(),
    setSelection: vi.fn(),
    updateContents: vi.fn(),
    on: (_event: string, fn: typeof textChange) => (textChange = fn),
  };
  return {
    quill,
    matcher: (delta: Delta) => matcher!({} as Node, delta),
    space: (index: number) => spaceHandler!({ index, length: 0 }),
    type: (delta: Delta, source = 'user') => textChange!(delta, new Delta(), source),
    paste: (text: string, html = '') => {
      const event = {
        clipboardData: { getData: (kind: string) => (kind === 'text/plain' ? text : html) },
        preventDefault: vi.fn(),
        stopPropagation: vi.fn(),
      };
      paste!(event as unknown as ClipboardEvent);
      return event;
    },
  };
}

describe('configureQuillMarkdownEditor', () => {
  beforeAll(() => {
    vi.stubGlobal('Node', { TEXT_NODE: 3 });
  });

  it('truncates pasted plain text to the remaining length', () => {
    const editor = fakeQuill('abc');
    configureQuillMarkdownEditor(editor.quill, 5);
    expect(editor.matcher(new Delta([{ insert: 'wxyz' }])).ops).toEqual([{ insert: 'wx' }]);
    expect(editor.matcher(new Delta([{ insert: { image: 'x' } }])).ops).toEqual([
      { insert: { image: 'x' } },
    ]);
  });

  it('drops pasted text when the editor is full', () => {
    const editor = fakeQuill('abcde');
    configureQuillMarkdownEditor(editor.quill, 5);
    expect(editor.matcher(new Delta([{ insert: 'x' }])).ops).toEqual([]);
  });

  it('formats a line when a block trigger is followed by a space', () => {
    const editor = fakeQuill('##');
    configureQuillMarkdownEditor(editor.quill, 100);
    expect(editor.space(2)).toBe(false);
    expect(editor.quill.deleteText).toHaveBeenCalledWith(0, 2, 'user');
    expect(editor.quill.formatLine).toHaveBeenCalledWith(0, 1, 'header', 2, 'user');
  });

  it('lets a space through when the line has no block trigger', () => {
    const editor = fakeQuill('hello');
    configureQuillMarkdownEditor(editor.quill, 100);
    expect(editor.space(5)).toBe(true);
    expect(editor.space(0)).toBe(true);
    expect(editor.quill.formatLine).not.toHaveBeenCalled();
  });

  it.each([
    ['**bold** ', 'bold', 'bold'],
    ['__bold__ ', 'bold', 'bold'],
    ['*it* ', 'it', 'italic'],
    ['_it_ ', 'it', 'italic'],
    ['~~gone~~ ', 'gone', 'strike'],
    ['`code` ', 'code', 'code'],
  ])('applies the inline shortcut in %j', (doc, content, format) => {
    const editor = fakeQuill(doc);
    configureQuillMarkdownEditor(editor.quill, 100);
    editor.type(new Delta().retain(doc.length - 1).insert(' '));
    expect(editor.quill.updateContents).toHaveBeenCalledWith(
      new Delta()
        .delete(doc.length)
        .insert(content, { [format]: true })
        .insert(' '),
      'user',
    );
  });

  it('ignores an italic marker that is part of a longer run', () => {
    const editor = fakeQuill('***x* ');
    configureQuillMarkdownEditor(editor.quill, 100);
    editor.type(new Delta().retain(5).insert(' '));
    expect(editor.quill.updateContents).not.toHaveBeenCalled();
  });

  it('ignores API edits and inserts that do not end in a space', () => {
    const editor = fakeQuill('**bold** ');
    configureQuillMarkdownEditor(editor.quill, 100);
    editor.type(new Delta().retain(8).insert(' '), 'api');
    editor.type(new Delta().retain(8).insert('x'));
    expect(editor.quill.updateContents).not.toHaveBeenCalled();
  });

  it('converts pasted Markdown into rich text', () => {
    const editor = fakeQuill('');
    configureQuillMarkdownEditor(editor.quill, 100);
    const event = editor.paste('**hi**');
    expect(event.preventDefault).toHaveBeenCalled();
    expect(editor.quill.updateContents).toHaveBeenCalledWith(
      new Delta().insert('hi', { bold: true }),
      'user',
    );
  });

  it('leaves plain text and rich HTML pastes to Quill', () => {
    const editor = fakeQuill('');
    configureQuillMarkdownEditor(editor.quill, 100);
    expect(editor.paste('just words').preventDefault).not.toHaveBeenCalled();
    expect(editor.paste('**hi**', '<strong>hi</strong>').preventDefault).not.toHaveBeenCalled();
    expect(editor.quill.updateContents).not.toHaveBeenCalled();
  });

  it('swallows a Markdown paste when the editor is full', () => {
    const editor = fakeQuill('abcde');
    configureQuillMarkdownEditor(editor.quill, 5);
    expect(editor.paste('**hi**').preventDefault).toHaveBeenCalled();
    expect(editor.quill.updateContents).not.toHaveBeenCalled();
  });
});
