<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Mentorship Rewrite — 00: Current Authorization Relations

Status: Proposal — for Architecture team review
Related: [01-current-system.md](./01-current-system.md), [04-authorization-model.md](./04-authorization-model.md)

This document is a ground-truth capture of the access-control relations that exist in the **current, legacy** Mentorship platform (jobspring backend + lfx-mentorship-upgrade frontend) — what is actually enforced today, expressed as subject–relation–object statements, independent of any FGA design.

It exists to serve as the baseline for validating [04-authorization-model.md](./04-authorization-model.md): the proposed model should be checked against this document, not the other way around. Where this document and 04 disagree, that is either (a) an intentional, product-approved change in the rewrite, or (b) a gap in 04 that needs a decision. Neither this document nor 04 should be silently edited to make the other agree — see [Divergences to reconcile](#divergences-to-reconcile).

**Method**: derived by reading jobspring's actual enforcement code (not its comments or intent), lfx-mentorship-upgrade's role-gating code, and in-app product copy — not by reading `01-current-system.md`'s narrative lifecycle or `04-authorization-model.md` itself, to avoid anchoring on either. The legacy platform's own terms (`maintainer`, `apprentice`) are used here only where quoting stored data or code; prose uses the target terms (`program admin`, `mentee`) per this repo's terminology rules.

## Legend

- **Enforced (backend)** — jobspring rejects the action server-side if the relation doesn't hold.
- **UI-only** — lfx-mentorship-upgrade hides the control or route, but jobspring does not check it; a caller who reaches the endpoint directly is not blocked.
- **Unenforced** — no check exists anywhere, frontend or backend.

## Subjects and objects

| Type | What it is |
| --- | --- |
| `user` | An authenticated LFID/Auth0 identity. |
| `program` | A mentorship program (`projects` table). |
| `program_term` | A term/cohort within a program. |
| `application` | A mentee's or mentor's application to a program (mentee: `program-term-mentees`; mentor: a `project-members` row in `pending`/`declined` status). |
| `membership` | An accepted mentor or program-admin's standing relation to a program (`project-members` row in `approved`/`active`/`withdrawn` status). |
| `task` | A mentee task/milestone. |

## Relations captured

### program

| Relation | Holds when | Grants | Enforcement |
| --- | --- | --- | --- |
| `owner` | `program.LFID == user.LFID` | update, hide/unhide, invite mentors, remove mentor | Enforced (backend); duplicated inline at each call site, not a shared check. A `program_admin` membership row does **not** grant any of these — none of the four endpoints consult `project-members` at all, despite their error text reading "Only the maintainer can update the project". Nuance: internal event-driven `UpdateProject` calls (SQS path) skip the owner check entirely. UI mismatch: the Edit button is shown to any `program_admin` (`isMaintainer`), but the edit route's `ProjectOwnerGuard` compares LFIDs and bounces a program admin who is not the owner. |
| `program_admin` (membership) | `project-members` row, `member_type = maintainer`, **any status — including `pending`** (the query's status filter exists in the type but is never applied) | bulk-decline pending applications, update application notes, list applications, create/update/list program terms (all also grantable via `mentor` — see below) | Enforced (backend), via membership lookup |
| `mentor` (membership) | `project-members` row, `member_type = mentor`, **any status — including `pending`** | *backend*: identical grants to `program_admin` for terms and application handling — the backend never distinguishes the two member types for these actions. *UI*: mentors never see the Terms or Mentors tabs (tab links are `*ngIf="isMaintainer"`), and the mentee decision dropdown is read-only for them — so the admin/mentor distinction the product intends is **UI-only** for every one of these actions | Enforced (backend) only at the "some membership" level; the mentor-vs-admin distinction is **UI-only** |
| *(unauthenticated)* | — | **API**: list a program's approved members — all member types including mentees — with `email` and `name` in the JSON (`GET /{projectID}/member` has no auth middleware; pending applicants are excluded by a `status = approved` DB filter). **UI**: never renders these emails anywhere — public pages show only name/avatar/bio — so this is an API-level exposure, not a visible product behavior | **Unenforced** at the API; the UI's restraint is cosmetic |
| *(unauthenticated, by design)* | — | view a published program's public page: metadata, accepted mentors (name, avatar, bio, link to their public profile page), and — if the program enables `showAcceptedMentees` — accepted mentees (name, avatar, bio). No emails | Public by design |
| *(any authenticated user)* | — | create program (becomes `owner`); the LF project it sits under is free-form — the creator picks any project from a dropdown, with no check that they have any relation to it | **Unenforced** by design (anyone may propose a program; the LF-admin approval step is the actual control) |
| *(env allowlist)* | caller's LFID in `ALLOWED_APPROVERS` env var, or possesses a valid signed one-time email link | approve/decline program (publish) | Enforced (backend), but not a data-backed relation — see [Divergences](#divergences-to-reconcile) |

### application (mentee)

| Relation | Holds when | Grants | Enforcement |
| --- | --- | --- | --- |
| `applicant` | `application.user_id == user.id` | view own application status | Implicit (self-scoped by lookup, not an explicit check) |
| *(any program membership)* | caller holds a `mentor` or `program_admin` row on the program, any status | approve/decline the application (`UpdateMenteeStatus`) | **Enforced (backend)**, but the backend does not require `program_admin` — **UI-only** restricts the decision control to `program_admin` (mentors see a read-only status badge). Product copy: *"project admin gets notified via email to review the submission and make the admission decision. Mentors can assign tasks and milestones to accepted mentees."* (`mentees-tab.component.html`). This is the single clearest case in the platform of the backend being looser than the documented and UI-enforced intent. |

### application (mentor)

| Relation | Holds when | Grants | Enforcement |
| --- | --- | --- | --- |
| `applicant` | `application.user_id == user.id` | view own application status | Implicit |
| *(any program membership, with a carve-out)* | caller holds a `mentor` or `program_admin` row (any status, including `pending`); additionally, if the caller's *only* qualifying row is a `mentor` row, they may not act on any application but their own | approve/decline the mentor application (`UpdateMentorStatus`) | Enforced (backend) — note this rule is **not symmetric** with the mentee-application rule above. In practice the action is admin-only anyway because the Mentors tab (the only UI path to it) is hidden from non-admins — but that gate is the tab link alone; the tab component itself has no role check |

### membership (accepted mentor / program admin)

| Relation | Holds when | Grants | Enforcement |
| --- | --- | --- | --- |
| `program_admin` | `project-members` row, `member_type = maintainer` | remove a mentor from the program | Enforced (backend), via `owner` check (`program.LFID == user.LFID`), not the `program_admin` membership row itself — these are treated as interchangeable but are separately-stored facts (see [Divergences](#divergences-to-reconcile)) |
| `mentor` | `project-members` row, `member_type = mentor`, `status = active`/`approved` | view assigned mentee's tasks, contact info | Enforced (backend), correctly scoped to the specific program |
| `mentor` (UI only) | frontend `getProjectUserRoles` returns `mentor` | see "Mentors" management tab | **UI-only** — the tab link and its approve/decline/delete-mentor controls are hidden via `*ngIf`, but the route itself carries no role guard; a `mentor` (non-admin) who navigates there directly is only blocked by the backend `program_admin`-independent membership check above (i.e., not blocked at all beyond "some membership") |

### task

| Relation | Holds when | Grants | Enforcement |
| --- | --- | --- | --- |
| `owner`/`assignee`/`creator` | `task.owner_id`, `task.assignee_id`, or `task.created_by == user.id` | update task | Enforced (backend) |
| *(program membership, fallback)* | caller is a `mentor` or `program_admin` of the task's program | update task (if not owner/assignee/creator) | Enforced (backend) |
| self | `task.assignee_id == user.id` | view own current/past tasks | Enforced (backend), correctly project-scoped |
| *(program membership)* | caller is `mentor`/`program_admin` of the task's program(s) | view a mentee's tasks | Enforced (backend), correctly project-scoped |
| *(any authenticated user)* | — | create task for **any** program ID in the payload; list all tasks; list submitted tasks | **Unenforced** — no membership check on these three actions, unlike the sibling actions above |

### user profile

| Relation | Holds when | Grants | Enforcement |
| --- | --- | --- | --- |
| self | `profile.user_id == user.id` | update own profile | Implicit (ID comes from the caller's own JWT lookup, not request input) |
| *(mentor/program_admin, linked)* | caller holds a `mentor` or `program_admin` row on a program where the target user is a mentee | view the mentee's private profile fields | Enforced (backend), via a page-local `isLinked` helper |
| *(unauthenticated)* | target user is an accepted mentor on any published program | view the mentor's public profile page: avatar, name, bio, skills, GitHub/LinkedIn links, programs, mentees section — no email | Public by design (`/mentor/{id}` route has no auth guard, only an existence check) |
| self | `user.id == subject.id` | withdraw self from a program | **Unenforced** — `WithdrawMenteeFromProject` takes the target `UserID` from the request and never compares it to the caller; any authenticated user can withdraw an arbitrary mentee if they know/guess the ID |

### employer (vestigial, per [01-current-system.md](./01-current-system.md))

| Relation | Holds when | Grants | Enforcement |
| --- | --- | --- | --- |
| `owner` | `employer.LFID == user.LFID` | update employer | Enforced (backend), a separate code path from program `owner` despite the identical shape |

## Divergences to reconcile

These are cases where two things the platform does about "who can do X" don't match each other. Recorded here as observations for comparison against 04, not resolved:

1. **`owner` (LFID field-compare) and `program_admin` (membership row) are two disjoint permission sets, not interchangeable.** Program update/hide/invite/remove consult *only* the LFID field — a program-admin membership row grants none of them (the error messages saying "only the maintainer" are misleading) — while application/term handling consults *only* membership rows. The UI blurs this: the Edit button shows for any program admin, but the route guard then rejects non-owners.
2. **The mentor vs. program-admin distinction exists only in the UI.** For terms, application listing, and application decisions, the backend accepts either member type identically; the UI hides the Terms and Mentors tabs and locks the mentee decision control to admins. The product's stated intent (the mentee-application copy above) says only program admins make binding decisions — the rewrite should enforce that server-side.
3. **Mentee-application approval is enforced at the "any membership" level, but the product and UI both express program-admin-only intent.** This is the most concrete, product-copy-backed instance of backend enforcement being looser than documented intent.
4. **Mentor-application approval has an extra self-protection rule that mentee-application approval lacks**, despite both being "decide an application" actions.
5. **"Approve a program" has no data-backed relation at all** — it's an env-var allowlist or bare possession of a mailed link — so there is nothing in this document's relation vocabulary to map an FGA `approver`/`lf_admin` relation onto; it would be a new relation with no legacy precedent to validate against.
6. **Three actions on `task` (create, list-all, list-submitted) have no relation check whatsoever**, while their close siblings do. If 04 defines FGA checks for task creation/listing, there is no legacy behavior to compare them against — the legacy answer is "unrestricted."
7. **A `pending` membership row is as good as an approved one.** Every membership-based check (terms, application listing, application decisions) filters by member type only; the status filter was designed into the query type but never wired up, so a not-yet-approved mentor applicant already passes all of them. The rewrite must require an accepted/active membership.
8. **The member-list API leaks `email`/`name` of all approved members (including mentees) to anonymous callers**, while the UI never displays an email anywhere. The intended public surface — confirmed from the actual public pages — is: program metadata, accepted mentors (name/avatar/bio/public profile), and, when the program opts in, accepted mentees (name/avatar/bio). That, minus the emails, is the behavior to reproduce.
