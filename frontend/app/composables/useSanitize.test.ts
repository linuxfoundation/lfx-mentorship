// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { describe, expect, it } from 'vitest';
import { hasHtmlTags, useSanitize } from './useSanitize';

const { sanitize, renderDescription } = useSanitize();

describe('useSanitize', () => {
  it('strips script and event handlers', () => {
    expect(sanitize('<p onclick="x()">hi</p><script>alert(1)</script>')).toBe('<p>hi</p>');
  });

  it('sanitizes HTML descriptions as they are', () => {
    expect(renderDescription('<p>a</p><img src=x onerror=alert(1)>')).toBe('<p>a</p><img src="x">');
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
