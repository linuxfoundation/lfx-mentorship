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
| **Auth** | `anonymous` — no token required (Heimdall's `anonymous_authenticator`). `required` — a valid OIDC token. `optional` — formerly `optionalJWT`, now removed; see the note on visibility below |
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
| `GET/POST /v1/admin/approver-team/members`, `DELETE /v1/admin/approver-team/members/{userID}` | — | — | — | **Not routed through the gateway.** Approver-team administration is cluster-local until a platform staff relation is approved; the service keeps its `manage:mentorship:approvers` scope check as defence in depth |

## Public catalog reads

Public collections are service-owned and reach the gateway through `allow_all` rules: there is no object UID to check, so each service pins what is public — published programs and publicly listable profiles only. Caller-owned collections (a user's programs, applications, or tasks) are served by Query Service with `filter_grants=direct` and have no RuleSet rule. Beside the collections sit the public slug→UID resolver, directory profile reads, and UID-addressed reads, which keep the wildcard `viewer` check.

| Route | Auth | Object | Relation | Service must also |
| --- | --- | --- | --- | --- |
| `GET /v1/programs` | anonymous | — | `allow_all` | **Pin `status = published` in the service**, whatever the caller sends — the edge performs no object check |
| `GET /v1/programs/catalog` | anonymous | — | `allow_all` | **Pin `status = published` in the service**, whatever the caller sends — the edge performs no object check. Nested terms, skills, and mentors must stay public fields |
| `GET /v1/programs/resolve/{id}` | anonymous | — | `allow_all` | **The slug→UID resolver, and the one route that must accept a slug.** Rule 1 depends on it existing. It must not leak non-public programs: resolve only to UIDs the caller could read anyway |
| `GET /v1/programs/{uid}` | anonymous | `mentorship_program:{uid}` | `viewer` | — |
| `GET /v1/programs/{uid}/catalog` | anonymous | `mentorship_program:{uid}` | `viewer` | — |
| `GET /v1/programs/{uid}/header` | anonymous | `mentorship_program:{uid}` | `viewer` | — |
| `GET /v1/programs/{uid}/skills` | anonymous | `mentorship_program:{uid}` | `viewer` | — |
| `GET /v1/programs/{uid}/mentees` | anonymous | `mentorship_program:{uid}` | `viewer` | Keep the accepted/graduated filter — `viewer` admits the public, so the *set* of mentees returned is a payload decision, not an access one |
| `GET /v1/programs/{uid}/members` | anonymous | `mentorship_program:{uid}` | `viewer` | Keep the email redaction ([05](./05-heimdall-gateway.md) GW-9, fixed in [linuxfoundation/lfx-mentorship#144](https://github.com/linuxfoundation/lfx-mentorship/pull/144)) — withheld from every caller this route admits, so redaction is correct here and a route split would be wrong |
| `GET /v1/programs/{uid}/funding-stats` | anonymous | `mentorship_program:{uid}` | `viewer` | — |
| `GET /v1/programs/{uid}/terms` | anonymous | `mentorship_program:{uid}` | `viewer` | — |
| `GET /v1/programs/{uid}/transactions` | anonymous | `mentorship_program:{uid}` | `viewer` | — |
| `GET /v1/programs/{uid}/sponsors` | anonymous | `mentorship_program:{uid}` | `viewer` | — |
| `GET /v1/mentees`, `/v1/mentors` | anonymous | — | `allow_all` | Return only publicly listable profiles — display fields only, never contact details |
| `GET /v1/mentees/summary`, `/v1/mentors/summary`, `/v1/summary`, `/v1/funding-stats/total` | anonymous | — | `allow_all` | Aggregate counts and totals, computed over published programs only — including the funding total, so non-public programs' funding cannot be inferred; `/v1/summary` also returns a public preview of recently graduated mentees (name and avatar). The summaries are listed in the rule explicitly rather than relying on the `/mentees/:id` and `/mentors/:id` profile rule matching `summary` as an ID |
| `GET /v1/mentees/{id}`, `/v1/mentors/{id}` | anonymous | — | `allow_all` | These are directory profiles, not model types. Nothing in the model keys on them, so there is no check to make — the service must return only publicly-listable records |

**The `optional` authenticator disappears.** Five routes used `optionalJWT` (`resolve`, `{id}`, `mentees`, `transactions`, `sponsors`), and the service no longer registers it; it existed solely so `resolveVisibleProgram` can let a `hidden` program's owner still fetch it. Behind Heimdall that is a relation, not a token check: `viewer` is `[user:*] or auditor`, and the owner holds `writer` → `manager` → `auditor`. So the rule is `oidc` **then** `anonymous_authenticator` — an anonymous caller fails the wildcard only when the program is not public, and the owner passes via `auditor`. The service's `hidden`-owner special case then has nothing left to do, provided the archived/hidden transitions re-emit `update_access` **without** `public` as [04 §lifecycle](./04-authorization-model.md) requires. If that emission is missed, a hidden program stays publicly readable and this row is the leak.

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
| `GET /v1/me/programs` | required | — | `allow_all` | Return only programs where `principal` is an active `program_admin` member. **An exception to the caller-owned-collection rule above, pending platform sign-off**: the admin list needs the current term, the open/completed grouping, and search, status filter, and paging across them, which the program index document does not yet carry |
| `GET /v1/me/mentor-programs` | required | — | `allow_all` | Return only `published` programs where `principal` is an active `mentor` member. **An exception to the caller-owned-collection rule above, pending platform sign-off**, as `/v1/me/programs` is: the mentor list needs the chosen term, the `term_status` grouping, and the term's mentee, applicant, and tasks-to-review counts, which the program index document does not carry |

## Program routes

| Route | Auth | Object | Relation | Service must also |
| --- | --- | --- | --- | --- |
| `POST /v1/programs` | required | — | `allow_all` | **Per AQ-6 the create route does not check `mentorship_program_creator`** — any authenticated user creates, and approval publishes. The relation stays in the model so this can tighten to a `project:{uid}` check later without a model migration |
| `PATCH /v1/programs/{uid}` | required | `mentorship_program:{uid}` | `writer` | **Metadata only — must reject a `status` field** rather than ignoring it, so a smuggled transition fails loudly ([04 §decision 6](./04-authorization-model.md)) |
| `POST /v1/programs/{uid}/submit` | required | `mentorship_program:{uid}` | `writer` | New route (`pending → submitted`) |
| `POST /v1/programs/{uid}/decision` | required | `mentorship_approver_team:global` | `member` | New route (`submitted → published \| rejected`). **The one rule whose object is static**, not extracted from the path — approval is deliberately outside the program's own relations so admins cannot approve their own programs |
| `DELETE /v1/programs/{uid}` | required | `mentorship_program:{uid}` | `writer` | Emit `delete_access` for the program **and** every application and task beneath it, or their tuples are orphaned |
| `POST /v1/programs/{uid}/skills` | required | `mentorship_program:{uid}` | `writer` | — |
| `DELETE /v1/programs/{uid}/skills/{skillId}` | required | `mentorship_program:{uid}` | `writer` | Parent-child invariant (rule 2) — already enforced via `AND program_id = $2` |
| `POST /v1/programs/{uid}/members` | required | `mentorship_program:{uid}` | `writer` | Emits `member_put` on accept, not here — a pending invitation has no tuple ([04 §decision 4](./04-authorization-model.md)) |
| `GET /v1/programs/{uid}/mentor-candidates` | required | `mentorship_program:{uid}` | `writer` | Invite typeahead. Exact email or LFID lookups reach auth-service over NATS, so only callers who can invite may search |
| `PATCH /v1/programs/{uid}/members/{memberId}` | required | `mentorship_program:{uid}` | `writer` | Parent-child invariant (rule 2) — already enforced |
| `DELETE /v1/programs/{uid}/members/{memberId}` | required | `mentorship_program:{uid}` | `writer` | As above. Emit `member_remove` **naming the relation** (`mentor` or `writer`) |
| `POST /v1/programs/{uid}/members/{memberId}/resend-invite` | required | `mentorship_program:{uid}` | `writer` | Only an `invited` mentor on a `published` program; signs a fresh token and re-sends `mentor_invited`. No tuple change |

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
| `PATCH /v1/tasks/{uid}` | required | `mentorship_task:{uid}` | `manager` | Mentors' and admins' full edit: content, `submit_file`, and any `status` from any `status` ([lfx-mentorship#227](https://github.com/linuxfoundation/lfx-mentorship/issues/227)). **Must reject the assignee's `file` and the denormalised `application_status`/`program_term_status`**, and refuse the assignee — split per decision 6 |
| `PATCH /v1/tasks/{uid}/submission` | required | `mentorship_task:{uid}` | `assignee` | New route — the mentee |
| `PATCH /v1/tasks/{uid}/review` | required | `mentorship_task:{uid}` | `manager` | New route — mentors and admins (`reviewer from mentorship_application`) |
| `DELETE /v1/tasks/{uid}` | required | `mentorship_task:{uid}` | `manager` | — |

## File routes

New routes, none of which exist today. The storage contract is [02 §object storage](./02-target-architecture.md#object-storage): uploads go through the API, public and private files live in separate buckets, and every upload writes a fresh key. Route spelling follows the skill's singleton pattern — `POST …/X-upload`, `GET …/X-download`, `DELETE …/X` (RM-7). **Authorization is per file class, and the class determines the bucket**: the handler picks the bucket from the route, never from a request field.

**These routes are the only writers of the file fields**, and the generic endpoints must reject those fields — program create/update (`logo_url`) and `PATCH /v1/tasks/{uid}` (`file`) otherwise persist arbitrary URLs with no content-type check, no size cap, and no route-selected bucket. Profile logos and avatars are not file classes ([02 §file classes](./02-target-architecture.md#file-classes)): another service hosts profile logos, so profile create/update accept `logo_url` as given, and `PATCH /v1/me` accepts `avatar_url`. `profile_links.resumeLink` is not accepted at all — resumes are not a file class ([02 §file classes](./02-target-architecture.md#file-classes)) — while `linkedinProfileLink` and `githubProfileLink` pass through the generic profile routes unchanged (`backend/db/migrations/001_initial.up.sql:69`).

Handler mechanics are the platform baseline ([lfx-object-store-design](https://github.com/linuxfoundation/lfx-skills/blob/main/skills/lfx-object-store-design/SKILL.md)). This table adds only the relation guarding each route and the bucket each class writes to. Profile routes hang off `/v1/me/profiles/by-id/{id}`, the existing self-service profile path: it names exactly one row — a user may hold several mentor profiles — and its handler already checks that the row belongs to the principal — the one approved ownership check ([02 §authorization](./02-target-architecture.md#authorization), RM-5).

| Route | Auth | Object | Relation | Service must also |
| --- | --- | --- | --- | --- |
| `POST /v1/programs/{uid}/logo-upload` | required | `mentorship_program:{uid}` | `writer` | Public class. Image allowlist, **SVG excluded**, 2 MB. New key, conditional row update, previous key queued for deletion unless it is a foreign URL ([02 §stored value and delivery](./02-target-architecture.md#stored-value-and-delivery)). Returns the stored CDN URL as `public_url` |
| `DELETE /v1/programs/{uid}/logo` | required | `mentorship_program:{uid}` | `writer` | Null the column; queue the key for deletion ([02 §deleting the bytes](./02-target-architecture.md#deleting-the-bytes)) |
| `GET /v1/programs/{uid}/logo-download` | anonymous | `mentorship_program:{uid}` | `viewer` | Public class. Same relation as `GET /v1/programs/{uid}`, which already returns `logo_url` |
| `POST /v1/tasks/{uid}/file-upload` | required | `mentorship_task:{uid}` | `assignee` | **Private class.** PDF/doc allowlist, 20 MB. Only before review closes — a state rule, service-side. Pairs with `PATCH /v1/tasks/{uid}/submission` |
| `GET /v1/tasks/{uid}/file-download` | required | `mentorship_task:{uid}` | `auditor` | Admits the assignee and the reviewers. Filename is the key minus its `{uuid}-` prefix ([02 §stored value and delivery](./02-target-architecture.md#stored-value-and-delivery)). **This route, not `tasks.file`, is what the program-admin mentee listing returns per task** |
| `DELETE /v1/tasks/{uid}/file` | required | `mentorship_task:{uid}` | `assignee` | Only before submission (`incomplete` or `in_progress`), since a submitted task that requires a file must keep one (`backend/internal/service/task_service.go:261-263`); a submitted file is replaced through `file-upload`, not deleted. Null the column and queue the key for deletion |

Three things to note.

1. **A size cap and the content-type allowlist apply to every upload route, enforced on the payload bytes, not the declared `Content-Type`.** Heimdall authorizes the caller, not the payload. 20 MB is the platform ceiling; image classes cap at 2 MB ([02 §stored value and delivery](./02-target-architecture.md#stored-value-and-delivery)). The allowlists are the legacy accept lists less SVG (next note): **logos** PNG and JPEG (`lfx-mentorship-upgrade/src/app/shared/logo-field/logo-field.component.html:13`); **task submissions** PDF, DOC, DOCX and plain text — legacy offers `.txt` on the built-in Coding Challenge task (`lfx-mentorship-upgrade/src/app/shared/tasks/task/task.component.ts:254-258`), so it is allowed for the class or the migration would quarantine those submissions. Identify the format from content — the file signature for PNG, JPEG, PDF and DOC, structural validation for DOCX, whose ZIP signature is shared with other formats, and for plain text, which has none (valid UTF-8, no NUL bytes) — and reject a payload that fails either check. A signature identifies a format, not the client's name — DOC's compound-file signature is shared with XLS, PPT and MSI — so the download name's extension comes from the identified type, never the client ([02 §stored value and delivery](./02-target-architecture.md#stored-value-and-delivery)).
2. **SVG is excluded from every image allowlist.** S3 serves SVG as executable content, making it a stored-XSS vector on any origin that renders it. It is also why the migration quarantines rather than drops legacy `image/svg+xml` logos ([03 §S3 objects](./03-migration-plan.md#migration-specific-tasks)).
3. **The public-class `GET` routes exist because the skill requires them, not because anything depends on them.** Stored values always hold the CDN URL — `LOGOS_CDN_URL_PREFIX` is required configuration — so these routes are an explicitly addressed fallback. A foreign URL has no object behind it: the route answers `404` and never fetches it, which would be an SSRF path. **No private-class download route may be `anonymous`.** And unpublishing a record does not revoke its logo: the object is public from the moment of upload and stays retrievable from the CDN for as long as it exists, and edge caches can serve it for up to `max-age=86400` after it is deleted (RM-6).

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
| RM-5 | A user may hold a mentee profile and any number of mentor profiles, so a `/v1/me/profile/...` file route would not name a single row. Which profile is canonical for each file class? | **Resolved.** The file routes hang off `/v1/me/profiles/by-id/{id}`, the existing self-service profile path that names one row and already checks ownership. No by-type variant is added |
| RM-6 | Public-class objects stay retrievable from the CDN after the owning record is unpublished. Accept that, or move to signed URLs with invalidation on visibility transitions? | **Accept it.** Logos are public by intent, and the alternative adds signing, short expiry, and an invalidation call to every transition — for content that was already public. Anything that must be revocable belongs in the private class instead |
| RM-7 | The file routes were first drafted as `PUT /v1/me/profile/logo` and `PUT /v1/tasks/{uid}/submission/file`; the platform singleton baseline in [lfx-object-store-design](https://github.com/linuxfoundation/lfx-skills/blob/main/skills/lfx-object-store-design/SKILL.md) is `POST /resources/{uid}/logo-upload`, `GET /resources/{uid}/logo-download`, `DELETE /resources/{uid}/logo`. Adopt the baseline spelling, or keep the resource-nested one? | **Resolved: adopt the baseline spelling** (`logo-upload`, `logo-download`, `DELETE …/logo`; `file-upload`, `file-download`, `DELETE …/file` for tasks). Nothing is built yet, and Crowdfunding's presigned-URL route is not a precedent, since the skill forbids presigned uploads |
