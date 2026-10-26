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

// Signed-in flows that legacy emails and bookmarks link to and that Self Serve now serves. They are
// checked before RE_LEGACY_FLOW, which sends the rest home, and drop the query for the same reason.
const RE_LEGACY_APPROVAL = /^\/email\/process-approval\/?$/;
// The pathname is already percent-encoded, so the captured program id is safe to forward as is.
const RE_LEGACY_PROGRAM_MENTEES = /^\/project\/([^/]+)\/mentees\/?$/;
const SELF_SERVE_FLOWS: ReadonlyArray<readonly [RegExp, string]> = [
  // Includes the old invitation links, /participate/mentor/join|decline/{id}.
  [/^\/participate\/mentor(?:\/.*)?$/, '/mentorship/mentor'],
  [/^\/profile\/mentor\/edit\/?$/, '/mentorship/mentor/profile'],
  [/^\/mentee\/tasks\/?$/, '/mentorship/mentee/tasks'],
  [/^\/(?:mentee\/applications|project\/applied)\/?$/, '/mentorship/mentee'],
];

// The Self Serve path a legacy signed-in link maps to, or undefined when it has none.
function selfServePath(pathname: string, searchParams: URLSearchParams): string | undefined {
  if (RE_LEGACY_APPROVAL.test(pathname)) {
    const programId = searchParams.get('id');
    return programId ? `/mentorship/program-review/${encodeURIComponent(programId)}` : undefined;
  }
  const programMentees = RE_LEGACY_PROGRAM_MENTEES.exec(pathname);
  if (programMentees) return `/mentorship/admin/${programMentees[1]}`;
  return SELF_SERVE_FLOWS.find(([pattern]) => pattern.test(pathname))?.[1];
}

export default defineEventHandler((event) => {
  const method = event.method.toUpperCase();
  if (method !== 'GET' && method !== 'HEAD') return;

  const { pathname, search, searchParams } = getRequestURL(event);
  const selfServe = selfServePath(pathname, searchParams);
  if (selfServe) {
    const base = useRuntimeConfig(event).public.selfServeUrl.replace(/\/$/, '');
    // 302, so browsers do not cache a redirect into another app's routes.
    return sendRedirect(event, `${base}${selfServe}`, 302);
  }
  // Checked first: /project/applied and /mentee/tasks would otherwise read as ids.
  if (RE_LEGACY_FLOW.test(pathname)) return sendRedirect(event, '/', 301);

  const program = RE_LEGACY_PROGRAM.exec(pathname);
  if (program) return sendRedirect(event, `/programs/${program[1]}${search}`, 301);
  const mentee = RE_LEGACY_MENTEE.exec(pathname);
  if (mentee) return sendRedirect(event, `/mentees/${mentee[1]}${search}`, 301);
  const mentor = RE_LEGACY_MENTOR.exec(pathname);
  if (mentor) return sendRedirect(event, `/mentors/${mentor[1]}${search}`, 301);
});
