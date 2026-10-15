// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { describe, expect, it } from 'vitest';
import { useSanitize } from './useSanitize';

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
});
