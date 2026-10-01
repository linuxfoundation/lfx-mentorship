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
| Accept / decline | `GET /projects/{id}/member/mentor/status?token=` and `.../decline?token=`. Require login, verify the signed token, find the member row **by the caller's email**, approve or disapprove, email the owner (`admin-mentor-accepted` on accept, `admin-mentor-declined` on decline), mark the token used. The member row had no expiry. | `backend/project/transport_http.go`; `backend/project/service_project_member.go` `UpdateProjectMemberByProjectID`; `backend/signing/` |
| Accept / decline pages | `participate/mentor/join/:projectId` and `participate/mentor/decline/:projectId` read `?token=`, bounce to `/` if not logged in, show the mentor-profile form when the caller has none, then call the routes above. The form was a nudge, not a gate: the backend approved the row whether or not a mentor profile existed. | `MentorJoinComponent`, `MentorDeclineComponent` |
| Remove / re-invite | `POST /projects/{id}/remove-mentor` (owner only) deletes the member row. No re-send endpoint: inviting the same address again created a second pending row and sent another email. No program-status gate: owners could invite before the program was approved. | `RemoveProgramMentor`; `CreateProjectMemberRecordByEmail`; `InviteMentorsToProject` |

What this means for parity:

1. Invitation is by **email**, not by user id. The recipient may have no account.
2. Legacy auto-approved anyone who already had a mentor profile. That contradicts the requirement and is **not** carried over.
3. The shipped email had no accept/decline links, but the token routes and pages exist at both ends. The consent flow was designed and never wired. The rewrite ships it wired.
4. Accept/decline required login and matched the caller to the invitation **by email**. The token was single-use.
5. A mentor profile was never required to accept. Removing a mentor existed; re-send, revoke and expiry did not.

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
- **C — Mentorship-owned.** One invitation row, one email via lfx-v2-email-service, one page in Self Serve. The invite service is not involved.

**Decision: C.** The one thing the requirement needs that the invite service cannot do is the email itself, and once Decline is ours the service would own nothing but the Accept link. Its account-less leg is not unique: our invitation page requires login, and Auth0 universal login handles sign-up, exactly as Self Serve's committee invite page does today. Costs of C, stated plainly: Mentorship keeps its own invitation table, and mentorship invitations do not appear in Self Serve's Pending Actions widget (today nothing but committees does).

## Design

### Data

New table `mentor_invitations`:

| Column | Notes |
| --- | --- |
| `id` | UUID v4; also the credential carried in the email link (see Link) |
| `program_id` | FK `programs` `ON DELETE CASCADE`, as `program_members` |
| `email`, `name` | `email` stored trimmed and lower-cased; `name` as entered by the Program Admin |
| `invited_by` | FK `users` (the Program Admin) |
| `status` | `pending` \| `accepted` \| `declined` — named string type with `IsValid()`, per Code Style |
| `expires_at` | 30 days from creation; refreshed by a repeat invite (see API) |
| `accepted_user_id` | FK `users`, set on accept |
| `created_at`, `responded_at` | |

Partial unique index on `(program_id, email) WHERE status = 'pending'`: one open invitation per address per program. A `declined` row does not block a new invitation, and an expired `pending` row is refreshed in place by the next `POST`, so the index never strands an address.

Why a new table rather than `program_members.status = 'invited'` plus its `email` column: `user_id NOT NULL REFERENCES users` is the exact constraint that makes the current flow unusable. Loosening it would make every roster query defend against null users and the unique key would stop deduplicating invites. `program_members` stays "people who are in the program"; an invitation is a proposal to join. [04](./04-authorization-model.md) decision 4 is unchanged: a pending invitation is a Postgres row with no FGA tuple and no indexer emission.

`program_members.status = 'invited'` becomes unused. Dropping it from the CHECK constraint is a follow-up, not part of this change.

### Link

The email link carries the invitation `id`; there is no signed token. Accept and decline already require login and an email match, so a signature would prove nothing the row does not. A v4 UUID is not guessable, expiry lives on the row, and the status change makes the link single-use. lfx-v2-committee-service's accept route takes the invite UID the same way. The HMAC helper (`internal/infrastructure/auth/invite_token.go`) and `MENTOR_INVITE_SECRET` (chart `values.yaml`, `validate.yaml`) go away. TTL: 30 days (the platform invite service's default; the current code's 7 days is short for a volunteer mentor).

