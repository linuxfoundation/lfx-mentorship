// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

/**
 * Converts rich-text HTML to plain text for text-only contexts (card previews,
 * meta descriptions, editor counters). Tags become spaces so adjacent blocks
 * stay separated, and common entities are decoded — `&amp;` last, so escaped
 * entities such as `&amp;lt;` are not decoded twice.
 */
export function plainTextFromHtml(html: string): string {
  return html
    .replace(/<[^>]*>/g, ' ')
    .replace(/&nbsp;/gi, ' ')
    .replace(/&lt;/gi, '<')
    .replace(/&gt;/gi, '>')
    .replace(/&quot;/gi, '"')
    .replace(/&amp;/gi, '&')
    .replace(/\s+/g, ' ')
    .trim();
}

export function plainTextLength(html: string): number {
  return plainTextFromHtml(html).length;
}
