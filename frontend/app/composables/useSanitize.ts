// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT
import DOMPurify from 'isomorphic-dompurify';

/**
 * Whether `raw` contains a `<` + letter with a `>` somewhere after it — what
 * /<[a-z][\s\S]*>/i matches, found in linear time. That regex backtracks
 * quadratically on input with many `<a` and no `>`. Checking the first `<` +
 * letter is enough: a `>` after any later one is also after the first.
 */
export const hasHtmlTags = (raw: string): boolean => {
  const firstTag = raw.search(/<[a-z]/i);
  return firstTag !== -1 && raw.indexOf('>', firstTag) !== -1;
};

export const useSanitize = () => {
  const sanitize = (dirty: string) => DOMPurify.sanitize(dirty);

  /**
   * Sanitizes a description and returns a safe HTML string for use with v-html.
   * If the input is plain text (no HTML tags), converts newlines to paragraph
   * and line-break elements so whitespace is preserved when rendered.
   */
  const escapeHtml = (text: string): string =>
    text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

  const renderDescription = (raw: string): string => {
    if (hasHtmlTags(raw)) {
      return DOMPurify.sanitize(raw);
    }
    // Plain text: normalize line endings (CRLF/CR → LF), split blank lines into
    // paragraphs, single newlines become <br>. Escape HTML-sensitive characters
    // before wrapping so they survive the HTML parser before DOMPurify runs.
    const html = raw
      .replace(/\r\n?/g, '\n')
      .split(/\n\n+/)
      .filter((p) => p.trim())
      .map((p) => `<p>${escapeHtml(p).replace(/\n/g, '<br>')}</p>`)
      .join('');
    return DOMPurify.sanitize(html || raw);
  };

  return { sanitize, renderDescription };
};
