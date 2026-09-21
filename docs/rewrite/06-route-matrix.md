<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Mentorship Rewrite — 06: Route Authorization Matrix

Status: Proposal — for Architecture team review
Related: [04-authorization-model.md](./04-authorization-model.md) (what FGA holds), [05-heimdall-gateway.md](./05-heimdall-gateway.md) (how requests reach the edge)

[04](./04-authorization-model.md) proposes the FGA model and [05](./05-heimdall-gateway.md) the edge that consumes it. Neither maps the API route by route, and [05](./05-heimdall-gateway.md) explicitly defers that mapping to the RuleSets PR (PR 3 of [04 §implementation path](./04-authorization-model.md)). This document is that mapping: every route in `backend/cmd/mentorship-api/server.go` walked into an explicit authenticator / authorizer / object-extraction / relation table.

**This table is the implementation contract for the RuleSets.** One rule per route, in this order, matching the platform pattern: `authenticator: oidc` → `authenticator: anonymous_authenticator` → `authorizer: openfga_check` → `finalizer: create_jwt`. A route marked `allow_all` omits the `openfga_check` step; it is still authenticated unless its **Auth** column says `anonymous`.

## How to read the table

| Column | Meaning |
| --- | --- |
| **Auth** | `anonymous` — no token required (Heimdall's `anonymous_authenticator`). `required` — a valid OIDC token. `optional` — currently `optionalJWT`; see the note on visibility below |
| **Object** | What Heimdall interpolates into the `openfga_check`. `—` means no check |
| **Relation** | The relation checked on that object. Names resolve **per type**: `manager` on `mentorship_program` is `writer or mentor`, but `manager` on `mentorship_application` is `writer from mentorship_program` (admins only). Every rule must name the type as well as the relation |
| **Service must also** | The residue the edge cannot express — enforced in the service, not at the gateway |

Three rules govern the whole table, and each is a place a RuleSet can be wrong while looking right:

1. **Every interpolated object ID must be a UID, never a slug.** `resolveVisibleProgram` (`internal/handler/program_handler.go`) accepts `{id}` as a UUID *or* a slug, but tuples are UID-keyed, so a RuleSet built from the raw capture would check `mentorship_program:{slug}`, find no tuple, and deny a valid URL. This is not a public-vs-protected distinction: per [05](./05-heimdall-gateway.md) GW-2, ID-addressed public reads keep the `viewer@user:*` wildcard check, so a slug is denied there too. Slug resolution therefore happens *ahead* of any check, via the public resolver route. Only routes with no object-level check at all may take a slug.
2. **Parent-authorized routes need a parent-child invariant in the service.** Wherever the check is on a parent but the mutation targets a child by its own ID, the service must verify the child belongs to that parent, or the edge authorized a different object than the one mutated ([04 §decision 7](./04-authorization-model.md)).
3. **A `POST` whose body carries the subject's identity needs that field bound to `principal`.** An edge check that only confirms authentication does not stop a caller supplying someone else's ID ([04 §decision 7](./04-authorization-model.md)).

## Health probes

| Route | Auth | Object | Relation | Service must also |
| --- | --- | --- | --- | --- |
| `GET /livez`, `/healthz`, `/readyz` | — | — | — | **Not routed through the gateway at all.** These are kubelet probe targets on the cluster-local Service; exposing them on `lfx-api.{lfx.domain}` publishes liveness surface for no caller that needs it |

## Public catalog reads

Per [05](./05-heimdall-gateway.md) GW-2, the split is by route *shape*: a collection route has no object UID to check, so it gets `allow_all` and keeps the service's existing `status = published` filter; an ID-addressed read keeps the wildcard check.

| Route | Auth | Object | Relation | Service must also |
| --- | --- | --- | --- | --- |
| `GET /v1/programs` | anonymous | — | `allow_all` | Keep the published-status filter — it is the only thing scoping this list |
| `GET /v1/programs/catalog` | anonymous | — | `allow_all` | As above |
| `GET /v1/programs/resolve/{id}` | anonymous | — | `allow_all` | **The slug→UID resolver, and the one route that must accept a slug.** Rule 1 depends on it existing. It must not leak non-public programs: resolve only to UIDs the caller could read anyway |
| `GET /v1/programs/{uid}` | anonymous | `mentorship_program:{uid}` | `viewer` | — |
| `GET /v1/programs/{uid}/catalog` | anonymous | `mentorship_program:{uid}` | `viewer` | — |
| `GET /v1/programs/{uid}/skills` | anonymous | `mentorship_program:{uid}` | `viewer` | — |
| `GET /v1/programs/{uid}/mentees` | anonymous | `mentorship_program:{uid}` | `viewer` | Keep the accepted/graduated filter — `viewer` admits the public, so the *set* of mentees returned is a payload decision, not an access one |
| `GET /v1/programs/{uid}/members` | anonymous | `mentorship_program:{uid}` | `viewer` | Keep the email redaction ([05](./05-heimdall-gateway.md) GW-9, fixed in [linuxfoundation/lfx-mentorship#144](https://github.com/linuxfoundation/lfx-mentorship/pull/144)) — withheld from every caller this route admits, so redaction is correct here and a route split would be wrong |
| `GET /v1/programs/{uid}/funding-stats` | anonymous | `mentorship_program:{uid}` | `viewer` | — |
| `GET /v1/programs/{uid}/terms` | anonymous | `mentorship_program:{uid}` | `viewer` | — |
| `GET /v1/programs/{uid}/transactions` | anonymous | `mentorship_program:{uid}` | `viewer` | — |
| `GET /v1/programs/{uid}/sponsors` | anonymous | `mentorship_program:{uid}` | `viewer` | — |
| `GET /v1/mentees`, `/v1/mentees/summary`, `/v1/mentors`, `/v1/mentors/summary`, `/v1/summary`, `/v1/funding-stats/total` | anonymous | — | `allow_all` | Aggregate/collection reads with no object UID. Keep whatever published-scope filter each already applies |
| `GET /v1/mentees/{id}`, `/v1/mentors/{id}` | anonymous | — | `allow_all` | These are directory profiles, not model types. Nothing in the model keys on them, so there is no check to make — the service must return only publicly-listable records |

**The `optional` authenticator disappears.** Six routes use `optionalJWT` today (`resolve`, `{id}`, `mentees`, `transactions`, `sponsors`) solely so `resolveVisibleProgram` can let a `hidden` program's owner still fetch it. Behind Heimdall that is a relation, not a token check: `viewer` is `[user:*] or auditor`, and the owner holds `writer` → `manager` → `auditor`. So the rule is `oidc` **then** `anonymous_authenticator` — an anonymous caller fails the wildcard only when the program is not public, and the owner passes via `auditor`. The service's `hidden`-owner special case then has nothing left to do, provided the archived/hidden transitions re-emit `update_access` **without** `public` as [04 §lifecycle](./04-authorization-model.md) requires. If that emission is missed, a hidden program stays publicly readable and this row is the leak.

## Invite routes

| Route | Auth | Object | Relation | Service must also |
| --- | --- | --- | --- | --- |
| `POST /v1/mentor-invites/{token}/accept` | required | — | `allow_all` | **Compare the token's subject to the caller** (AQ-7). `ValidateInviteToken` returns the `userID` that `GenerateInviteToken` signed; reject unless it matches `principal`. Without this, `allow_all` lets any authenticated caller accept a known invitation |
| `POST /v1/mentor-invites/{token}/decline` | required | — | `allow_all` | As above |

These are the documented exception to "no service-side authorization decisions" ([04](./04-authorization-model.md) AQ-7). Note the **Auth** change: both routes are unauthenticated today, since the token was the whole credential. Behind Heimdall they require a token *and* the ownership check — the HMAC alone no longer suffices, because the accept must bind to a real LFID.

## Identity routes

Per [04 §decision 7](./04-authorization-model.md), `user` has no relations of its own, so a `{id}` path here is uncheckable and the by-ID mutations become `/me` routes.

| Route | Auth | Object | Relation | Service must also |
| --- | --- | --- | --- | --- |
| ~~`GET /v1/users`, `/v1/user-profiles`~~ | — | — | — | **Removed** ([lfx-mentorship#153](https://github.com/linuxfoundation/lfx-mentorship/pull/153)). They returned every user's identity fields and profile PII to any authenticated caller, with nothing for the edge to check |
| `GET /v1/users/{id}`, `/v1/user-profiles/{id}`, `/v1/user-profiles/slug/{slug}` | required | — | `allow_all` | Reads of a public-ish profile; no relation exists to check |
| ~~`POST /v1/users`, `POST /v1/user-profiles`~~ | — | — | — | **Removed** ([lfx-mentorship#153](https://github.com/linuxfoundation/lfx-mentorship/pull/153)). Rule 3 could not be satisfied: the body-supplied `id`/`user_id` and the token's `principal` live in different identifier spaces. Creation returns as the `/v1/me` profile-sync upsert and `/v1/me` profile routes |
| `PATCH`/`DELETE /v1/users/{id}` → **`/v1/me`** | required | — | `allow_all` | Reshape per decision 7. The by-ID routes are already removed ([lfx-mentorship#153](https://github.com/linuxfoundation/lfx-mentorship/pull/153)); the `/v1/me` replacements are the follow-up. No target ID means no check; `principal` settles it |
| `PATCH`/`DELETE /v1/user-profiles/{id}` → **`/v1/me/profile`** | required | — | `allow_all` | As above — removed in the same PR, replacements in the follow-up |
| `GET /v1/users/{userId}/applications` → **`/v1/me/applications`** | required | — | `allow_all` | Filter by `principal`. As a `{userId}` route it is uncheckable *and* lets any caller read another user's applications |

## Program routes

| Route | Auth | Object | Relation | Service must also |
| --- | --- | --- | --- | --- |
| `POST /v1/programs` | required | — | `allow_all` | **Per AQ-6 the create route does not check `mentorship_program_creator`** — any authenticated user creates, and approval publishes. The relation stays in the model so this can tighten to a `project:{uid}` check later without a model migration |
| `PATCH /v1/programs/{uid}` | required | `mentorship_program:{uid}` | `writer` | **Metadata only — must reject a `status` field** rather than ignoring it, so a smuggled transition fails loudly ([04 §decision 6](./04-authorization-model.md)) |
| `POST /v1/programs/{uid}/submit` | required | `mentorship_program:{uid}` | `writer` | New route (`draft → submitted`) |
| `POST /v1/programs/{uid}/decision` | required | `mentorship_approver_team:global` | `member` | New route (`submitted → published \| rejected`). **The one rule whose object is static**, not extracted from the path — approval is deliberately outside the program's own relations so admins cannot approve their own programs |
| `DELETE /v1/programs/{uid}` | required | `mentorship_program:{uid}` | `writer` | Emit `delete_access` for the program **and** every application and task beneath it, or their tuples are orphaned |
| `POST /v1/programs/{uid}/skills` | required | `mentorship_program:{uid}` | `writer` | — |
| `DELETE /v1/programs/{uid}/skills/{skillId}` | required | `mentorship_program:{uid}` | `writer` | Parent-child invariant (rule 2) — already enforced via `AND program_id = $2` |
| `POST /v1/programs/{uid}/members` | required | `mentorship_program:{uid}` | `writer` | Emits `member_put` on accept, not here — a pending invitation has no tuple ([04 §decision 4](./04-authorization-model.md)) |
| `PATCH /v1/programs/{uid}/members/{memberId}` | required | `mentorship_program:{uid}` | `writer` | Parent-child invariant (rule 2) — already enforced |
| `DELETE /v1/programs/{uid}/members/{memberId}` | required | `mentorship_program:{uid}` | `writer` | As above. Emit `member_remove` **naming the relation** (`mentor` or `writer`) |

## Term routes

Terms are not an FGA type and the current paths expose no program UID, so all of these nest under the parent ([04 §decision 7](./04-authorization-model.md)).

| Route | Auth | Object | Relation | Service must also |
| --- | --- | --- | --- | --- |
| `GET /v1/program-terms/{id}` → **`/v1/programs/{uid}/terms/{id}`** | anonymous | `mentorship_program:{uid}` | `viewer` | Parent-child invariant (rule 2) |
| `POST /v1/programs/{uid}/terms` | required | `mentorship_program:{uid}` | `writer` | — |
| `PATCH`/`DELETE /v1/program-terms/{id}` → **`/v1/programs/{uid}/terms/{id}`** | required | `mentorship_program:{uid}` | `writer` | Parent-child invariant (rule 2) |
| `GET /v1/program-terms/{id}/applications` → **nested** | required | `mentorship_program:{uid}` | `manager` | **The program's `manager`** (`writer or mentor`) — admins and mentors, deliberately not `auditor`, which the applicant holds |
| `GET /v1/program-terms/{id}/applications/export` → **nested** | required | `mentorship_program:{uid}` | `manager` | As above |
| `POST /v1/program-terms/{id}/applications/bulk-decline` → **nested** | required | `mentorship_program:{uid}` | `writer` | Admins only — a bulk status change, so `writer`, not the wider `manager` |
| `GET /v1/program-terms/{id}/past-mentees` → **nested** | required | `mentorship_program:{uid}` | `manager` | — |
| `GET /v1/program-terms/{id}/tasks` → **nested** | required | `mentorship_program:{uid}` | `manager` | — |
| `POST /v1/program-terms/{id}/applications` → **nested** | required | `mentorship_program:{uid}` | `viewer` | **The apply route.** Authorized by authentication plus program visibility, not by a grant — the applicant is not yet related to the program. Bind the applicant to `principal` (rule 3); the application window is a Postgres rule |

## Application routes

| Route | Auth | Object | Relation | Service must also |
| --- | --- | --- | --- | --- |
| `GET /v1/applications/{uid}` | required | `mentorship_application:{uid}` | `auditor` | Admits the applicant — which is why the reviewer note is its own route |
| `PATCH /v1/applications/{uid}` | required | `mentorship_application:{uid}` | `writer` | **Content only — must reject `status`.** Split per decision 6 |
| `PATCH /v1/applications/{uid}/status` | required | `mentorship_application:{uid}` | `manager` | New route. **The application's `manager`** = `writer from mentorship_program` — admins only, excluding mentors |
| `POST /v1/applications/{uid}/withdraw` | required | `mentorship_application:{uid}` | `mentee` | New route. The `pending`-only limit is a workflow rule in the service |
| `POST /v1/applications/{uid}/withdraw-for-mentee` | required | `mentorship_application:{uid}` | `manager` | New route. Staff-assisted, permitted in any state |
| `POST /v1/applications/{uid}/reapply` | required | `mentorship_application:{uid}` | `mentee` | New route |
| `PUT /v1/applications/{uid}/evaluation` | required | `mentorship_application:{uid}` | `reviewer` | New route. Mentors and admins, **not** the applicant |
| `GET`/`PUT /v1/applications/{uid}/note` | required | `mentorship_application:{uid}` | `reviewer` | New route. Its own route precisely because `auditor` (which the applicant holds) may fetch the application |
| `DELETE /v1/applications/{uid}` | required | `mentorship_application:{uid}` | `manager` | Emit `delete_access` for the application **and** its tasks |

## Task routes

| Route | Auth | Object | Relation | Service must also |
| --- | --- | --- | --- | --- |
| `POST /v1/applications/{uid}/tasks` | required | `mentorship_application:{uid}` | `reviewer` | **`reviewer`, not `manager`** — the application's `manager` excludes mentors, but assigning tasks is a mentor capability ([04 §decision 7](./04-authorization-model.md)) |
| `GET /v1/applications/{uid}/tasks` | required | `mentorship_application:{uid}` | `auditor` | Admits the mentee, who must see their own tasks |
| `GET /v1/tasks/{uid}` | required | `mentorship_task:{uid}` | `auditor` | `assignee or manager` |
| `PATCH /v1/tasks/{uid}` | required | `mentorship_task:{uid}` | `manager` | **Content only — must reject submission and review fields.** Split per decision 6 |
| `PATCH /v1/tasks/{uid}/submission` | required | `mentorship_task:{uid}` | `assignee` | New route — the mentee |
| `PATCH /v1/tasks/{uid}/review` | required | `mentorship_task:{uid}` | `manager` | New route — mentors and admins (`reviewer from mentorship_application`) |
| `DELETE /v1/tasks/{uid}` | required | `mentorship_task:{uid}` | `manager` | — |

## File routes

New routes, none of which exist today. The storage contract is [02 §object storage](./02-target-architecture.md#object-storage): uploads go through the API, public and private files live in separate buckets. **Authorization is per file class, and the class determines the bucket** — a public-class upload writing to the private bucket (or the reverse) is an access bug the edge cannot catch, so the handler must pick the bucket from the route, never from a request field.

**These routes are the only writers of the *file* fields.** Worth stating because the generic endpoints currently defeat it: program create/update accepts `logo_url`, user create/update accepts `avatar_url`, profile create/update accepts `logo_url` and a raw `profile_links` blob, and `PATCH /v1/tasks/{uid}` accepts `file` (`TaskUpdateInput.File`) — the last of which the task section above already says must be rejected, for the same reason. Any of them can persist an arbitrary URL, or a private key, with none of the content-type checks, the size cap, or the route-selected bucket above, which leaves the bucket rule advisory rather than enforced.

The reservation is **per field, not per column**. `logo_url`, `avatar_url`, and `tasks.file` are wholly file-owned and the generic endpoints reject them outright. `profile_links` is not: it is `{resumeLink, linkedinProfileLink, githubProfileLink}` (`backend/db/migrations/001_initial.up.sql:69`), and only `resumeLink` is a file reference. Rejecting the whole blob would take LinkedIn and GitHub links down with it, so a generic profile write that carries **`resumeLink`** is rejected — that field alone — and the other two links pass through. Rejecting rather than silently stripping is deliberate: a caller that sent the field would otherwise get a success response for a write that did not happen.

The handler side is the platform baseline, not invented here: read [lfx-object-store-design](https://github.com/linuxfoundation/lfx-skills/blob/main/skills/lfx-object-store-design/SKILL.md) for the upload/download flow, body encoding (raw for these singletons, not multipart), the SDK and chart contract, and the local stack. This table adds only what is Mentorship-specific — which relation guards each route, and which bucket each class writes to.

| Route | Auth | Object | Relation | Service must also |
| --- | --- | --- | --- | --- |
| `POST /v1/programs/{uid}/logo` | required | `mentorship_program:{uid}` | `writer` | Public class. Image allowlist, **SVG excluded**. Returns `public_url` **when `CDN_URL_PREFIX` is set** — the field is omitted in the no-CDN mode this matrix supports, rather than returned empty |
| `DELETE /v1/programs/{uid}/logo` | required | `mentorship_program:{uid}` | `writer` | Null the column; `DeleteObject` is idempotent |
| `PUT /v1/me/profile/logo`, `PUT /v1/me/avatar` | required | — | `allow_all` | Public class, self-scoped. **No object to check — the target is the principal's own profile**, so the handler must resolve it from `principal` and never from a body field (rule 3). Image allowlist, **SVG excluded**. Which profile row `/me/profile/` names is not yet decided — see RM-5 |
| `PUT /v1/me/profile/resume` | required | — | `allow_all` | **Private class**, self-scoped, same `principal` rule. PDF/doc allowlist |
| `GET /v1/me/profile/resume` | required | — | `allow_all` | Owner reads their own. Streams from the private bucket |
| `DELETE /v1/me/profile/logo`, `DELETE /v1/me/avatar`, `DELETE /v1/me/profile/resume` | required | — | `allow_all` | Self-scoped removal, same `principal` rule. Null the column and `DeleteObject` the current key. Without these there is no way to withdraw a resume, which a private PII class has to support |
| `GET /v1/applications/{uid}/resume` | required | `mentorship_application:{uid}` | `reviewer` | **The reviewer read path, and the one row here worth arguing about.** A resume is profile-level but is read *because* someone is reviewing an application, so the object checked is the application, not the profile — there is no profile type in the model and adding one to express "a reviewer may read this" would duplicate the application relation. The relation is `reviewer` (`manager or mentor from mentorship_program`, [04](./04-authorization-model.md)), **not `manager`** — on this type `manager` resolves to `writer from mentorship_program`, admins only, which would lock out the mentors whose review is the route's whole purpose (see rule 3 below). Consequence: the route takes an application UID, not the profile UID the legacy path suggests (see RM-4). **It is the *applicant's* resume, not the mentee's** — `applications.role` is `mentor \| mentee` (`001_initial.up.sql:182`) and `reviewer` authorizes both, so the profile is selected from `application.role`, which also settles which of the applicant's profiles supplies it (RM-5) |
| `PUT /v1/tasks/{uid}/submission/file` | required | `mentorship_task:{uid}` | `assignee` | **Private class.** PDF/doc allowlist, same as the resume routes. The mentee uploads; only before review closes, the same state rule as the `DELETE` below. Pairs with `PATCH /v1/tasks/{uid}/submission` in the task section |
| `GET /v1/tasks/{uid}/submission/file` | required | `mentorship_task:{uid}` | `auditor` | Admits the assignee and the reviewers. `Content-Disposition: attachment`, `Range` pass-through |
| `DELETE /v1/tasks/{uid}/submission/file` | required | `mentorship_task:{uid}` | `assignee` | Only before review closes — a state rule, service-side |
| `GET /v1/programs/{uid}/logo` | anonymous | `mentorship_program:{uid}` | `viewer` | Public class. Matches `GET /v1/programs/{uid}`, which already returns `logo_url` to the same callers |
| `GET /v1/user-profiles/{uid}/logo`, `GET /v1/users/{uid}/avatar` | anonymous | — | `allow_all` | Public class. Directory profiles have no model type (RM-2), so there is no object to check — same shape as `GET /v1/mentees/{id}`. The service must serve these only for publicly-listable profiles |

Five things to note. First, **the public classes have service `GET` routes too.** The CDN does not replace them — it supplements them ([02 §object storage](./02-target-architecture.md#object-storage)): when `CDN_URL_PREFIX` is unset (local development without the gateway stand-in, or a CDN outage) the service route is the only path to the bytes. Each carries the same relation as the record that holds the URL, but the CDN path enforces no relation at any point — it serves the object to anyone holding the URL from the moment it is uploaded (fifth note). **No *private*-class download route may be `anonymous`** — there the bucket split is the whole point, and an anonymous route would be a way around it.

Second, the 20 MB cap and the content-type allowlist are service concerns on **every** upload route — Heimdall authorizes the caller, not the payload — so the per-row notes name only what differs by class (image allowlist with SVG excluded for the public classes, PDF/doc for the private ones — resumes and task submissions alike).

Third, **SVG is excluded from every image allowlist**: S3 serves it as executable content, making it a stored-XSS vector on any origin that renders it.

Fourth, **the `/me/profile/` routes need a canonical-profile rule.** `user_profiles` has no `UNIQUE(user_id)` — only a plain index (`backend/db/migrations/001_initial.up.sql:251`) — and `profile_type` is `mentor | mentee`, so one user may hold both rows and "the principal's own profile" does not name one of them. The avatar is unambiguous (it lives on `users`, one per user); the profile logo and the resume are not. This has to be settled before the handlers are designed, not left to them (RM-5).

Fifth, **a public-class object stays retrievable after the record stops being visible.** Unpublishing a program re-emits `update_access` without `public`, which revokes the `user:*` tuple and makes the service `GET` start denying — but the CDN keeps serving the object until its `max-age=86400` expires, and anyone already holding the URL can refetch it regardless. So the two paths *can* diverge, and the bound is: **a public-class object is public from the moment it is uploaded; unpublishing shortens discovery, not access.** That is acceptable for logos and avatars, which are public by intent, and it is precisely why resumes and submissions are private-class rather than public-class with a relation in front. Making public objects genuinely revocable would mean short-expiry signed URLs plus a CDN invalidation on every visibility transition — a different delivery model (RM-6).

## What this matrix surfaces

Writing the table out is where the remaining decision-7-shaped exceptions appear. Four are worth an explicit call:

1. **`GET /v1/users` and `GET /v1/user-profiles` returned every user to any authenticated caller.** Neither had an object to check, so the gateway could not have fixed it — the edge would have waved both through. It was the one finding here that was a live authorization gap rather than a reshape, independent of the Heimdall work, and it is now closed: [lfx-mentorship#153](https://github.com/linuxfoundation/lfx-mentorship/pull/153) removed both routes (RM-1).
2. **Ten routes are new** (`submit`, `decision`, `status`, `withdraw`, `withdraw-for-mentee`, `reapply`, `evaluation`, `note`, `submission`, `review`) and fourteen more move (the `/me` reshapes and the nested term routes). None is a model change; all are prerequisites landed *before* the cutover flag, which is why [05](./05-heimdall-gateway.md) step 6 is not a pure configuration change.
3. **`manager` is the trap in this table.** It resolves to three different sets depending on type — `writer or mentor` on the program, `writer from mentorship_program` (admins only) on the application, `reviewer from mentorship_application` on the task. A RuleSet that names the relation without the type, or copies a row between sections, silently widens or narrows access. Every rule must name both.
4. **The `optional` authenticator has no successor and should not get one.** Its only job today is the hidden-program owner case, which `auditor` already expresses. Carrying `optionalJWT` forward would re-introduce a service-side visibility decision that the model is meant to own.

## Open questions

| # | Question | Proposed default |
| --- | --- | --- |
| RM-1 | ~~Do `GET /v1/users` and `GET /v1/user-profiles` stay on the gateway host at all?~~ **Resolved — removed.** | Both routes are deleted in [lfx-mentorship#153](https://github.com/linuxfoundation/lfx-mentorship/pull/153), along with the ID-addressed identity writes ([05](./05-heimdall-gateway.md) GW-5). They had no checkable object and no legitimate caller; fixing the leak was not gated on Heimdall |
| RM-2 | `GET /v1/mentees/{id}` and `/v1/mentors/{id}` are directory profiles with no model type. Leave them `allow_all`, or give them one? | **Leave them.** They expose only publicly-listable records, and adding a type to express "is public" duplicates what the service filter already does — the `user:*` wildcard is for objects that have a private state, which these do not |
| RM-3 | Does `POST /v1/program-terms/{id}/applications` check `viewer` on the program, or `allow_all` plus a service-side visibility check? | **`viewer`.** It is ID-addressed and the program is in the path once nested, so there is a real object to check; `allow_all` would let a caller apply to a hidden or archived program and rely on the service to notice |
| RM-4 | Reviewer access to a mentee's resume is checked on `mentorship_application`, so the route needs an application UID rather than the profile UID its legacy path suggests. Confirm the shape when the file handlers are designed. | Key it by application, with relation `reviewer`. The alternative — a profile type existing only to carry this one relation — adds a model type to express something the application relation already says |
| RM-5 | A user may hold both a mentor and a mentee profile row, so `PUT /v1/me/profile/logo` and `/resume` do not name a single row. Which profile is canonical for each file class? | **Open.** The undecided part is *which row*, not the spelling, and rewriting the paths to `/v1/user-profiles/{uid}/...` would move the ambiguity into a path parameter rather than resolve it — a caller still has to know which of their two profile UIDs to pass. What is settled: a "most recently updated profile" tiebreak is rejected, since it silently writes to the wrong row; `/me/avatar` is unaffected, because avatars live on `users` and there is exactly one row; and the reviewer read path is already decided independently (RM-4 keys it by application, and `applications.role` selects the profile). Once the canonical row is chosen, the route spelling follows from it — either an explicit `{uid}` or a `/me/` path with a documented resolution rule — and this table is updated then |
| RM-6 | Public-class objects stay retrievable from the CDN after the owning record is unpublished. Accept that, or move to signed URLs with invalidation on visibility transitions? | **Accept it.** Logos and avatars are public by intent, and the alternative adds signing, short expiry, and an invalidation call to every transition — for content that was already public. Anything that must be revocable belongs in the private class instead |
| RM-7 | The file routes here are spelled `PUT /v1/me/profile/logo` and `PUT /v1/tasks/{uid}/submission/file`; the platform singleton baseline in [lfx-object-store-design](https://github.com/linuxfoundation/lfx-skills/blob/main/skills/lfx-object-store-design/SKILL.md) is `POST /resources/{uid}/logo-upload`, `GET /resources/{uid}/logo-download`, `DELETE /resources/{uid}/logo`. Adopt the baseline spelling, or keep the resource-nested one? | **Keep the resource-nested spelling** — it is what V2 actually does. No shipped service uses the `-upload`/`-download` suffix in a URL path: [member-service](https://github.com/linuxfoundation/lfx-v2-member-service) serves `POST /b2b_orgs/{uid}/logo`, [committee-service](https://github.com/linuxfoundation/lfx-v2-committee-service) `POST /committees/{uid}/documents` and `GET /committees/{uid}/documents/{document_uid}/download`, and [project-service](https://github.com/linuxfoundation/lfx-v2-project-service) and [meeting-service](https://github.com/linuxfoundation/lfx-v2-meeting-service) follow the same shape. The suffix does appear in generated Goa **method names** (`UploadB2bOrgLogo`), which is where the baseline's spelling most likely comes from — but a method identifier is not a URL. So the "shared SDK cost" argued against nesting is not real: the SDKs are generated from these nested paths. Treat the skill's suffix as illustrative rather than normative, and raise it there rather than renaming working routes to match a spelling no service uses. Note this is precedent overriding the written baseline, which by the rule in [02 §object storage](./02-target-architecture.md#object-storage) is a difference to raise upstream, not to adopt silently |
