<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Mentorship Rewrite — 08: Mentor Invitations

Status: Draft — under discussion (see Decision log)
Related: [04-authorization-model.md](./04-authorization-model.md) (decision 4, AQ-7), [06-route-matrix.md](./06-route-matrix.md) (invite routes), [07-email-delivery.md](./07-email-delivery.md) (email rail, `Notifier`), [01-current-system.md](./01-current-system.md) (legacy platform)
Decision ticket: [linuxfoundation/lfx-self-serve#2187](https://github.com/linuxfoundation/lfx-self-serve/issues/2187)

A Program Admin invites a mentor by email; the mentor accepts or declines from that email. This doc records how the rewrite implements that flow, what the legacy platform actually did (the parity baseline), why the platform's [lfx-v2-invite-service](https://github.com/linuxfoundation/lfx-v2-invite-service) was evaluated and set aside, and the history of the decision. The ticket stays the tracker; this file holds the reasoning so it survives ticket rewrites.

## Requirement

One email, three links:

> Michal invited you to join the mentorship program **XXX** as a mentor.
> [Accept the invitation] · [Decline] · [View the program]

- Sent to an **email address**. The invitee need not have an LFX account or have ever visited Mentorship.
- **Accept** and **Decline** are explicit choices by the invitee. Nobody becomes a mentor without accepting.
- **View the program** points at the public program page and is included only when the program is `published`.
- Feature parity with legacy, and no more.

## Legacy baseline (what parity means)

Source: [jobspring](https://github.com/linuxfoundation/jobspring) (backend) and [lfx-mentorship-upgrade](https://github.com/linuxfoundation/lfx-mentorship-upgrade) (frontend).

| Step | Legacy behaviour | Where |
| --- | --- | --- |
| Invite UI | Mentors tab on the program detail page: a searchable user picker (backed by `GET /users/search`, gated to owners of an approved program to stop PII enumeration, MENV2-1606) plus "+ Invite". Sends `{name, email}` per mentor. | `src/app/pages/projects/project-detail/mentors-tab/`; `backend/user/service.go` `SearchUsers` |
| Invite API | `POST /projects/{id}/invite-mentors` with `mentors: [{name, email}]`. Only the program owner may call it. | `backend/project/transport_http.go`; `backend/project/service.go` `InviteMentorsToProject` |
| Existing mentor profile | Found by `GetProfilesByEmail(email, "mentor")` → a member row is created **already `Approved`**. No consent step. | `backend/project/service.go` `InviteMentorsToProject` |
| No mentor profile | A member row is created **by email** (no user id), pending. | `CreateProjectMemberRecordByEmail` |
| Email | Mandrill template `mentor-project-invite`: admin name, program name, `CREATE_URL` (`/participate/mentor`, the "create your mentor profile" page) and `EDIT_URL`. **No accept or decline link.** A helper that builds `/participate/mentor/join/{id}?token=` and `/decline/{id}?token=` exists but has **zero callers**. | `backend/email/service.go` `SendMentorInviteEmail`, `getMentorAcceptRejectLink` |
| Accept / decline | `GET /projects/{id}/member/mentor/status?token=` and `.../decline?token=`. Require login, verify the signed token, find the member row **by the caller's email**, approve or disapprove, email the owner (`mentor-admin-declined` on decline), mark the token used. | `backend/project/transport_http.go`; `backend/project/service_project_member.go` `UpdateProjectMemberByProjectID`; `backend/signing/` |
| Accept / decline pages | `participate/mentor/join/:projectId` and `participate/mentor/decline/:projectId` read `?token=`, bounce to `/` if not logged in, show the mentor-profile form when the caller has none, then call the routes above. | `MentorJoinComponent`, `MentorDeclineComponent` |

What this means for parity:

1. Invitation is by **email**, not by user id. The recipient may have no account.
2. Legacy auto-approved anyone who already had a mentor profile. That contradicts the requirement and is **not** carried over.
3. The shipped email had no accept/decline links, but the token routes and pages exist at both ends. The consent flow was designed and never wired. The rewrite ships it wired.
4. Accept/decline required login and matched the caller to the invitation **by email**. The token was single-use.

## Where the rewrite stands

- `POST /v1/programs/{uid}/members` with `member_type=mentor` creates a `program_members` row with status `invited`, mints an HMAC token over `(programID, userID)` (`internal/infrastructure/auth/invite_token.go`, 7-day TTL, `MENTOR_INVITE_SECRET`) and calls `Notifier.NotifyMentorInvited` — today a log line (`server.go:68`, `NewLogNotifier`).
- `POST /v1/mentor-invites/{token}/accept|decline` (`server.go:214-215`) validate the token and require the caller to be the `userID` it names (`program_member_service.go:213,257`).
- **Blocker:** `program_members.user_id` is `NOT NULL REFERENCES users` (`001_initial.up.sql:162`) and the service rejects a missing `user_id` (`program_member_service.go:115`). A `users` row exists only after that person's first login (`UserService.Bootstrap`, `user_service.go:25`), and v2 has no user directory or search. The current flow can only invite people who have already used the new platform, so it cannot send a real invitation.
- [04](./04-authorization-model.md) decision 4 and AQ-7 (resolved 2026-09-14, [ca701d0](https://github.com/linuxfoundation/lfx-mentorship/commit/ca701d0)): pending invitations have no FGA presence; the accept route is `allow_all` at the edge with a service-side ownership check via the signed token; a `mentorship_invite` FGA type mirroring `committee_invite` was rejected. The ticket's "recommended direction" (a `mentorship_invite` FGA type plus the LFID invite JWT) predates that resolution and was never reconciled with it. This doc reconciles them.

## The invite service, evaluated

[lfx-v2-invite-service](https://github.com/linuxfoundation/lfx-v2-invite-service) is the platform rail the ticket points at. What it offers against what the requirement needs:

| Need | Invite service | Fit |
| --- | --- | --- |
| Signed, expiring link | Yes: HS256 JWT, `expiration_days` default 30, max 90 | ✓ |
| Account-less invitee | Yes: Self Serve `/invite?token=` requires login; Auth0 universal login offers sign-up; then redirects to `return_url` | ✓ |
| The three-link email | **No.** It sends its own fixed single-link template ("Accept invitation: {ReturnURL}") and `send_invite` returns `{uid, email, expires_at}` — **not the link** — so the caller cannot embed it in its own email | ✗ |
| Decline | **No.** Statuses are `pending` and `accepted` only. No decline or revoke API (an `InviteRevokedSubject` constant exists; nothing publishes it) | ✗ |
| Re-send, revoke | No | ✗ |
| Invitee-facing pending-invitations view | Committees only; the generic `/invite` route just redirects | ✗ |
| Audit record | Permanent KV record in the `invites` bucket, including the recipient's email | neutral |

Three ways to use it, and what each costs:

- **A — invite service end-to-end.** The invitee gets the generic platform email with one link, logs in at Self Serve, and is redirected to a Mentorship landing page offering Accept / Decline / View. Two clicks instead of one, the email copy is not ours, and Decline is Mentorship-owned regardless, so we still need our own invitation row and routes. The service ends up owning only the Accept link.
- **B — invite service as the link minter.** Requires a change in lfx-v2-invite-service (return the link, suppress its email). Accept then goes through Self Serve's `/invite` and the `lfx.invite.accepted` event, while Decline is a Mentorship token: two token formats, a NATS subscription, and a cross-team dependency, all for one link.
- **C — Mentorship-owned.** One invitation row, one token, one email via lfx-v2-email-service, two pages in our own frontend. The invite service is not involved.

**Decision: C.** The one thing the requirement needs that the invite service cannot do is the email itself, and once Decline is ours the service would own nothing but the Accept link. Its account-less leg is not unique: our Accept page requires login, and Auth0 universal login handles sign-up (the frontend's `login()` already accepts `screenHint: 'signup'`, `useAuth.ts:42`). Costs of C, stated plainly: Mentorship keeps a signing secret and its own invitation table, and mentorship invitations do not appear in Self Serve's pending-invitations surface (today nothing but committees does).

## Design

### Data

New table `mentor_invitations`:

| Column | Notes |
| --- | --- |
| `id` | UUID |
| `program_id` | FK `programs` |
| `email`, `name` | as entered by the Program Admin; email compared case-insensitively |
| `invited_by` | FK `users` (the Program Admin) |
| `status` | `pending` \| `accepted` \| `declined` \| `revoked` — named string type with `IsValid()`, per Code Style |
| `expires_at` | 30 days from creation |
| `accepted_user_id` | FK `users`, set on accept |
| `created_at`, `responded_at` | |

Partial unique index on `(program_id, lower(email)) WHERE status = 'pending'`: one open invitation per address per program.

Why a new table rather than `program_members.status = 'invited'` plus its `email` column: `user_id NOT NULL REFERENCES users` is the exact constraint that makes the current flow unusable. Loosening it would make every roster query defend against null users and the unique key would stop deduplicating invites. `program_members` stays "people who are in the program"; an invitation is a proposal to join. [04](./04-authorization-model.md) decision 4 is unchanged: a pending invitation is a Postgres row with no FGA tuple and no indexer emission.

`program_members.status = 'invited'` becomes unused. Dropping it from the CHECK constraint is a follow-up, not part of this change.

### Token

Keep the HMAC helper in `internal/infrastructure/auth`; the payload becomes the invitation id plus expiry instead of `(programID, userID)`. Validation yields the id; the row is the source of truth for status and expiry. Single-use falls out of the status change. TTL: 30 days (the platform invite service's default; the current code's 7 days is short for a volunteer mentor).

### API

| Route | Auth | Object | Authorizer | Service rule |
| --- | --- | --- | --- | --- |
| `POST /v1/programs/{uid}/invitations` `{email, name}` | required | `mentorship_program:{uid}` | `writer` | Create row, send email. Becomes the mentor-invite entry point; `POST /v1/programs/{uid}/members` remains for Program Admins. |
| `GET /v1/programs/{uid}/invitations` | required | `mentorship_program:{uid}` | `writer` | Management list. Closes the "invitation management has no read path" gap noted in [05 GW-9](./05-heimdall-gateway.md). |
| `GET /v1/mentor-invites/{token}` | required | — | `allow_all` | Program name, inviter, invitee email, status, so the page can render. The token is the credential. |
| `POST /v1/mentor-invites/{token}/accept` | required | — | `allow_all` | Row `pending` and unexpired; `principal.EmailVerified` and `lower(principal.Email) == lower(row.email)`; upsert `program_members` mentor `active` (same path as `ensureAcceptedMentorMembership`, `application_repository.go:458`); emit `member_put`; row → `accepted`; notify Program Admins. |
| `POST /v1/mentor-invites/{token}/decline` | required | — | `allow_all` | Same check; row → `declined`; `NotifyMentorDeclined`. |

The accept and decline checks are the same shape AQ-7 already accepted: `allow_all` at the edge, ownership in the service. The subject moves from the `userID` inside the token to the verified email on the row, because at invite time there is no user.

### Frontend

Two pages, mirroring legacy's `participate/mentor/join/:id` and `decline/:id`:

- `/mentor-invites/[token]/accept` and `/mentor-invites/[token]/decline`. Both require login (redirect to Auth0 with a return URL; universal login offers sign-up).
- Each shows the invitation and **one confirm button**. The mutation is a `POST` from the page, so link prefetchers in mail clients cannot accept or decline by following the link.
- Email mismatch renders "This invitation was sent to a@example.org; you are signed in as b@example.org" with a sign-out link.
- Program Admin side: an "Invite mentor" form (email, name) and the pending list on the program's management page. No user picker: v2 has no user search, and legacy's was gated for PII reasons anyway.

### Email

Rendered in this repo and sent through lfx-v2-email-service, per [07](./07-email-delivery.md).

```text
Subject: {Inviter} invited you to mentor {Program}

Hi {Name},

{Inviter} invited you to join the mentorship program {Program} as a mentor.

Accept the invitation: {accept_url}
Decline: {decline_url}
View the program: {program_url}          ← only when the program is published

This invitation expires on {expires_at}.
```

`Notifier` changes: `NotifyMentorInvited(ctx, invitation)` replaces `(ctx, programID, userID, token)` and takes over the legacy `mentor-project-invite` slot in 07's mapping. `NotifyMentorAccepted` is added for the Program Admin notification legacy sent on accept, alongside the existing `NotifyMentorDeclined`.

## Answers to the ticket

| # | Question in #2187 | Answer |
| --- | --- | --- |
| 1 | Pending-invite representation | Postgres row in `mentor_invitations`. No FGA type, no indexer emission, not shown in Self Serve. Same as 04 decision 4. |
| 2 | Accept authorization | `allow_all` at Heimdall; service checks token validity and verified-email match. Same shape as AQ-7; subject is the email on the row. |
| 3 | Account-less invitees | Login-required pages on the Mentorship frontend; Auth0 universal login handles sign-up. No Self Serve or invite-service involvement. |

## Changes to apply in other docs once approved

- **04**: decision 4 and AQ-7 wording, "user named in the signed invitation token" → "verified email recorded on the invitation". Lifecycle table unchanged (`member_put` on accept).
- **06**: invite routes section gains the three new routes; note that `POST /v1/programs/{uid}/members` is no longer the mentor-invite entry point.
- **07**: `Notifier` mapping — `NotifyMentorInvited(invitation)` → this email; add `NotifyMentorAccepted`.

## Deliberate divergences from legacy

- No auto-approval of existing mentors. Everyone accepts.
- No user search. Email and name only.
- 30-day expiry (current rewrite code: 7 days).

## Open questions

| # | Question | Default if unanswered |
| --- | --- | --- |
| 1 | Invitee signs in with a different email than the one invited. Keep the strict match (legacy) or accept bearer semantics (whoever holds the link)? | Strict match, clear error, sign-out link. |
| 2 | Re-send and revoke in the first cut? | Follow-up. Both are a row update plus a resend. |
| 3 | May a Program Admin invite before the program is `published`? | Yes. The View link is omitted. |

## Decision log

| Date | Event |
| --- | --- |
| 2026-09-04 | [#2187](https://github.com/linuxfoundation/lfx-self-serve/issues/2187) opened. Recommended direction at the time: own the invite resource, reuse the rails (`mentorship_invite` FGA type mirroring `committee_invite`, LFID invite JWT for the account leg). |
| 2026-09-14 | AQ-7 resolved in [04](./04-authorization-model.md) ([ca701d0](https://github.com/linuxfoundation/lfx-mentorship/commit/ca701d0)): `allow_all` plus signed token; `mentorship_invite` FGA type rejected. Contradicts the ticket's recommended direction; the ticket was not updated. |
| 2026-09-22 | [07](./07-email-delivery.md) approved ([#2188](https://github.com/linuxfoundation/lfx-self-serve/issues/2188)): email via lfx-v2-email-service, templates owned here. |
| 2026-09-24 | This doc opened. Legacy flow researched in jobspring and lfx-mentorship-upgrade. Requirement fixed as the three-link email. lfx-v2-invite-service evaluated: cannot produce the email or a decline; set aside. Option C recorded. Status: Draft. |
