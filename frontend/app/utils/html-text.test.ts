// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { describe, expect, it } from 'vitest';
import { plainTextFromHtml, plainTextLength } from './html-text';

describe('plainTextFromHtml', () => {
  it('turns tags into separators and decodes entities once', () => {
    expect(plainTextFromHtml('<p>Learn <b>Go</b></p><p>&amp;lt;tag&amp;gt; &nbsp;x</p>')).toBe(
      'Learn Go &lt;tag&gt; x',
    );
    expect(plainTextLength('<p>a</p><p>b</p>')).toBe(3);
  });

  it('keeps text after a `<` that is never closed', () => {
    expect(plainTextFromHtml('a <i>b</i> < c')).toBe('a b < c');
    expect(plainTextFromHtml('x <y')).toBe('x <y');
  });

  it('strips exactly what the former /<[^>]*>/g pattern stripped', () => {
    const strip = (html: string) =>
      html
        .replace(/<[^>]*>/g, ' ')
        .replace(/\s+/g, ' ')
        .trim();
    const alphabet = ['<', '>', 'a', ' ', '<p>', '</p>'];
    let seed = 7;
    const next = () => (seed = (seed * 1103515245 + 12345) % 2 ** 31);
    for (let i = 0; i < 500; i++) {
      const html = Array.from(
        { length: next() % 30 },
        () => alphabet[next() % alphabet.length],
      ).join('');
      expect(plainTextFromHtml(html)).toBe(strip(html));
    }
  });

  it.each([
    ['repeated <', '<'.repeat(1_000_000)],
    ['repeated <a', '<a'.repeat(500_000)],
    ['one < then text', `<${'a'.repeat(1_000_000)}`],
  ])('runs in linear time on 1 MB of %s with no closing >', (_name, html) => {
    const start = performance.now();
    expect(plainTextFromHtml(html)).toBe(html);
    expect(performance.now() - start).toBeLessThan(1000);
  });
});