### API

| Route | Auth | Object | Authorizer | Service rule |
| --- | --- | --- | --- | --- |
| `POST /v1/programs/{uid}/invitations` `{email, name}` | required | `mentorship_program:{uid}` | `writer` | Trim and lower-case the email. No row for the address, or only `declined` rows: insert and send the email. A `pending` row: refresh `expires_at`, re-send, return it — this is the re-send, and it also revives an expired invitation. An `accepted` row: 409. Becomes the mentor-invite entry point; `POST /v1/programs/{uid}/members` remains for Program Admins. |
| `GET /v1/programs/{uid}/invitations` | required | `mentorship_program:{uid}` | `writer` | Management list. A nested, edge-authorised list stays service-side, like `GET /v1/programs/{uid}/members`; the Query Service rule in [06](./06-route-matrix.md) covers caller-owned collections. Closes the "invitation management has no read path" gap noted in [05 GW-9](./05-heimdall-gateway.md). |
| `DELETE /v1/programs/{uid}/invitations/{id}` | required | `mentorship_program:{uid}` | `writer` | Delete a `pending` row (legacy `remove-mentor`). Any other status: 409. |
| `GET /v1/mentor-invites/{id}` | required | — | `allow_all` | Program name, inviter, invitee email, status, expiry, so the page can render. The id is the credential. |
| `POST /v1/mentor-invites/{id}/accept` | required | — | `allow_all` | Caller email = the Heimdall `email` claim, falling back to the stored `users.email` for the principal; must equal `row.email`, else 403. In one transaction: `UPDATE … SET status = 'accepted' WHERE id = $1 AND status = 'pending' AND expires_at > now()`, zero rows → 409 (the race guard); upsert `program_members` mentor `active` (same path as `ensureAcceptedMentorMembership`, `application_repository.go:458`); emit `member_put` for `mentor`. Then `NotifyMentorAccepted`. |
| `POST /v1/mentor-invites/{id}/decline` | required | — | `allow_all` | Same email check and conditional update; row → `declined`; `NotifyMentorDeclined`. |

