<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Mentorship Rewrite — 00: Current Authorization Relations

Status: Proposal — for Architecture team review
Related: [01-current-system.md](./01-current-system.md), [04-authorization-model.md](./04-authorization-model.md)

A ground-truth capture of the access-control relations in the **current, legacy** Mentorship platform (jobspring backend + lfx-mentorship-upgrade frontend), expressed as subject–relation–object statements independent of any FGA design. It is the baseline for validating [04-authorization-model.md](./04-authorization-model.md): 04 is checked against this document, not the other way around. Where the two disagree, that is either an intentional product change or a gap in 04 — neither document should be silently edited to make the other agree.

**Method**: read from jobspring's enforcement code, the frontend's role-gating code, and in-app product copy — deliberately not from `01-current-system.md` or `04-authorization-model.md`, to avoid anchoring on either. Legacy terms (`maintainer`, `apprentice`) appear only when quoting stored data or code; prose uses the target terms (`program admin`, `mentee`).

**The spec is the product's observable behavior, not the legacy enforcement point.** The rewrite reproduces what a user can and cannot do; *where* legacy happens to check it is provenance, not a design input. So a rule legacy enforces only in Angular is still a rule the rewrite owes its users — implemented wherever it belongs server-side. Conversely, a capability the API exposes but the product never surfaces (the anonymous email leak, the unscoped withdraw endpoint) is a legacy defect, not behavior to carry forward. The Legend below therefore records where legacy put each check; it does not rank the rules.

## Legend

- **Enforced (backend)** — jobspring rejects the action server-side if the relation doesn't hold.
- **UI-only** — lfx-mentorship-upgrade hides the control or route, but jobspring does not check it; a caller who reaches the endpoint directly is not blocked. **These are still requirements** — see above.
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

**Status vocabulary** (`ProjectMemberStatus`, `project.go:1227-1249`) — one constant set is shared by both application types, but each validates a different subset:

| Application type | Accepted values | Terminal-accept term |
| --- | --- | --- |
| Mentee (`UpdateMemberRequest`, `project.go:1166`) | `pending`, `accepted`, `declined`, `withdrawn`, `graduated` | `accepted` |
| Mentor (`UpdateMentorRequest`, `project.go:1181`) | `pending`, `approved`, `declined`, `withdrawn` | `approved` |

So `accepted` and `approved` mean the same thing for different roles, and code spanning both must accept either (`service.go:3651`, `service_rebuild.go:243`). `rejected` and `hold` also exist as constants but appear in neither validator. Mentors have **no `graduated` status**. The rewrite should collapse this to one vocabulary — the repo's terminology rules already reserve `rejected` for program moderation and use `declined` for applications.

## Relations captured

### program

