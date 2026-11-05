// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT
import DOMPurify from 'isomorphic-dompurify';
import { RICH_TEXT_SANITIZE_CONFIG } from '~/utils/rich-text';

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

  const escapeHtml = (text: string): string =>
    text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

  /**
   * Sanitizes author-supplied rich text and returns a safe HTML string for use
   * with v-html, keeping only RICH_TEXT_SANITIZE_CONFIG's formatting markup.
   * If the input is plain text (no HTML tags), converts newlines to paragraph
   * and line-break elements so whitespace is preserved when rendered.
   */
  const renderDescription = (raw: string): string => {
    if (hasHtmlTags(raw)) {
      return DOMPurify.sanitize(raw, RICH_TEXT_SANITIZE_CONFIG);
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
    return DOMPurify.sanitize(html || raw, RICH_TEXT_SANITIZE_CONFIG);
  };

  return { sanitize, renderDescription };
};
