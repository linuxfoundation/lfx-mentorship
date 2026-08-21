// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

/**
 * Markdown input helpers for Quill editors (paste + typing shortcuts).
 * Ported from lfx-mentorship-upgrade for Quill 2 / Nuxt 4.
 * Storage remains Quill HTML — Markdown is converted to Delta on input only.
 */
import DOMPurify from 'isomorphic-dompurify';
import { marked } from 'marked';
import Delta from 'quill-delta';
import type { Op } from 'quill-delta';

type Token = marked.Token;

marked.setOptions({
  gfm: true,
  breaks: true,
  headerIds: false,
  mangle: false,
});

const DOMPURIFY_CONFIG = {
  ALLOWED_TAGS: [
    'h1',
    'h2',
    'h3',
    'h4',
    'h5',
    'h6',
    'p',
    'br',
    'ul',
    'ol',
    'li',
    'strong',
    'b',
    'em',
    'i',
    'u',
    's',
    'del',
    'blockquote',
    'pre',
    'code',
    'a',
    'span',
  ],
  ALLOWED_ATTR: ['href', 'target', 'rel'],
  ALLOW_DATA_ATTR: false,
  FORBID_TAGS: ['script', 'style', 'iframe', 'object', 'embed', 'form', 'input'],
  FORBID_CONTENTS: ['script', 'style'],
};

type QuillLike = {
  clipboard: { addMatcher: (nodeType: number, matcher: (node: Node, delta: Delta) => Delta) => void };
  keyboard: {
    addBinding: (
      key: { key: number | string },
      context: { collapsed: boolean },
      handler: (range: { index: number; length: number }) => boolean,
    ) => void;
  };
  root: HTMLElement;
  getLength: () => number;
  getLine: (index: number) => [unknown, number];
  getText: (index: number, length: number) => string;
  getSelection: (focus?: boolean) => { index: number; length: number } | null;
  deleteText: (index: number, length: number, source?: string) => void;
  formatLine: (index: number, length: number, format: string, value: string | number | boolean, source?: string) => void;
  setSelection: (index: number, length: number, source?: string) => void;
  updateContents: (delta: Delta, source?: string) => void;
  on: (event: 'text-change', handler: (delta: Delta, old: Delta, source: string) => void) => void;
};

/** Detect common Markdown constructs users paste into the description editor. */
export function looksLikeMarkdown(text: string): boolean {
  if (!text?.trim()) {
    return false;
  }
  return /(?:^|\n)\s{0,3}(?:#{1,6}\s|[-*+]\s|\d+\.\s|>\s|```)|(?:\*\*[^*\n]+\*\*|__[^_\n]+__|(?<!\*)\*[^*\n]+\*(?!\*)|(?<!_)_[^_\n]+_(?!_)|~~[^~\n]+~~|`[^`\n]+`|\[[^\]]+\]\([^)]+\))/.test(
    text,
  );
}

/**
 * True when clipboard HTML already carries rich formatting we should keep
 * (e.g. paste from Word/Google Docs), rather than treating plain text as Markdown.
 */
export function clipboardHtmlLooksRich(html: string): boolean {
  if (!html?.trim()) {
    return false;
  }
  return /<(?:h[1-6]|ul|ol|li|strong|b|em|i|blockquote|pre|code|a|table|tr|td)\b/i.test(html);
}

/** Converts Markdown to sanitized HTML (inspection / display helpers). */
export function markdownToHtml(markdown: string): string {
  const html = marked.parse(markdown, { async: false }) as string;
  return sanitizeQuillHtml(html);
}

export function sanitizeQuillHtml(html: string): string {
  if (!html) {
    return '';
  }
  return DOMPurify.sanitize(html, DOMPURIFY_CONFIG);
}

/** Converts Markdown to a Quill Delta without assigning user HTML to the DOM. */
export function markdownToDelta(markdown: string): Delta {
  const tokens = marked.lexer(markdown || '');
  const ops: Op[] = [];
  appendBlockTokens(tokens, ops);

  if (!ops.length) {
    return new Delta();
  }

  const last = ops[ops.length - 1];
  if (last && typeof last.insert === 'string' && !last.insert.endsWith('\n')) {
    ops.push({ insert: '\n' });
  }

  return new Delta(ops);
}

export interface MarkdownBlockMatch {
  deleteCount: number;
  format: string;
  value: string | number | boolean;
}

