// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { defineEventHandler, getRequestURL, sendRedirect } from 'h3';

// The legacy host (mentorship.lfx.linuxfoundation.org) redirects here keeping the path,
// so project websites, docs and old emails still arrive with the legacy app's URL shapes.
// Program ids are kept by the import, and both sites key mentees and mentors by user id.
const RE_LEGACY_PROGRAM = /^\/project\/([^/]+)(?:\/.*)?$/;
// Legacy mentee links can carry the program as a ",{projectId}" suffix.
const RE_LEGACY_MENTEE = /^\/mentee\/([^/,]+?)(?:(?:,|%2C)[^/]*)?\/?$/i;
const RE_LEGACY_MENTOR = /^\/mentor\/([^/]+)\/?$/;
// Signed-in flows with no page on the public site; their tokens are legacy-only, so the query is dropped.
const RE_LEGACY_FLOW =
  /^\/(?:project\/applied|mentee\/(?:applications|tasks)|participate|profile|email)(?:\/.*)?$/;

export default defineEventHandler((event) => {
  const method = event.method.toUpperCase();
  if (method !== 'GET' && method !== 'HEAD') return;

  const { pathname, search } = getRequestURL(event);
  // Checked first: /project/applied and /mentee/tasks would otherwise read as ids.
  if (RE_LEGACY_FLOW.test(pathname)) return sendRedirect(event, '/', 301);

  const program = RE_LEGACY_PROGRAM.exec(pathname);
  if (program) return sendRedirect(event, `/programs/${program[1]}${search}`, 301);
  const mentee = RE_LEGACY_MENTEE.exec(pathname);
  if (mentee) return sendRedirect(event, `/mentees/${mentee[1]}${search}`, 301);
  const mentor = RE_LEGACY_MENTOR.exec(pathname);
  if (mentor) return sendRedirect(event, `/mentors/${mentor[1]}${search}`, 301);
});