The accept and decline checks are the same shape AQ-7 already accepted: `allow_all` at the edge, ownership in the service. The subject moves from the `userID` inside the token to the email on the row, because at invite time there is no user. There is no `email_verified` to check: Heimdall populates the `email` claim only from a verified source and no such claim exists downstream, so a per-service check would be redundant (platform rule; lfx-v2-committee-service resolves the caller's email the same way). The claim is present only when the OIDC contextualizer is enabled, hence the fallback to the stored user row. Each new route ships with its HTTPRoute and RuleSet rule in the same PR.

### Frontend

Both sides live in Self Serve: [02](./02-target-architecture.md) gives it the authenticated Program Admin and mentor experiences, and it already hosts mentor profile registration ([linuxfoundation/lfx-self-serve#3148](https://github.com/linuxfoundation/lfx-self-serve/pull/3148)) and the committee invite page. The Nuxt site only serves the public program page the View link points at.

- One invitee page, `/mentor-invites/:id`, with Accept and Decline buttons. The email's Accept and Decline links open the same page with `?action=accept|decline`, which preselects the button. Login required; Auth0 universal login offers sign-up.
- The mutation is a `POST` from the page, so link prefetchers in mail clients cannot accept or decline by following the link.
- Email mismatch renders "This invitation was sent to a@example.org; you are signed in as b@example.org" with a sign-out link.
- After accept: the mentor profile setup page when the user has no mentor profile, otherwise the mentor dashboard. Legacy showed the profile form on the join page but never gated approval on it; neither does the rewrite.
- Program Admin side: the mentors tab of the admin program-detail page replaces its user-picker placeholder (and the mock `invitable-users` BFF route behind it) with an "Invite mentor" form (email, name) and a pending-invitations list with Resend (the same `POST`) and Remove (`DELETE`). No user picker: v2 has no user search, and legacy's was gated for PII reasons anyway.

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

`{accept_url}` and `{decline_url}` are the Self Serve invitation page with `?action=`; `{program_url}` is the public Nuxt program page.

`Notifier` changes: `NotifyMentorInvited(ctx, invitation)` replaces `(ctx, programID, userID, token)` and takes over the legacy `mentor-project-invite` slot in 07's mapping. `NotifyMentorAccepted` is added for the Program Admin notification legacy sent on accept (`admin-mentor-accepted`), alongside the existing `NotifyMentorDeclined` (`admin-mentor-declined`).

## Answers to the ticket

| # | Question in #2187 | Answer |
| --- | --- | --- |
| 1 | Pending-invite representation | Postgres row in `mentor_invitations`. No FGA type, no indexer emission. Program Admins see it in Self Serve's mentors tab through the nested list route; it is not in the Pending Actions widget. Same as 04 decision 4. |
| 2 | Accept authorization | `allow_all` at Heimdall; service checks that the row is `pending` and unexpired and that the caller's email (Heimdall `email` claim, fallback stored user email) matches the row. Same shape as AQ-7 and as lfx-v2-committee-service's accept; subject is the email on the row. |
| 3 | Account-less invitees | Login-required page in Self Serve; Auth0 universal login handles sign-up. No invite-service involvement. |

## Changes to apply in other docs once approved

- **04**: decision 4 and AQ-7 wording, "user named in the signed invitation token" → "email recorded on the invitation", "signed token" → "invitation id". Lifecycle table unchanged (`member_put` on accept).
- **06**: invite routes section: the two `{token}` routes become the three `{id}` routes above; add the three `programs/{uid}/invitations` routes; note that `POST /v1/programs/{uid}/members` is no longer the mentor-invite entry point.
- **07**: `Notifier` mapping — `NotifyMentorInvited(invitation)` → this email; add `NotifyMentorAccepted` (`admin-mentor-accepted`).
- **Backend chart**: drop `MENTOR_INVITE_SECRET` from `values.yaml` and `validate.yaml`; RuleSet paths `mentor-invites/:token/*` → `:id`.

## Deliberate divergences from legacy

- No auto-approval of existing mentors. Everyone accepts.
- No user search. Email and name only.
- No signed token. The link carries the invitation id; login plus email match is the guard.
- 30-day expiry and in-place re-send. Legacy had neither; re-inviting created a second row. Current rewrite code: 7 days.
- The match is against the caller's primary email only. Accounts with several addresses are a follow-up (lfx-v2-auth-service lookup, as committee-service does); until then the Program Admin re-invites the address the mentor signs in with.

## Open questions

All resolved on 2026-09-30 (see Decision log):

1. Invitee signs in with a different email than the one invited: strict match, clear error, sign-out link. Legacy behaviour.
2. Re-send and remove ship in the first cut, as the idempotent `POST` and the `DELETE` above. 07's email send is fire-and-forget, so re-send is needed from day one. No `revoked` status.
3. Inviting before the program is `published`: yes, as legacy allowed. The View link is omitted.

## Decision log

| Date | Event |
| --- | --- |
| 2026-09-04 | [#2187](https://github.com/linuxfoundation/lfx-self-serve/issues/2187) opened. Recommended direction at the time: own the invite resource, reuse the rails (`mentorship_invite` FGA type mirroring `committee_invite`, LFID invite JWT for the account leg). |
| 2026-09-14 | AQ-7 resolved in [04](./04-authorization-model.md) ([ca701d0](https://github.com/linuxfoundation/lfx-mentorship/commit/ca701d0)): `allow_all` plus signed token; `mentorship_invite` FGA type rejected. Contradicts the ticket's recommended direction; the ticket was not updated. |
| 2026-09-22 | [07](./07-email-delivery.md) approved ([#2188](https://github.com/linuxfoundation/lfx-self-serve/issues/2188)): email via lfx-v2-email-service, templates owned here. |
| 2026-09-24 | This doc opened. Legacy flow researched in jobspring and lfx-mentorship-upgrade. Requirement fixed as the three-link email. lfx-v2-invite-service evaluated: cannot produce the email or a decline; set aside. Option C recorded. Status: Draft. |
| 2026-09-30 | Reviewed against the platform rules, the legacy code, lfx-v2-invite-service, lfx-v2-committee-service and Self Serve. Changes: no signed token, the link carries the invitation id; caller email from the Heimdall `email` claim with a stored-row fallback, no `email_verified` (none exists downstream); idempotent `POST` is the re-send, `DELETE` is remove, `revoked` dropped; conditional update as the race guard; email normalised at creation; both pages move to Self Serve, one invitee page; the management list stays service-side as a nested edge-authorised list; legacy baseline corrected (`admin-mentor-declined`, profile never gated approval, no expiry or re-send). Open questions resolved. |