/** Matches line-start Markdown block triggers typed before Space (e.g. "##" → header 2). */
export function matchMarkdownBlockTrigger(linePrefix: string): MarkdownBlockMatch | null {
  const text = linePrefix.replace(/\u00a0/g, ' ');

  const header = text.match(/^(#{1,6})$/);
  if (header) {
    return { deleteCount: header[1]?.length || 0, format: 'header', value: header[1]?.length || 0 };
  }

  if (text === '-' || text === '*' || text === '+') {
    return { deleteCount: 1, format: 'list', value: 'bullet' };
  }

  if (/^\d+\.$/.test(text)) {
    return { deleteCount: text.length, format: 'list', value: 'ordered' };
  }

  if (text === '>') {
    return { deleteCount: 1, format: 'blockquote', value: true };
  }

  return null;
}

/**
 * Configures a Quill editor for Markdown-friendly input:
 * - enforces max character length on paste
 * - converts pasted Markdown into Quill rich text when appropriate
 * - converts typed Markdown shortcuts (e.g. "## " → Heading 2)
 */
export function configureQuillMarkdownEditor(quill: QuillLike, maxLength: number): void {
  quill.clipboard.addMatcher(Node.TEXT_NODE, (_node, delta) => {
    const remaining = maxLength - (quill.getLength() - 1);

    if (remaining <= 0) {
      return new Delta();
    }

    const ops = (delta.ops || []).map((op) => {
      if (typeof op.insert === 'string') {
        return { ...op, insert: op.insert.substring(0, remaining) };
      }
      return op;
    });

    return new Delta(ops);
  });

  enableMarkdownTypingShortcuts(quill);
  enableMarkdownPaste(quill, maxLength);
}

function appendBlockTokens(tokens: Token[], ops: Op[], lineAttrs: Record<string, unknown> = {}): void {
  for (const token of tokens) {
    switch (token.type) {
      case 'space':
        break;
      case 'heading': {
        const heading = token as marked.Tokens.Heading;
        appendInlineTokens(heading.tokens || [plainTextToken(heading.text)], ops, {});
        ops.push({ insert: '\n', attributes: { header: heading.depth, ...lineAttrs } });
        break;
      }
      case 'paragraph': {
        const paragraph = token as marked.Tokens.Paragraph;
        appendInlineTokens(paragraph.tokens || [plainTextToken(paragraph.text)], ops, {});
        ops.push(newlineOp(lineAttrs));
        break;
      }
      case 'list': {
        const list = token as marked.Tokens.List;
        for (const item of list.items) {
          appendListItem(item, ops, list.ordered ? 'ordered' : 'bullet', lineAttrs);
        }
        break;
      }
      case 'blockquote': {
        const quote = token as marked.Tokens.Blockquote;
        appendBlockTokens(quote.tokens || [], ops, { ...lineAttrs, blockquote: true });
        break;
      }
      case 'code': {
        const code = token as marked.Tokens.Code;
        const text = code.text.endsWith('\n') ? code.text : `${code.text}\n`;
        ops.push({ insert: text, attributes: { 'code-block': true, ...lineAttrs } });
        break;
      }
      case 'html': {
        const html = token as marked.Tokens.HTML;
        const text = stripTags(html.text || '');
        if (text) {
          ops.push({ insert: text });
          if (!text.endsWith('\n')) {
            ops.push(newlineOp(lineAttrs));
          }
        }
        break;
      }
      case 'text': {
        const textToken = token as marked.Tokens.Text;
        appendInlineTokens(textToken.tokens || [textToken], ops, {});
        ops.push(newlineOp(lineAttrs));
        break;
      }
      default:
        break;
    }
  }
}

function appendListItem(
  item: marked.Tokens.ListItem,
  ops: Op[],
  listType: 'ordered' | 'bullet',
  lineAttrs: Record<string, unknown>,
): void {
  const inline = (item.tokens || []).filter((t: Token) => t.type !== 'list');
  const nested = (item.tokens || []).filter((t: Token) => t.type === 'list');

  if (inline.length) {
    for (const token of inline) {
      if (token.type === 'paragraph' || token.type === 'text') {
        const withTokens = token as marked.Tokens.Paragraph | marked.Tokens.Text;
        appendInlineTokens(withTokens.tokens || [plainTextToken(withTokens.text)], ops, {});
      } else if (token.type === 'heading') {
        const heading = token as marked.Tokens.Heading;
        appendInlineTokens(heading.tokens || [plainTextToken(heading.text)], ops, {});
      }
    }
  } else if (item.text) {
    ops.push({ insert: item.text });
  }

  ops.push({ insert: '\n', attributes: { list: listType, ...lineAttrs } });

  for (const nestedList of nested) {
    appendBlockTokens([nestedList], ops, {
      ...lineAttrs,
      indent: (Number(lineAttrs.indent) || 0) + 1,
    });
  }
}

function appendInlineTokens(tokens: Token[], ops: Op[], attrs: Record<string, unknown>): void {
  for (const token of tokens) {
    switch (token.type) {
      case 'text': {
        const text = token as marked.Tokens.Text;
        if (text.tokens?.length) {
          appendInlineTokens(text.tokens, ops, attrs);
        } else if (text.text) {
          ops.push({ insert: decodeMarkedEntities(text.text), attributes: attrsOrUndefined(attrs) });
        }
        break;
      }
      case 'strong': {
        const strong = token as marked.Tokens.Strong;
        appendInlineTokens(strong.tokens || [], ops, { ...attrs, bold: true });
        break;
      }
      case 'em': {
        const em = token as marked.Tokens.Em;
        appendInlineTokens(em.tokens || [], ops, { ...attrs, italic: true });
        break;
      }
      case 'codespan': {
        const code = token as marked.Tokens.Codespan;
        ops.push({
          insert: decodeMarkedEntities(code.text),
          attributes: attrsOrUndefined({ ...attrs, code: true }),
        });
        break;
      }
      case 'del': {
        const del = token as marked.Tokens.Del;
        appendInlineTokens(del.tokens || [], ops, { ...attrs, strike: true });
        break;
      }
      case 'link': {
        const link = token as marked.Tokens.Link;
        const href = sanitizeHref(link.href || '');
        const linkAttrs = href ? { ...attrs, link: href } : attrs;
        appendInlineTokens(link.tokens || [plainTextToken(link.text)], ops, linkAttrs);
        break;
      }
      case 'br':
        ops.push({ insert: '\n', attributes: attrsOrUndefined(attrs) });
        break;
      case 'escape': {
        const escape = token as marked.Tokens.Escape;
        ops.push({ insert: decodeMarkedEntities(escape.text), attributes: attrsOrUndefined(attrs) });
        break;
      }
      case 'html': {
        const html = token as marked.Tokens.HTML;
        const text = stripTags(html.text || '');
        if (text) {
          ops.push({ insert: text, attributes: attrsOrUndefined(attrs) });
        }
        break;
      }
      case 'image': {
        const image = token as marked.Tokens.Image;
        const fallback = image.text || image.raw || '';
        if (fallback) {
          ops.push({ insert: decodeMarkedEntities(fallback), attributes: attrsOrUndefined(attrs) });
        }
        break;
      }
      default:
        break;
    }
  }
}

function plainTextToken(text: string): marked.Tokens.Text {
  return { type: 'text', raw: text, text };
}

function attrsOrUndefined(attrs: Record<string, unknown>): Record<string, unknown> | undefined {
  return Object.keys(attrs).length ? attrs : undefined;
}

function newlineOp(lineAttrs: Record<string, unknown> = {}): Op {
  const attributes = attrsOrUndefined({ ...lineAttrs });
  return attributes ? { insert: '\n', attributes } : { insert: '\n' };
}

function sanitizeHref(href: string): string {
  const value = (href || '').trim();
  const lower = value.toLowerCase();
  if (
    lower.startsWith('http://') ||
    lower.startsWith('https://') ||
    lower.startsWith('mailto:') ||
    lower.startsWith('/')
  ) {
    return value;
  }
  return '';
}

function stripTags(value: string): string {
  return DOMPurify.sanitize(value || '', { ALLOWED_TAGS: [], ALLOWED_ATTR: [] });
}

/** Reverses HTML escaping marked applies to inline token text. */
export function decodeMarkedEntities(text: string): string {
  if (!text) {
    return '';
  }
  return text.replace(/&(?:amp|lt|gt|quot|#39);/g, (match) => {
    switch (match) {
      case '&amp;':
        return '&';
      case '&lt;':
        return '<';
      case '&gt;':
        return '>';
      case '&quot;':
        return '"';
      case '&#39;':
        return "'";
      default:
        return match;
    }
  });
}

function enableMarkdownTypingShortcuts(quill: QuillLike): void {
  quill.keyboard.addBinding({ key: 32 }, { collapsed: true }, (range) => {
    const [line, offset] = quill.getLine(range.index);
    if (!line || offset === 0) {
      return true;
    }

    const lineStart = range.index - offset;
    const linePrefix = quill.getText(lineStart, offset);
    const blockMatch = matchMarkdownBlockTrigger(linePrefix);
    if (!blockMatch) {
      return true;
    }

    quill.deleteText(lineStart, offset, 'user');
    quill.formatLine(lineStart, 1, blockMatch.format, blockMatch.value, 'user');
    quill.setSelection(lineStart, 0, 'silent');
    return false;
  });

  quill.on('text-change', (delta, _old, source) => {
    if (source !== 'user') {
      return;
    }

    const inserted = getInsertedText(delta);
    if (!inserted?.endsWith(' ')) {
      return;
    }

    const cursorIndex = cursorIndexAfterDelta(delta);
    if (cursorIndex <= 0) {
      return;
    }

    applyInlineMarkdownShortcut(quill, cursorIndex);
  });
}

function enableMarkdownPaste(quill: QuillLike, maxLength: number): void {
  quill.root.addEventListener(
    'paste',
    (event: ClipboardEvent) => {
      const clipboardData = event.clipboardData;
      if (!clipboardData) {
        return;
      }

      const text = clipboardData.getData('text/plain');
      const html = clipboardData.getData('text/html');

      if (!text || !looksLikeMarkdown(text)) {
        return;
      }

      if (clipboardHtmlLooksRich(html)) {
        return;
      }

      event.preventDefault();
      event.stopPropagation();

      const selection = quill.getSelection(true) || { index: quill.getLength(), length: 0 };
      const remaining = maxLength - (quill.getLength() - 1 - selection.length);
      if (remaining <= 0) {
        return;
      }

      const pastedDelta = trimTrailingUnformattedNewline(truncateDelta(markdownToDelta(text), remaining));
      const update = new Delta().retain(selection.index).delete(selection.length).concat(pastedDelta);
      quill.updateContents(update, 'user');
      quill.setSelection(selection.index + pastedDelta.length(), 0, 'silent');
    },
    true,
  );
}

export function trimTrailingUnformattedNewline(delta: Delta): Delta {
  if (!delta?.ops?.length) {
    return delta || new Delta();
  }

  const lastOp = delta.ops[delta.ops.length - 1];
  if (lastOp && deltaEndsWith(delta, '\n') && isUnformattedOp(lastOp)) {
    return delta.compose(new Delta().retain(delta.length() - 1).delete(1));
  }

  return delta;
}

function deltaEndsWith(delta: Delta, text: string): boolean {
  if (!delta?.ops?.length) {
    return false;
  }
  const lastOp = delta.ops[delta.ops.length - 1];
  if (!lastOp) {
    return false;
  }
  return typeof lastOp.insert === 'string' && lastOp.insert.endsWith(text);
}

function isUnformattedOp(op: Op): boolean {
  return !op.attributes || Object.keys(op.attributes).length === 0;
}

function getInsertedText(delta: Delta): string {
  if (!delta?.ops) {
    return '';
  }
  return delta.ops
    .filter((op) => typeof op.insert === 'string')
    .map((op) => op.insert as string)
    .join('');
}

function cursorIndexAfterDelta(delta: Delta): number {
  if (!delta?.ops) {
    return -1;
  }
  let index = 0;
  for (const op of delta.ops) {
    if (typeof op.retain === 'number') {
      index += op.retain;
    }
    if (typeof op.insert === 'string') {
      index += op.insert.length;
    } else if (op.insert) {
      index += 1;
    }
  }
  return index;
}

function applyInlineMarkdownShortcut(quill: QuillLike, cursorIndex: number): void {
  const lookbehind = Math.min(cursorIndex, 200);
  const start = cursorIndex - lookbehind;
  const text = quill.getText(start, lookbehind);

  const patterns: Array<{ regex: RegExp; format: string }> = [
    { regex: /\*\*(.+?)\*\* $/, format: 'bold' },
    { regex: /__(.+?)__ $/, format: 'bold' },
    { regex: /\*([^*\n]+)\* $/, format: 'italic' },
    { regex: /_([^_\n]+)_ $/, format: 'italic' },
    { regex: /~~(.+?)~~ $/, format: 'strike' },
    { regex: /`([^`\n]+)` $/, format: 'code' },
  ];

  for (const { regex, format } of patterns) {
    const match = text.match(regex);
    if (!match || match.index === undefined) {
      continue;
    }

    const matched = match[0];
    const content = match[1];
    const matchedStart = start + match.index;

    if (format === 'italic') {
      const prev = matchedStart > start ? text.charAt(match.index - 1) : '';
      if (prev === '*' || prev === '_') {
        continue;
      }
    }

    const replacement = new Delta()
      .retain(matchedStart)
      .delete(matched.length)
      .insert(content || '', { [format]: true })
      .insert(' ');
    quill.updateContents(replacement, 'user');
    quill.setSelection(matchedStart + (content?.length || 0) + 1, 0, 'silent');
    return;
  }
}

function truncateDelta(delta: Delta, maxChars: number): Delta {
  let remaining = maxChars;
  const ops: Op[] = [];

  for (const op of delta.ops || []) {
    if (remaining <= 0) {
      break;
    }
    if (typeof op.insert === 'string') {
      const insert = op.insert.substring(0, remaining);
      remaining -= insert.length;
      ops.push({ ...op, insert });
    } else if (op.insert) {
      remaining -= 1;
      ops.push(op);
    }
  }

  return new Delta(ops);
}
