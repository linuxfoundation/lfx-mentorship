// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { defineEventHandler, setResponseHeader } from 'h3';

// Defence in depth behind the rich-text sanitizer: even if injected markup reaches the page, it
// cannot submit a form off-origin, rebase relative URLs, or load a plugin. The site has no forms of
// its own. Script, style and image sources are not restricted here; the page loads them from
// several third-party and per-environment CDN hosts.
export const CONTENT_SECURITY_POLICY = "form-action 'self'; base-uri 'self'; object-src 'none'";

export default defineEventHandler((event) => {
  setResponseHeader(event, 'Content-Security-Policy', CONTENT_SECURITY_POLICY);
});
