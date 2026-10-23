// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

/**
 * Converts rich-text HTML to plain text for text-only contexts (card previews,
 * meta descriptions, editor counters). Tags become spaces so adjacent blocks
 * stay separated, and common entities are decoded — `&amp;` last, so escaped
 * entities such as `&amp;lt;` are not decoded twice.
 */
export function plainTextFromHtml(html: string): string {
  return stripTags(html)
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

/**
 * Replaces each `<…>` run with a space, as `/<[^>]*>/g` would, in one linear pass. The regex
 * re-scans the rest of the input from every `<` that has no closing `>`, which is quadratic on
 * stored text such as a long run of `<`. Once a `<` has no `>` after it, no later `<` can either,
 * so the remainder is kept as text.
 */
function stripTags(html: string): string {
  let out = '';
  let from = 0;
  for (let open = html.indexOf('<'); open !== -1; open = html.indexOf('<', from)) {
    const close = html.indexOf('>', open + 1);
    if (close === -1) break;
    out += html.slice(from, open) + ' ';
    from = close + 1;
  }
  return out + html.slice(from);
}
