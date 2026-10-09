<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Mentorship Rewrite — 08: Mentor Invitations

Status: Draft — under discussion (see Decision log)
Related: [04-authorization-model.md](./04-authorization-model.md) (decision 4, AQ-7), [06-route-matrix.md](./06-route-matrix.md) (invite routes), [07-email-delivery.md](./07-email-delivery.md) (email rail, `Notifier`), [01-current-system.md](./01-current-system.md) (legacy platform)
Decision ticket: [linuxfoundation/lfx-self-serve#2187](https://github.com/linuxfoundation/lfx-self-serve/issues/2187)

A Program Admin picks anyone with an LF account and invites them to mentor a published program; the mentor accepts or declines from the email. This doc is the design and its decision record; the ticket stays the tracker.

In short:

- The flow `main` has with [linuxfoundation/lfx-mentorship#255](https://github.com/linuxfoundation/lfx-mentorship/pull/255) and [linuxfoundation/lfx-mentorship#262](https://github.com/linuxfoundation/lfx-mentorship/pull/262) stays: a `program_members` mentor row in `invited`, for anyone with an LF account by `lfid`, `email` or `user_id`, published programs only, a signed link that lasts 7 days, a resend route, and `DELETE` to remove.
- `main` changes in three places: only the invitee can make an invited user a mentor, accept needs a published program, and the email gets Accept, Decline and View links.
- [lfx-v2-invite-service](https://github.com/linuxfoundation/lfx-v2-invite-service) is not used: it exists for people without an LF account, and every invitee here has one.

## Requirement

One email, three links:

> Michal invited you to join the mentorship program **XXX** as a mentor.
> [Accept the invitation] · [Decline] · [View the program]

- Sent to anyone with an **LF account**. Someone who has never used LFX Mentorship gets a `users` row from lfx-v2-auth-service when invited; the email goes to `users.email` (see Email).
- Nobody becomes a mentor without explicitly accepting.
- **View the program** links the public program page.
- Feature parity with legacy, plus only the changes under [Deliberate divergences from legacy](#deliberate-divergences-from-legacy).

## Legacy baseline

Source: [jobspring](https://github.com/linuxfoundation/jobspring) (backend) and [lfx-mentorship-upgrade](https://github.com/linuxfoundation/lfx-mentorship-upgrade) (frontend).

| Step | Legacy behaviour | Where |
| --- | --- | --- |
| Invite UI | Mentors tab on the program page: a user picker over the LF-wide user directory (`GET /users/search`, proxied to LF user-service, limited to owners of an approved program to stop PII enumeration, MENV2-1606) plus "+ Invite", disabled until a user is picked. | `src/app/pages/projects/project-detail/mentors-tab/`; `backend/user/service.go` `SearchUsers` |
| Invite API | `POST /projects/{id}/invite-mentors` `{mentors: [{name, email}]}`, program owner only. | `backend/project/transport_http.go`; `backend/project/service.go` `InviteMentorsToProject` |
| Existing mentor profile | Found by `GetProfilesByEmail(email, "mentor")`; the member row is created **already `Approved`**, with no consent step. | `InviteMentorsToProject` |
| No mentor profile | A pending member row is created **by email**, with no user id. | `CreateProjectMemberRecordByEmail` |
| Email | Mandrill `mentor-project-invite`: admin and program names, `CREATE_URL` (`/participate/mentor`, create a mentor profile) and `EDIT_URL`. **No accept or decline link.** The helper that builds `/participate/mentor/join/{id}?token=` and `/decline/{id}?token=` has zero callers. | `backend/email/service.go` `SendMentorInviteEmail`, `getMentorAcceptRejectLink` |
| Accept / decline | `GET /projects/{id}/member/mentor/status?token=` and `.../decline?token=`: login required, signed single-use token, member row found **by the caller's email**, owner emailed (`admin-mentor-accepted` / `admin-mentor-declined`). The member row had no expiry. | `backend/project/transport_http.go`; `backend/project/service_project_member.go` `UpdateProjectMemberByProjectID`; `backend/signing/` |
| Accept / decline pages | `participate/mentor/join/:projectId` and `.../decline/:projectId` read `?token=`, bounce to `/` if not logged in, show the mentor-profile form when the caller has none, then call the routes above. Approval never depended on the profile. | `MentorJoinComponent`, `MentorDeclineComponent` |
| Remove / re-invite | `POST /projects/{id}/remove-mentor` (owner only) deletes the row. No re-send: inviting again created a second pending row and another email. Owners could invite before the program was approved. | `RemoveProgramMentor`; `CreateProjectMemberRecordByEmail`; `InviteMentorsToProject` |

So parity means: the Program Admin picks an **existing account** from a search; login is required to respond; no mentor profile is required to accept; remove exists. Two legacy behaviours are **not** carried over:

- Auto-approving anyone who already had a mentor profile. It contradicts the requirement.
- The email without accept and decline links. The consent flow was built at both ends and never wired; the rewrite wires it.

## Why not lfx-v2-invite-service

[lfx-v2-invite-service](https://github.com/linuxfoundation/lfx-v2-invite-service) exists for invitees without an account: a signed link, sign-up through Auth0 universal login, then a redirect. Every invitee here already has an account. The service also cannot send this email (it sends its own one-link template and does not return the link) and has no decline, so it would add a second rail without removing any Mentorship code. lfx-v2-committee-service, its main consumer, keeps its own invite state, accept, decline and revoke as well.

## Design

### Starting point: `main`

`main` here includes [linuxfoundation/lfx-mentorship#255](https://github.com/linuxfoundation/lfx-mentorship/pull/255) and [linuxfoundation/lfx-mentorship#262](https://github.com/linuxfoundation/lfx-mentorship/pull/262).

| Area | `main` | This design |
| --- | --- | --- |
| Invitee | Anyone with an LF account, by `lfid`, `email` or `user_id`; a missing `users` row is created from lfx-v2-auth-service; no LF account is 422 | Same |
| Record | `program_members` row, `member_type = mentor`, `status = invited` | Same |
| Program status | `published` only, otherwise 400 | Same |
| Invite route | `POST /v1/programs/{uid}/members` | Same |
| Finding the invitee | `GET /v1/programs/{uid}/mentor-candidates`; Self Serve's picker is still on a mock BFF route | Self Serve's picker calls `mentor-candidates` |
| List | `GET /v1/programs/{uid}/member-management` lists every mentor row, `invited` included | Same |
| Link | HMAC token over `(programID, userID)`, 7 days, `MENTOR_INVITE_SECRET` | Same |
| Resend | `POST /v1/programs/{uid}/members/{memberId}/resend-invite` signs a fresh token | Same |
| Remove | `DELETE /v1/programs/{uid}/members/{memberId}` deletes the row in any status | Same |
| Second invite | 409 on the unique key until the row is removed | Same |
| Accept and decline | Caller must be the user in the token; the row is found by program, user and member type | Same; accept also needs a `published` program |
| Consent | A `status` in the create body, or an admin `PATCH` from `invited` to `active`, skips it | Only the invitee moves `invited` to `active` |
| Email | One link, "You have been added as a mentor" | Accept, Decline and View links |

What already matches: accept and decline require login and are `allow_all` at Heimdall with the ownership check in the service (04 AQ-7); accept goes through `UpdateIfStatus`, which queues the `member_put` and the program index refresh; `NotifyAdminMentorAccepted` and `NotifyAdminMentorDeclined` exist; Self Serve's invitee page exists and accepts the signed token.

### Flow

```mermaid
sequenceDiagram
    actor PA as Program Admin
    participant SS as Self Serve
    participant H as Heimdall
    participant API as Mentorship API
    participant E as lfx-v2-email-service
    participant S as fga-sync
    actor M as Invited mentor

    PA->>SS: search, pick a person, Invite
    SS->>H: POST /v1/programs/{uid}/members {lfid, member_type: mentor}
    H->>API: forward if writer on mentorship_program:{uid}
    API->>API: program published, else 400
    API->>API: find or create the users row from lfx-v2-auth-service, 422 if no LF account
    API->>API: insert an invited row, else 409
    API-)E: invitation email with a signed token
    E-)M: Accept, Decline, View the program
    M->>SS: open /mentorship/mentor/invites?token={token}
    Note over M,SS: login required
    M->>SS: Accept
    SS->>H: POST /v1/mentor-invites/{token}/accept
    H->>API: forward (allow_all)
    API->>API: token valid and names the caller, else 400 or 403
    API->>API: invited to active
    API-->>SS: 200
    API-)S: member_put mentor
    API-)E: admin-mentor-accepted
```

Decline takes the same path to `declined`, with no tuple, then `admin-mentor-declined`.

### Data

No new table. An invitation is a `program_members` row with `member_type = mentor` and `status = invited`; it has no FGA tuple and no indexer emission (04 decision 4). The unique key `(program_id, user_id, member_type)` allows one row per mentor and program, so a second invite for the same user is 409 until the Program Admin removes the row.

```mermaid
stateDiagram-v2
    [*] --> invited: POST
    invited --> invited: resend
    invited --> active: invitee accepts
    invited --> declined: invitee declines
    invited --> [*]: DELETE
    declined --> [*]: DELETE
```

`DELETE` removes a row in any status (FR-022), so it is also how a Program Admin invites a declined or withdrawn mentor again: remove the row, then invite. `active` becomes `approved` once [linuxfoundation/lfx-mentorship#221](https://github.com/linuxfoundation/lfx-mentorship/pull/221) lands.

### Link

The link carries an HMAC token over `(programID, userID)`, signed with `MENTOR_INVITE_SECRET` and valid for 7 days (`internal/infrastructure/auth/invite_token.go`). Accept and decline require login and check that the caller is the user the token names. Legacy also used a signed token.

A resend signs a fresh token. Earlier tokens stay valid until they expire; they all name the same user and program, and accepting moves the row out of `invited`, so only the first response counts. The same holds after a `DELETE` and a new invite of the same person: old links answer the new invitation, which names the same user and program. After a `DELETE`, or a move to `declined`, with no new invite, an old link finds no `invited` row and gets a 400. When a token expires, the Program Admin resends.

### API

| Route | Edge | Does |
| --- | --- | --- |
| `POST /v1/programs/{uid}/members` `{lfid, member_type: mentor}` | `writer` | Invite |
| `POST /v1/programs/{uid}/members/{memberId}/resend-invite` | `writer` | Resend |
| `GET /v1/programs/{uid}/mentor-candidates?search=` | `writer` | Picker search |
| `GET /v1/programs/{uid}/member-management` | unchanged | Mentor rows, `invited` included |
| `DELETE /v1/programs/{uid}/members/{memberId}` | unchanged | Remove |
| `POST /v1/mentor-invites/{token}/accept` | `allow_all` | Accept |
| `POST /v1/mentor-invites/{token}/decline` | `allow_all` | Decline |

- **Invite.** As in `main`: the person is identified by `lfid`, `user_id` or `email` (`lfid` with `user_id` is 400). An `lfid` or `email` with no `users` row creates one from lfx-v2-auth-service; no LF account, or a primary email another user already holds, is 422 and creates nothing; auth-service unreachable is 503; a user who already has a mentor row on the program, in any status, is 409. The change to `main`: a `status` in the body for a mentor is 400, so a mentor row is always created as `invited`. `program_admin` rows are unchanged.
- **Resend.** As in #255: only an `invited` mentor on a `published` program, otherwise 409. 07's send is fire-and-forget, so a lost email is a real case.
- **Remove.** As in #255: the row is deleted in any status, and an active mentor's FGA relation is removed with it.
- **Consent.** `memberTransitions` drops `invited → active` and `invited → pending`, so a Program Admin's `PATCH` cannot make an invited user a mentor. An admin can still remove the row, or move it to `declined`.
- **Accept and decline.** As in `main`: an invalid or expired token is 400; a caller who is not the token's user is 403; no `invited` mentor row for that user and program is 400. Self Serve's invite page already shows a 400 or 409 as "expired or already answered".
  - **Accept** also needs the program to be `published` (409 otherwise), the one change here. Then `UpdateIfStatus` from `invited` to `active`; the existing update queues `member_put` and the program index refresh. Then `NotifyAdminMentorAccepted`.
  - **Decline** works in any program status, since it creates nothing: `invited` to `declined`, then `NotifyAdminMentorDeclined`.

#255 and #262 add the RuleSet rules for the resend and `mentor-candidates` routes; no other rule changes. The chart's single `/mentorship/` HTTPRoute already covers the paths.

### Frontend

Both sides live in Self Serve: [02](./02-target-architecture.md) gives it the authenticated Program Admin and mentor experiences. The Nuxt site only serves the public program page behind the View link.

- **Invitee page.** It already exists: `/mentorship/mentor/invites?token=` ([linuxfoundation/lfx-self-serve#3171](https://github.com/linuxfoundation/lfx-self-serve/pull/3171)), Accept and Decline on one page; both email links open it. The token format does not change, so the page stays as it is. It maps a 403 to "This invitation is for a different account" and a 400 or 409 to "expired or already answered". Accept and decline are `POST`s from the page, so mail-client link prefetchers cannot trigger them. After accept: the mentor profile setup page when the user has none, otherwise the mentor dashboard.
- **Program Admin side.** The mentors tab's picker and Invite button sit on a mock `invitable-users` BFF route. The picker moves to `GET /v1/programs/{uid}/mentor-candidates?search=` (at least 2 characters; up to 10 results of `lfid`, name and avatar, never an email; a whole email finds the one LF account that owns it), and Invite sends the picked person's `lfid`. A 422 shows the API's message. [`backend/docs/mentor-invite-integration.md`](../../backend/docs/mentor-invite-integration.md) is the integration guide. Invited mentors already appear in the tab's list from `member-management`; Resend calls the resend route and Remove the existing `DELETE`.

### Email

Rendered in this repo and sent through lfx-v2-email-service, per [07](./07-email-delivery.md), to the invitee's `users.email`. An invite by `lfid` or `email` fills a missing or blank address with the LF account's primary email and never overwrites a stored one. An invite by `user_id` uses the row as it is; if its address is blank, `NotifyMentorInvited` logs the failure and sends nothing. Self Serve invites by `lfid`. That field is caller-writable through `PUT /v1/me`, which does not matter here: it is the invitee's own address, and acceptance is matched by user id, not by address.

```text
Subject: {Inviter} invited you to mentor {Program}

Hi {Name},

{Inviter} invited you to join the mentorship program {Program} as a mentor.

Accept the invitation: {accept_url}
Decline: {decline_url}
View the program: {program_url}

These links work for 7 days. Please log in with the username {LFID} to respond.
```

`{accept_url}` and `{decline_url}` are both `/mentorship/mentor/invites?token={token}` on Self Serve; `{program_url}` is the public Nuxt program page, always present because only `published` programs can invite. The LFID line exists today and stays, since the invitee must sign in as the invited user.

`Notifier` changes: `NotifyMentorInvited` keeps the token and gains the inviter's user id for the greeting; on a resend, the inviter is the Program Admin who resends. Accept and decline keep `NotifyAdminMentorAccepted` (`admin-mentor-accepted`) and `NotifyAdminMentorDeclined` (`admin-mentor-declined`).

### Legacy invitations

Not migrated and not re-sent. The legacy email had no accept or decline link, so no outstanding invitation could be acted on, and the source cannot tell a pending invitation from a pending mentor application ([03](./03-migration-plan.md), open question on pending mentor rows). The ETL keeps skipping these rows (`UNMAPPED_MENTOR_MEMBER`) into the triage report 03 requires, and Program Admins re-invite.

## Answers to the ticket

| # | Question in [#2187](https://github.com/linuxfoundation/lfx-self-serve/issues/2187) | Answer |
| --- | --- | --- |
| 1 | Pending-invite representation | A `program_members` mentor row in `invited`, with no FGA tuple or indexer emission (04 decision 4), listed in Self Serve's mentors tab, not in Pending Actions. See Data. |
| 2 | Accept authorization | `allow_all` at Heimdall; the service checks that the signed token names the caller and that the caller's mentor row on the program is `invited` (AQ-7 as written). See API. |
| 3 | Account-less invitees | Anyone with an LF account can be invited, including someone who has never used LFX Mentorship: the API creates their `users` row from lfx-v2-auth-service ([linuxfoundation/lfx-mentorship#262](https://github.com/linuxfoundation/lfx-mentorship/pull/262)). Someone with no LF account cannot be invited (422), as in legacy, whose picker only offered existing LF accounts. |

## Follow-up changes once approved

- **03**: the open question on pending mentor rows: the invitation half is settled here (see Legacy invitations); classifying genuine mentor applications stays open. The `withdrawn` row in the mapping table calls FR-022 "withdraw-not-delete"; FR-022 deletes the record.
- **07**: `mentor-project-invite` row: three links.
- **Spec 001** ([`specs/001-mentorship-core-workflow/spec.md`](../../specs/001-mentorship-core-workflow/spec.md)): the edge case on an expired token and open question 5 are answered: the link lasts 7 days, and the Program Admin resends it by hand.
- **Self Serve**: the mentors tab's picker calls `mentor-candidates` and invites by `lfid`; it shows the 422 message; Resend calls the resend route.

## Deliberate divergences from legacy

- No auto-approval of existing mentors. Everyone accepts.
- Published programs only. Legacy let owners invite before approval; since anyone can create a program, that would let anyone email any user.
- Links last 7 days, and the Program Admin can resend. Legacy rows never expired and had no resend: inviting again created a second row and another email.
- Outstanding legacy invitations are dropped at cutover (see Legacy invitations).

## Open questions

- Does any program need mentors attached before it is approved? Legacy allowed it; this design does not. If yes, revisit published-only for invites.

## Decision log

| Date | Event |
| --- | --- |
| 2026-09-04 | [#2187](https://github.com/linuxfoundation/lfx-self-serve/issues/2187) opened. Recommended direction at the time: own the invite resource, reuse the rails (`mentorship_invite` FGA type mirroring `committee_invite`, LFID invite JWT for the account leg). |
| 2026-09-14 | AQ-7 resolved in [04](./04-authorization-model.md) ([ca701d0](https://github.com/linuxfoundation/lfx-mentorship/commit/ca701d0)): `allow_all` plus signed token; `mentorship_invite` FGA type rejected. Contradicts the ticket's recommended direction; the ticket was not updated. |
| 2026-09-22 | [07](./07-email-delivery.md) approved ([#2188](https://github.com/linuxfoundation/lfx-self-serve/issues/2188)): email via lfx-v2-email-service, templates owned here. |
| 2026-09-24 | This doc opened. Legacy flow researched in jobspring and lfx-mentorship-upgrade. Requirement fixed as the three-link email. lfx-v2-invite-service evaluated: cannot produce the email or a decline; set aside. Option C recorded. Status: Draft. |
| 2026-09-30 | Reviewed against the platform rules, the legacy code, lfx-v2-invite-service, lfx-v2-committee-service and Self Serve. Changes: no signed token, the link carries the invitation id; caller email from the Heimdall `email` claim with a stored-row fallback, no `email_verified` (none exists downstream); a repeat `POST` is the re-send, `DELETE` is remove, `revoked` dropped; conditional update as the race guard; email normalised at creation; both pages move to Self Serve, one invitee page; the management list stays service-side as a nested edge-authorised list; legacy baseline corrected (`admin-mentor-declined`, profile never gated approval, no expiry or re-send). Open questions resolved. |
| 2026-10-01 | Review on [linuxfoundation/lfx-mentorship#177](https://github.com/linuxfoundation/lfx-mentorship/pull/177). Changes: the `users.email` fallback is replaced by an lfx-v2-auth-service lookup, because `PUT /v1/me` lets the caller set that field; `GET /v1/mentor-invites/{id}` runs the email check; an `accepted` invitation blocks a new one only while that user is an active mentor; `POST /v1/programs/{uid}/members` accepts only `program_admin`; the existing `NotifyAdminMentorAccepted` and `NotifyAdminMentorDeclined` are reused; Self Serve's existing invite page ([linuxfoundation/lfx-self-serve#3171](https://github.com/linuxfoundation/lfx-self-serve/pull/3171)) is reused with the invitation id in `token`, and the `?action=` preselect is dropped; the `mentor-invites` rule enables the OIDC contextualizer, so a just-registered invitee who is not yet in lfx-v2-auth-service can still accept. |
| 2026-10-02 | Doc tightened for readability; no design change beyond two review fixes on [linuxfoundation/lfx-mentorship#177](https://github.com/linuxfoundation/lfx-mentorship/pull/177): `DELETE` matches `program_id` as well as `id` (04's parent-child invariant), and Self Serve's `isMentorshipMentorInviteToken` must accept a UUID, since the page's other parts stay. Added why extending lfx-v2-invite-service costs more than the Mentorship-owned design. Resolved open questions folded into the design. |
| 2026-10-02 | Second review round on [linuxfoundation/lfx-mentorship#177](https://github.com/linuxfoundation/lfx-mentorship/pull/177). `GET /v1/mentor-invites/{id}` dropped: Self Serve's invite page never calls it and already handles every outcome after the click. Accept sets `accepted_user_id` and both responses set `responded_at`; `status` gets a CHECK constraint; create rejects a missing name or invalid email with 400; the self-request path and `member-management` are named alongside the new routes; no HTTPRoute per route (the chart has one prefix route); spec 001 added to the follow-ups; timestamps follow the schema convention (`created_on`, `updated_on` with the shared trigger). Flow and status diagrams added; accept also queues the program index refresh, as every active-mentor change does. |
| 2026-10-02 | Third review round on [linuxfoundation/lfx-mentorship#177](https://github.com/linuxfoundation/lfx-mentorship/pull/177). Create and accept return 409 for a `rejected` or `archived` program; decline works in any status. Accept or decline of an unknown or removed invitation is 404 (`ErrMentorInvitationNotFound`), a malformed id 400, and Self Serve shows the 404 as expired. Create and re-send are one `INSERT … ON CONFLICT` on the partial index, so concurrent first sends converge. Stated that Self Serve's BFF already provisions a new invitee's `users` row (`PUT /v1/me` on the not-provisioned 401, then a retry). Legacy pending invitations are dropped, neither migrated nor re-sent; 03 and 05 added to the follow-ups. Line references replaced by symbols. The accepted mentor row is stored as `active` until [linuxfoundation/lfx-mentorship#221](https://github.com/linuxfoundation/lfx-mentorship/pull/221) renames it `approved`. |
| 2026-10-05 | Simplified after [Lewis Ojile's comparison with `main`](https://linuxfoundation.slack.com/archives/C0BK7NQ3Q6Q/p1791217342576309) and a parity re-check of jobspring and lfx-mentorship-upgrade: legacy's picker only offered existing LF accounts, so invitations go to existing users by `user_id`, as `main` already does. Dropped: the `mentor_invitations` table (back to the `program_members` row in `invited`), invitation by email and the caller-identity design (Heimdall `email` claim, OIDC contextualizer, lfx-v2-auth-service fallback), expiry (legacy rows never expired), and inviting before `published` (anyone can create a program). Kept from earlier rounds: an id instead of an HMAC token in the link (now the member row id), re-send by repeating the `POST`, 404 for an unknown invitation, the three-link email. Added: `GET /v1/programs/{uid}/invitable-users` for the picker; only the invitee moves `invited` to `active`. |
| 2026-10-06 | Aligned with [linuxfoundation/lfx-mentorship#255](https://github.com/linuxfoundation/lfx-mentorship/pull/255) after reviewing it. Taken from #255: the signed token with its 7-day expiry and `MENTOR_INVITE_SECRET`; resend as its own route rather than a repeat `POST`; invitation by `lfid` from Self Serve's existing LF user search, with 422 for an `lfid` that has no Mentorship account, so `invitable-users` is dropped; `DELETE` removes a row in any status, which is also how a declined or withdrawn mentor is invited again; an unknown invitation stays 400. Still changes to `main`: only the invitee moves `invited` to `active`, accept needs a `published` program, and the three-link email. |
| 2026-10-09 | Aligned with [linuxfoundation/lfx-mentorship#262](https://github.com/linuxfoundation/lfx-mentorship/pull/262), agreed in [Slack](https://linuxfoundation.slack.com/archives/C0BK7NQ3Q6Q/p1791311026046579): anyone with an LF account can be invited by `lfid`, `email` or `user_id`, and the API creates a missing `users` row from lfx-v2-auth-service over NATS, not user-service, which may be retired. The picker uses `GET /v1/programs/{uid}/mentor-candidates`; Self Serve's `/api/search/users` is not an LF directory search. Dropped: the 422 for someone who never used Mentorship, and the matching divergence from legacy. Review fixes on [linuxfoundation/lfx-mentorship#177](https://github.com/linuxfoundation/lfx-mentorship/pull/177): the lfx-v2-invite-service summary matches its section; Email says where the address comes from and that a blank one on a `user_id` invite is not sent; Link covers old links after a `DELETE` and a new invite. |
