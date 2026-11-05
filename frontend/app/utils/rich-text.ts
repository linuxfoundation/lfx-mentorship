// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

/**
 * DOMPurify config for author-supplied rich text: program descriptions and
 * mentor and mentee introductions. It allows only the formatting the LFX One
 * and legacy editors produce. Forms, inputs, `<style>`, images, SVG/MathML and
 * the `class`, `style`, `id` and `name` attributes are removed, so a stored
 * description cannot restyle the page, overlay it with a form, or load
 * third-party images. DOMPurify's default URI check still blocks
 * `javascript:` and `data:` hrefs.
 */
export const RICH_TEXT_SANITIZE_CONFIG = {
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
};
