# Research: LFX Mentorship Core Workflow

**Phase**: 0 | **Date**: 2026-08-20 | **Plan**: [plan.md](plan.md)

Resolves all 7 open clarifications from [spec.md](spec.md#outstanding-clarifications) and
documents key architectural decisions.

---

## Clarification Resolutions

### C-1 — Program submission: zero terms or 5th open term cap

**Decision**: Block at the service layer with `ErrInvalidInput` (HTTP 422).

- `POST /programs/{id}/terms` → 422 `"a program may not have more than 4 open terms"` if
  opening a term would exceed the cap. Only terms with `status = open` count toward the cap;
  `closed` and `deleted` terms do not.
- `PATCH /programs/{id}` with `status = submitted` → 422 `"program must have at least one
  open term before submission"` if no open terms exist.

**Rationale**: Fail fast at the API boundary; no DB constraint required (DB has no aggregate
term count constraint; enforced in service layer).

---

### C-2 — Reviewer blocking non-submitted programs

**Decision**: Service layer enforces valid source states for each transition. Attempting an
invalid transition returns `ErrInvalidStateTransition` → HTTP 409 Conflict.

Valid transitions enforced in `ProgramService.Update`:

| `from` | `to` | Actor |
|--------|------|-------|
| `draft` | `submitted` | program_admin |
| `submitted` | `published` | reviewer |
| `submitted` | `rejected` | reviewer |
| `rejected` | `submitted` | program_admin (resubmit) |
| `published` | `hidden` | program_admin (guard: no blocking apps) |
| `hidden` | `published` | program_admin |
| `published` \| `hidden` | `archived` | program_admin |

Any other `(from, to)` pair is rejected.

**Rationale**: State machines are enforced in the service layer (not DB constraints) to keep
transitions human-readable and testable.

---

### C-3 — `hold` status blocking program hide

**Decision**: YES — `hold` also blocks hiding a program. Blocking states for hide are:
`pending`, `accepted`, `graduated`, **`hold`**.

**Rationale**: An application in `hold` represents an active, unresolved decision. Hiding
the program while the program_admin is mid-review would create a confusing state. The
conservative choice is to include it.

---

### C-4 — Deleted terms and open-term cap / application eligibility

**Decision**:
- `deleted` terms are excluded from the 4-open-term cap count.
- `deleted` terms are excluded from application eligibility (a term with `status = deleted`
  cannot accept applications regardless of its window dates).

**Implementation**: The service queries `COUNT(*) WHERE program_id = $1 AND status = 'open'`
for the cap check. The application service verifies `program_terms.status = 'open'` before
accepting an application.

---

### C-5 — Invitation token expiry

**Decision**: Manual re-invitation. Invitation tokens are time-limited (7-day TTL) and
stored in the `program_members` row's invite metadata (not a separate table — the original
`invitation_tokens` table was removed). If a token expires:

- The `program_members.status` remains `invited`.
- The accept/decline token links return 410 Gone.
- The program_admin must remove the `program_members` record and re-invite.

**Implementation**: Token expiry is encoded in the signed JWT embedded in the email link.
The acceptance endpoint validates the JWT and rejects expired tokens with 410. No
background job required.

**Alternative considered**: Automatic re-send on expiry — rejected because it adds
background scheduler complexity with little benefit; manual re-invite is the v1 behavior.

---

### C-6 — Concurrent applications when program is hidden or archived

**Decision**: Program visibility changes do NOT affect existing application statuses.
Mentees retain their application records and can view them regardless of program `status`.

- `hidden` programs: existing applications are accessible to their owners; no status change.
- `archived` programs: existing applications are frozen at their current status; no new
  applications accepted.

**Rationale**: Changing application status as a side effect of program visibility would be
surprising and potentially lossy. Statuses are only ever changed by explicit actor actions.

---

### C-7 — Bulk decline and `tasks_submitted = true`

**Decision**: Bulk decline applies to **all** `pending` applications on the term regardless
of `tasks_submitted` state.

**Rationale**: The intent of bulk decline is administrative cleanup. Task submission state
is informational; it does not protect an application from admin decisions. If the program_admin
wants to spare tasks-submitted applicants, they can accept them individually first.

---

## Architectural Decisions

### AD-1 — State machine placement

State-transition rules live exclusively in the **service layer**. DB constraints enforce
only the valid enum values (the `CHECK` constraints already present). This keeps transitions
auditable in Go code and avoids opaque DB trigger logic.

### AD-2 — Notifications are in-process hooks

For the initial release, "notifications" (mentor decline notification, admin `tasks_submitted`
notification, mentee acceptance notification/HR paperwork) are logged events. The service
emits a `Notification` interface call; the initial implementation writes to the structured
logger. This provides a swap point for email/event delivery later without changing business
logic.

### AD-3 — Discovery label is computed, not stored

The `discoveryLabel` field on a `ProgramTerm` response is computed server-side from
`status`, `application_start_date`, `application_end_date`, and the current time. It is
never persisted. The computation lives in a helper in the `program_term` model package.

### AD-4 — Invitation tokens are signed JWTs in email links

The tokenised accept/decline link embeds a short-lived JWT (signed with the service's JWT
secret, 7-day TTL) carrying `{ program_id, user_id, action: "mentor-invite" }`. The
`/mentor-invites/{token}/accept` and `/mentor-invites/{token}/decline` endpoints are
**public** (no auth required) because the token itself is the credential.

### AD-5 — `tasks_submitted` is maintained by the task service

When a task is updated to `submitted` or `complete`, the `TaskService.Update` method checks
whether all `prerequisite` tasks on the same `application_id` are now `submitted` or
`complete`. If yes, it sets `applications.tasks_submitted = true` and calls the notification
hook. This is an in-process side effect, not a DB trigger.

### AD-6 — CSV export is streamed

`GET /program-terms/{id}/applications/export` streams CSV rows directly from a `pgx.Rows`
cursor to the HTTP response writer (`text/csv`). No in-memory accumulation required at
expected scale.
