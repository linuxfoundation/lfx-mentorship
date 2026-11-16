// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT
import { clearWindow, sanitize, type Config } from 'isomorphic-dompurify';

/**
 * How many sanitize calls share one jsdom window on the server before it is
 * replaced. Each replacement costs a few milliseconds; the window holds on to
 * every parsed element that has an `id` (see sanitizeHtml), so this bounds
 * that memory to the last few calls' worth of documents.
 */
export const SANITIZE_CALLS_PER_WINDOW = 25;

let callsOnWindow = 0;

/**
 * DOMPurify.sanitize, safe for a long-running server.
 *
 * On the server, isomorphic-dompurify parses into one shared jsdom window, and
 * jsdom registers every parsed element with an `id` (or a `name` on img, form,
 * embed or object) as a named property of that window, then never releases it:
 * the registration is only undone when the element is detached, and a parsed
 * document is simply dropped. Each such element keeps its whole document alive,
 * so sanitizing Markdown output (marked gives every heading an id) leaked the
 * entire article on every request until the Nitro server ran out of heap. This
 * happens while parsing, so stripping `id` in the config does not prevent it.
 * Replacing the window lets the old one, and everything registered on it, be
 * garbage collected. In the browser, clearWindow() is a no-op.
 */
export function sanitizeHtml(dirty: string, config: Config = {}): string {
  const clean = sanitize(dirty, config);
  callsOnWindow += 1;
  if (callsOnWindow >= SANITIZE_CALLS_PER_WINDOW) {
    callsOnWindow = 0;
    clearWindow();
  }
  return clean;
}