| Relation | Holds when | Grants | Enforcement |
| --- | --- | --- | --- |
| `owner` | `program.LFID == user.LFID` | update, hide/unhide, invite mentors, remove mentor | Enforced (backend); duplicated inline at each call site (`service.go:4057`, `service.go:4324`, `service_project_member.go:1107`), not a shared check. Reads as a second permission tier above `program_admin`, but **is not one in practice**: program creation writes the creator a `maintainer` membership row (`service.go:2669`), and that is the *only* site in the codebase that creates one — invitations create `mentor` rows only. So every program has exactly one admin, the creator, who always holds both the LFID match and the row. The two identities cannot come apart, which is why the redundancy is invisible. Nuance: internal event-driven `UpdateProject` calls (SQS path) skip the owner check entirely. UI mismatch: the Edit button is shown to any `program_admin` (`isMaintainer`), and the edit route's `ProjectOwnerGuard` compares LFIDs — harmless today only because the two always coincide. |
| `program_admin` (membership) | `project-members` row, `member_type = maintainer`, **any status — including `pending`** (the query's status filter exists in the type but is never applied) | bulk-decline pending applications, update application notes, list applications, create/update/list program terms (all also grantable via `mentor` — see below) | Enforced (backend), via membership lookup |
| `mentor` (membership) | `project-members` row, `member_type = mentor`, **any status — including `pending`** | *backend*: identical grants to `program_admin` for terms and application handling — the backend never distinguishes the two member types for these actions. *UI*: mentors never see the Terms or Mentors tabs (tab links are `*ngIf="isMaintainer"`), and the mentee decision dropdown is read-only for them — so a mentor cannot, in the product, manage terms or decide applications | Enforced (backend) only at the "some membership" level; the mentor-vs-admin distinction is **UI-only** in legacy. **Rewrite requirement**: terms and application decisions are admin-only, enforced server-side |
| *(unauthenticated)* | — | **API**: list a program's approved members — all member types including mentees — with `email` and `name` in the JSON (`GET /{projectID}/member` has no auth middleware; pending applicants are excluded by a `status = approved` DB filter). **UI**: never renders these emails anywhere — public pages show only name/avatar/bio | **Unenforced** at the API. Not reproduced: the product never exposes member emails publicly, so this is a leak to close, not behavior to carry forward (decision 5) |
| *(unauthenticated, by design)* | — | view a published program's public page: metadata, accepted mentors (name, avatar, bio, link to their public profile page), and two mentee sections — "Mentees" (accepted) and "Graduated Mentees" (paginated) — each name, avatar, bio. No emails. Both mentee sections render only while the program has applications open or an active term (`showAcceptedMentees` is a computed local flag, `project-public.component.ts:62`, not a program setting) | Public by design. The term-state gate on the mentee sections is **not** carried forward — the rewrite lists accepted and graduated mentees on any published program (decision 1) |
| *(any authenticated user)* | — | create program (becomes `owner`); the LF project it sits under is free-form — the creator picks any project from a dropdown, with no check that they have any relation to it | **Unenforced** by design (anyone may propose a program; the LF-admin approval step is the actual control) |
| *(env allowlist)* | caller's LFID in `ALLOWED_APPROVERS` env var, or possesses a valid signed one-time email link | approve/decline program (publish) | Enforced (backend), but not a data-backed relation — see [Divergences](#divergences-to-reconcile) |

### application (mentee)

| Relation | Holds when | Grants | Enforcement |
| --- | --- | --- | --- |
| `applicant` | `application.user_id == user.id` | view own application status | Implicit (self-scoped by lookup, not an explicit check) |
| *(any program membership)* | caller holds a `mentor` or `program_admin` row on the program, any status including `pending` | set any status on the application (`UpdateMenteeStatus`) — the admin dropdown offers all five of `accepted`/`declined`/`graduated`/`pending`/`withdrawn`, and neither UI nor backend restricts which transitions are legal from the current state | **Enforced (backend)**, but the backend does not require `program_admin` — the UI alone restricts the decision control to admins (mentors see a read-only status badge). Product copy: *"project admin gets notified via email to review the submission and make the admission decision. Mentors can assign tasks and milestones to accepted mentees."* (`mentees-tab.component.html`). The clearest case in the platform of the backend being looser than the documented, UI-enforced intent. **Rewrite requirement**: admin-only, enforced server-side. |
| `applicant` (self-withdraw) | URL `userID` on `PUT /mentees/{userID}/project/{projectID}/withdraw` | withdraw the mentee's application on that program | Authenticated, but **the caller's JWT is never compared to the URL `userID`** (`mentee/service.go:1231-1234`) — any logged-in user can withdraw any mentee, and the frontend feeds the endpoint `localStorage.getItem('userId')` (`project-card.component.ts:228`). The *from*-state is also inconsistent: the primary path filters `project-members` to `pending` only, while the fallback (`GetCurrentUserActiveApplicationsByUserAndProject`, `repository.go:5174`) matches `accepted`, `pending`, and `graduated`. In the product, withdrawal is offered only while `pending` — the Withdraw CTA renders on that status alone (`project-card.component.ts:261`), paired with a Re-Apply CTA for withdrawn applications. **Rewrite requirement**: that product rule, enforced server-side as a self-check plus a `pending` state check (decision 3) |

### application (mentor)

| Relation | Holds when | Grants | Enforcement |
| --- | --- | --- | --- |
| `applicant` | `application.user_id == user.id` | view own application status | Implicit |
| *(any program membership, with a carve-out)* | caller holds a `mentor` or `program_admin` row (any status, including `pending`); additionally, if the caller's *only* qualifying row is a `mentor` row, they may not act on any application but their own | approve/decline the mentor application (`UpdateMentorStatus`) | Enforced (backend) — note this rule is **not symmetric** with the mentee-application rule above. In practice the action is admin-only anyway because the Mentors tab (the only UI path to it) is hidden from non-admins — but that gate is the tab link alone; the tab component itself has no role check |

### membership

| Relation | Holds when | Grants | Enforcement |
| --- | --- | --- | --- |
| `mentor` | `project-members` row, `member_type = mentor`, `status = approved` | view assigned mentee's tasks and contact info | Enforced (backend), correctly scoped to the specific program — and one of the few checks that *does* test status |

Removing a mentor is an `owner` action, listed under [program](#program). In legacy that is a distinct check from the `program_admin` row, though both always describe the same person; the rewrite collapses them (decision 6).

One non-authorization constraint that interacts with the relations above: **a mentee may hold only one `accepted` mentorship at a time** (`service_program_term_mentees.go:152`, "Mentee can only have one accepted mentorship"). This is why self-withdrawal matters beyond convenience — a mentee who cannot exit an accepted mentorship cannot apply anywhere else.

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

### employer (vestigial, per [01-current-system.md](./01-current-system.md))

| Relation | Holds when | Grants | Enforcement |
| --- | --- | --- | --- |
| `owner` | `employer.LFID == user.LFID` | update employer | Enforced (backend), a separate code path from program `owner` despite the identical shape |

## Divergences to reconcile

These are cases where two things the platform does about "who can do X" don't match each other. Recorded here as observations for comparison against 04, not resolved:

1. **`owner` (LFID field-compare) and `program_admin` (membership row) are two mechanisms for one role.** Program update/hide/invite/remove consult *only* the LFID field, while application/term handling consults *only* membership rows — so on paper they are disjoint permission sets. In practice they always coincide: the creator is written a `maintainer` row at creation (`service.go:2669`), and nothing else in the codebase ever creates one, so a program has exactly one admin who holds both. The distinction is unreachable, and the "only the maintainer" error messages are misleading rather than wrong. **Resolved** — see decision 6.
2. **The mentor vs. program-admin distinction exists only in the UI.** For terms, application listing, and application decisions the backend accepts either member type identically, while the UI hides the Terms and Mentors tabs and locks the mentee decision control to admins. Mentee-application approval is the sharpest case, because the product copy states the boundary explicitly and the backend still doesn't enforce it. The rewrite enforces it server-side — the observable rule is unchanged, only its enforcement point moves.
3. **Mentor-application approval has an extra self-protection rule that mentee-application approval lacks** (a lone mentor may act only on their own application), despite both being "decide an application" actions.
4. **A `pending` membership row is as good as an approved one.** Every membership-based check (terms, application listing, application decisions) filters by member type only; the status filter was designed into the query type but never wired up, so a not-yet-approved mentor applicant already passes all of them. The rewrite must require an accepted/active membership.
5. **Self-withdrawal is neither self-scoped nor state-consistent.** The endpoint never compares the URL's `userID` to the caller (any authenticated user can withdraw any mentee), and its two code paths disagree on which states are withdrawable — `pending` on the primary path, but `accepted`/`pending`/`graduated` on the fallback. The UI meanwhile offers withdrawal only while `pending`. Three different answers to one question; the product's answer is the one that counts. **Resolved** — see decision 3.
6. **"Approve a program" has no data-backed relation at all** — it's an env-var allowlist or bare possession of a mailed link — so there is nothing in this document's relation vocabulary to map an FGA `approver`/`lf_admin` relation onto; it would be a new relation with no legacy precedent to validate against.
7. **Three actions on `task` (create, list-all, list-submitted) have no relation check whatsoever**, while their close siblings do. If 04 defines FGA checks for task creation/listing, there is no legacy behavior to compare them against — the legacy answer is "unrestricted."
8. **The member-list API leaks `email`/`name` of all approved members (including mentees) to anonymous callers**, while the UI never displays an email anywhere. The intended public surface — confirmed from the actual public pages — is: program metadata, accepted mentors (name/avatar/bio/public profile), and accepted plus graduated mentees (name/avatar/bio). **Resolved** — see decision 5.
9. **Two status vocabularies for one lifecycle** (`accepted` vs `approved`; see [Status vocabulary](#subjects-and-objects)) force several call sites to test for both. Not an authorization defect, but it makes "is this member accepted?" ambiguous in exactly the checks authorization depends on.

## Product decisions for the rewrite

Decisions made during review of this document (2026-09-04). These resolve divergences above and are inputs to validating [04-authorization-model.md](./04-authorization-model.md).

The governing rule: **replicate the product's observable behavior; choose the enforcement point freely.** A user's functional experience — what they can and cannot do — must match legacy. Where legacy enforced a rule only in the UI, the rewrite reproduces the rule server-side; where the legacy API allowed something the product never offered, the rewrite does not carry it forward.

1. **Public program-page visibility is asymmetric by role**, and gated on the program being published — not on term state:
   - **Mentors — accepted only.** Pending, invited-but-not-accepted, declined, and withdrawn mentors are never publicly listed (an invitee has not consented to a public affiliation). Mentors have no `graduated` state to include: the status constant is documented as *"Mentee has graduated the Mentorship program"* (`project.model.ts:348`) and the mentor validator rejects the value (`project.go:1181`).
   - **Mentees — accepted and graduated.** Legacy renders these as two distinct sections fed by two separate calls: "Mentees" from `getProjectMentees` and "Graduated Mentees" from `getAllProjectMenteesByStatus(..., 'graduated', ...)`, the latter paginated (`project-public.component.ts:56-59`, `project-public.component.html:38-113`). Keep both sections.
   - **Deliberate divergence — drop the term-state gate.** Legacy hides *both* mentee sections unless the program currently has applications open or an active term (`showAcceptedMentees`, a computed flag from `areApplicationsOpenOrHasActiveTerms()`, `project-public.component.ts:62`). The rewrite shows accepted and graduated mentees on every **published** program regardless of term status. This is the one visibility rule the rewrite intentionally changes: the legacy behavior erases a program's alumni record between terms — exactly when the roster is most useful to a prospective applicant — and the participants it hides are the same ones it showed publicly a day earlier, so nothing is protected by hiding them. Program status remains the gate: unpublished programs expose nothing.
2. **Tasks**: program admins and mentors create tasks; the mentee (assignee) submits material (file upload or marking the task submitted); program admins and mentors accept/complete or decline the submission. The legacy "mentors may create tasks only for accepted mentees" rule is kept and **enforced in the service layer** — the assignee must hold an accepted application on the program. FGA is unaffected; it answers only "is the caller an admin or mentor of this program".
3. **Application withdrawal**: a mentee may withdraw their **own** application only while it is `pending`; a program admin may withdraw a mentee's application in any state. This matches what the legacy product actually offers users (the Withdraw CTA renders only for `pending`, `project-card.component.ts:261`) and fixes the API-level defects behind it — the missing self-check and the two code paths that disagree on withdrawable states. Reproducing the UI's rule as a real server-side rule is a simplification, not added complexity. Consequence to accept: because a mentee may hold only one accepted mentorship, an accepted mentee who wants out must ask an admin — there is no self-service mid-term exit. That is legacy's effective behavior too.
4. **Mentee profile visibility**: editable only by the mentee; viewable by program admins and mentors of a program the mentee has applied to (matches the legacy `isLinked` scope). The mentee's LFX user profile is not included in that grant.
5. **Member emails are never public**: the public program surface is program metadata, accepted mentors (name/avatar/bio/public profile link), and accepted plus graduated mentees (name/avatar/bio) per decision 1. No emails, for any member type. This reproduces the product exactly and closes divergence 8; the anonymous member-list API is a defect, not a feature.
6. **One program-admin role, granted at creation**: the rewrite drops the separate `owner` concept. The creator receives a `program_admin` relation when the program is created, and every action legacy gated on the LFID field-compare (update, hide/unhide, invite mentor, remove mentor) becomes admin-gated. No user loses a capability: legacy already writes the creator a `maintainer` row at creation and never creates another, so owner and admin are the same person in every existing program. Resolves divergence 1, and removes the `ProjectOwnerGuard` mismatch. Unlike legacy, this leaves room to add a second program admin later without a permission cliff.

**Admin-only actions**, consolidated (each admin-only in the legacy product, whether or not legacy's backend enforced it): create/update program terms; decide mentee and mentor applications; withdraw a mentee's application in any state; invite and remove mentors; update and hide/unhide the program.

## Open questions

1. **OQ-1 — Program approver**: "LF admin" needs to become a real, data-backed relation (legacy has only an env-var allowlist and signed one-time email links — divergence 6). Proposed: a platform-level team relation for authorization, with new-program notifications sent to a single configured address (mailing-list alias) rather than resolving individual approver emails. Open: who manages that team's membership.
2. **OQ-2 — Application state machine**: legacy permits any status-to-any-status transition (divergence 5 and the admin dropdown). The rewrite needs an explicit legal-transition table — a service-layer concern, not an FGA one, but it determines what "withdraw" and "graduate" actually mean. Note this is the one place where "replicate observable behavior" gives no answer: legacy's admin dropdown genuinely offers every transition, so the rewrite is choosing a rule rather than reproducing one.
