# Feature Specification: LFX Mentorship Core Workflow

**Feature Branch**: `001-mentorship-core-workflow`
**Created**: 2026-08-20
**Status**: Draft
**Input**: User description: "LFX Mentorship program lifecycle covering program creation and approval, mentor invitation and self-request, mentee profile and application, prerequisite task evaluation, and application disposition."

## Execution Flow (main)
```
1. Program Admin creates a program in draft and submits it for review
2. Reviewer publishes or rejects the program
3. Program Admin invites mentors, or mentors self-request; each is approved or declined
4. Mentee creates a profile (subject to an eligibility gate) and applies to an open term
5. Prerequisite tasks are cloned onto the application; mentee completes them
6. Program Admin reviews submitted tasks and accepts, declines, or holds the application
7. Accepted mentees begin the program, are later graduated, or withdraw
8. Program Admin manages terms and program visibility throughout
```

---

## User Scenarios & Testing *(mandatory)*

### Primary User Story
A program_admin stands up a mentorship program, defines one or more terms, and submits
it for review. Once published, mentors join by invitation or self-request, and
prospective mentees create a profile, apply to an open term, and complete prerequisite
tasks. The program_admin reviews completed tasks and decides whether to accept, decline,
or hold each applicant. Accepted mentees move through the program to graduation, and
the program_admin can manage the program's visibility and terms throughout its lifecycle.

### Acceptance Scenarios

1. **Given** a program_admin with a linked LF project, **When** they create a program with
   a unique name, description, repository URL, logo, at least one skill tag, and at
   least one term, **Then** the program is created with `status = draft`.

2. **Given** a `draft` program that meets all required fields, **When** the program_admin
   submits it, **Then** `status` transitions to `submitted` and it becomes visible to
   reviewers.

3. **Given** a `submitted` program, **When** a reviewer approves it, **Then**
   `status` transitions to `published` and the Mentees, Mentors, and Terms admin tabs
   become active. **When** a reviewer instead declines it, **Then** `status`
   transitions to `rejected` and the program_admin may revise and resubmit.

4. **Given** a `published` program with no `pending`, `accepted`, or `graduated`
   applications, **When** the program_admin hides it, **Then** `status` transitions to
   `hidden` and the program returns a 404 to all users except the owner.

5. **Given** a `hidden` program, **When** the program_admin unhides it, **Then**
   `status` returns to `published`.

6. **Given** a `published` program, **When** the program_admin marks it complete,
   **Then** `status` transitions to `archived`.

7. **Given** an `open` term with an `accepted` application still active, **When**
   the program_admin attempts to close the term, **Then** the close is blocked until
   that mentee is `graduated` or `declined`.

8. **Given** a `closed` term whose end date is still in the future, **When** the
   program_admin reopens it, **Then** `status` returns to `open`.

9. **Given** an `open` term whose current date falls before
   `application_start_date`, **When** a prospective mentee views the program,
   **Then** the discovery label reads "Coming Soon". **When** the current date falls
   within the application window, **Then** the label reads "Apply Now". **When** the
   window has closed but the term remains `open`, **Then** the label reads
   "In Progress". **When** the term is `closed`, **Then** the label reads
   "Completed".

10. **Given** a `published` program, **When** the program_admin searches for and invites
    an LF user as a mentor, **Then** a `program_members` record is created with
    `member_type = mentor`, `status = invited`, and an invitation email with a
    tokenised accept/decline link is sent.

11. **Given** an invited mentor, **When** they accept, **Then** `status` transitions
    to `active`. **When** they decline, **Then** `status` transitions to `declined`
    and the program admin is notified.

12. **Given** a `published` program, **When** a mentor self-requests participation
    without an invitation, **Then** a `program_members` record is created with
    `member_type = mentor`, `status = requested`. **When** the program_admin approves,
    **Then** `status` transitions to `active`. **When** the program_admin declines,
    **Then** `status` transitions to `declined`.

13. **Given** an `active` mentor, **When** the program_admin removes them, **Then**
    `status` transitions to `withdrawn`.

14. **Given** a prospective mentee who confirms all three eligibility criteria
    (minimum age at program start, work eligibility for the program duration, and no
    existing active mentee profile in another LF mentorship), **When** they proceed,
    **Then** they may create a `user_profile` with `profile_type = mentee`.
    **When** any criterion fails, **Then** profile creation is blocked.

15. **Given** an existing mentee profile, **When** that mentee applies to a program,
    **Then** profile creation is skipped and they apply directly.

16. **Given** a term that is `open` and within its application window, **When** a
    mentee submits an application, **Then** an `application` record is created with
    `status = pending` and the program's prerequisite task templates are cloned onto
    the mentee, linked via `application_id`, each with `category = prerequisite`.

17. **Given** a `withdrawn` application whose term application window is still open,
    **When** the mentee reapplies, **Then** a new application is accepted. **Given**
    a `declined` application, **When** the mentee attempts to reapply, **Then** the
    reapplication is rejected.

18. **Given** a mentee with prerequisite tasks in `incomplete`, **When** the mentee
    starts work, **Then** the task transitions to `in_progress`, and **When** they
    submit it, **Then** it transitions to `submitted`. **Given** a `submitted` task,
    **When** the program_admin or mentor reviews it, **Then** it transitions to
    `complete`, or is reset to `incomplete` for rework.

19. **Given** an application whose prerequisite tasks all reach `submitted` or
    `complete`, **When** the last one transitions, **Then**
    `applications.tasks_submitted` is set to `true`, the program admin is notified,
    and the application `status` remains `pending` (task completion never
    auto-changes application status).

20. **Given** a `pending` application reviewed by the program_admin, **When** the
    mentee qualifies, **Then** `status` transitions to `accepted` and the program_admin
    supplies `attendance_type` (`full_time` or `part_time`), triggering mentee
    notification and HR paperwork. **When** the mentee does not qualify, **Then**
    `status` transitions to `declined`.

21. **Given** a `pending` application, **When** the mentee voluntarily exits,
    **Then** `status` transitions to `withdrawn`.

22. **Given** an application under review, **When** the program_admin needs more
    information before deciding, **Then** `status` transitions to `hold`.

23. **Given** an `accepted` application whose program period has begun, **When**
    the program_admin marks it, **Then** `status` transitions to `active`, and
    **When** the program_admin manually marks the program period complete for that
    mentee, **Then** `status` transitions to `graduated` (this is never automatic at
    term end).

24. **Given** an `active` mentee, **When** the program_admin or mentor assigns
    additional work, **Then** a task is created with `category = non_prerequisite`.

25. **Given** a term with multiple `pending` applications, **When** the program_admin
    performs a bulk decline, **Then** all `pending` applications for that term
    transition to `declined`.

26. **Given** a set of applications, **When** the program_admin exports by status
    (including `tasks_submitted`), **Then** a CSV is produced matching the filter.

27. **Given** a `closed` term, **When** the program_admin views Past Mentees, **Then**
    a read-only list of that term's mentees is shown.

### Edge Cases

- What happens when a program_admin attempts to submit a program with zero terms, or a
  fifth open term (exceeding the 4-open-term maximum)? → Submission/term-creation must
  be blocked. [NEEDS CLARIFICATION: exact error/validation message not specified in
  source]
- What happens when a reviewer attempts to act on a program that is not in
  `submitted` state? → Out of scope for this spec; assumed blocked by state guard.
  [NEEDS CLARIFICATION: no explicit rule given]
- What happens if a program_admin tries to hide a program while an application is
  `hold`? → Not addressed by source document; only `pending`, `accepted`, and
  `graduated` are named as blockers. [NEEDS CLARIFICATION: confirm `hold` should or
  should not also block hide]
- What happens when a mentee applies to a term where the window is open but the term
  itself has been soft-deleted? → Assumed blocked, since a deleted term cannot be
  `open`. [NEEDS CLARIFICATION: confirm deleted terms are excluded from open-term
  count and application eligibility]
- What happens when an invited mentor's token has expired before they respond? →
  Not specified. [NEEDS CLARIFICATION: token expiry / re-invitation flow undefined]
- What happens when a mentee holds a concurrent application on a different program
  and that program is later hidden or archived? → Not specified.
  [NEEDS CLARIFICATION]
- What happens on bulk decline if some `pending` applications in the term have
  `tasks_submitted = true`? → Source implies bulk decline applies to all `pending`
  regardless of task state, but this is not explicit. [NEEDS CLARIFICATION]

---

## Requirements *(mandatory)*

### Functional Requirements — Program Lifecycle

- **FR-001**: System MUST allow a program_admin to create a program with `status =
  draft`, requiring a linked LF project, a unique name, a description, a repository
  URL, a logo, at least one skill tag, and at least one term.
- **FR-002**: System MUST support the following optional program fields: CII project
  ID, website URL, code of conduct, and prerequisite task templates.
- **FR-003**: System MUST enforce a maximum of 4 open terms per program.
- **FR-004**: System MUST allow a program_admin to transition a program from `draft` to
  `submitted` only when all required fields (FR-001) are present.
- **FR-005**: System MUST allow a reviewer to transition a `submitted` program to
  either `published` or `rejected`.
- **FR-006**: System MUST activate the Mentees, Mentors, and Terms admin tabs only
  once a program reaches `published`.
- **FR-007**: System MUST allow a program_admin to revise and resubmit a `rejected`
  program.
- **FR-008**: System MUST allow a program_admin to transition a `published` program to
  `hidden`, and MUST block this transition while any application on the program has
  `status` of `pending`, `accepted`, or `graduated`.
- **FR-009**: System MUST return a 404 response for a `hidden` program to all users
  except its owner.
- **FR-010**: System MUST allow a program_admin to transition a `hidden` program back to
  `published`.
- **FR-011**: System MUST allow a program_admin to transition a `published` program to
  `archived` upon completion.

### Functional Requirements — Term Lifecycle

- **FR-012**: System MUST support term statuses of `open`, `closed`, and (soft
  delete) `deleted`.
- **FR-013**: System MUST block closing a term while any application on that term has
  `status = accepted`, until each such mentee is `graduated` or `declined`.
- **FR-014**: System MUST allow reopening a `closed` term only when its end date is
  still in the future.
- **FR-015**: System MUST store `application_start_date` and `application_end_date`
  on each term.
- **FR-016**: System MUST permit a mentee to apply only when the term `status = open`
  AND the current date falls within the application window.
- **FR-017**: System MUST derive a public discovery label from term status and
  application window per the following mapping: open + window in future → "Coming
  Soon"; open + within window → "Apply Now"; open + window closed → "In Progress";
  closed → "Completed".

### Functional Requirements — Mentor Invitation & Self-Request

- **FR-018**: System MUST allow a program_admin, after a program is `published`, to
  search LF users and create a `program_members` record with `member_type = mentor`
  and `status = invited`.
- **FR-019**: System MUST dispatch an invitation email containing a tokenised
  accept/decline link when a mentor is invited.
- **FR-020**: System MUST transition an invited mentor's `status` to `active` on
  acceptance, or to `declined` on decline, and MUST notify the program admin on
  decline.
- **FR-021**: System MUST allow a program_admin to manually set a mentor's `status` to
  `pending`.
- **FR-022**: System MUST allow a program_admin to remove a mentor, setting `status` to
  `withdrawn`.
- **FR-023**: System MUST allow a mentor to self-request participation in a
  `published` program, creating a `program_members` record with `member_type =
  mentor` and `status = requested`.
- **FR-024**: System MUST allow a program_admin to approve a self-requested mentor
  (`status = active`), decline them (`status = declined`), or remove them (`status =
  withdrawn`).

### Functional Requirements — Mentee Eligibility, Profile & Application

- **FR-025**: System MUST require confirmation of three eligibility criteria before
  mentee profile creation: (a) will be at least 18 by program start date, (b) is
  eligible to work in their country of residence for the program duration, and (c)
  does not hold an active mentee profile in another LF mentorship program.
- **FR-026**: System MUST block participation if any eligibility criterion (FR-025)
  is not met.
- **FR-027**: System MUST allow a new mentee to create a `user_profile` with
  `profile_type = mentee`, capturing identity, phone, address, a validated GitHub
  URL, LinkedIn, resume, current skills, skills to improve, and optional demographic
  and socioeconomic data.
- **FR-028**: System MUST allow an existing mentee to skip profile creation and apply
  directly.
- **FR-029**: System MUST allow a mentee to submit an `application` against a
  specific `program_term` only while that term is `open` and the current date is
  within the application window; new applications MUST start with `status = pending`.
- **FR-030**: System MUST allow reapplication from a `withdrawn` application only
  while the application window remains open, and MUST NOT allow reapplication from a
  `declined` application.
- **FR-031**: System MUST allow a mentee to hold concurrent applications across
  different programs, and MUST surface those cross-program applications on the
  program_admin's Mentee row view.

### Functional Requirements — Prerequisite Task Evaluation

- **FR-032**: System MUST clone the program's prerequisite task templates onto the
  mentee upon application submission, linking each via `application_id` with
  `category = prerequisite`.
- **FR-033**: System MUST restrict task status transitions by actor: a mentee may
  move a task `incomplete → in_progress` and `in_progress → submitted`; a program_admin
  or mentor may move a task `submitted → complete` or reset any state to
  `incomplete`.
- **FR-034**: System MUST set `applications.tasks_submitted = true` once all
  `prerequisite` tasks on an application reach `submitted` or `complete`, and MUST
  notify the program admin at that point.
- **FR-035**: System MUST keep an application's `status` at `pending` regardless of
  task completion; task state changes MUST NEVER automatically change application
  status.

### Functional Requirements — Application Disposition

- **FR-036**: System MUST allow a program_admin to transition a `pending` application
  to `accepted` when the mentee qualifies, and MUST require `attendance_type`
  (`full_time` or `part_time`) to be supplied at the same time.
- **FR-037**: System MUST trigger mentee notification and HR paperwork generation
  when an application is set to `accepted`.
- **FR-038**: System MUST allow a program_admin to transition a `pending` application to
  `declined` when the mentee does not qualify.
- **FR-039**: System MUST allow a mentee to voluntarily transition their own
  `pending` application to `withdrawn`.
- **FR-040**: System MUST allow a program_admin to transition an application to `hold`
  pending additional information.
- **FR-041**: System MUST allow a program_admin to transition an `accepted` application
  to `active` when the program period begins.
- **FR-042**: System MUST allow a program_admin to manually transition an `active`
  application to `graduated`; this transition MUST NOT occur automatically at term
  end.
- **FR-043**: System MUST allow a program_admin or mentor to assign additional tasks
  with `category = non_prerequisite` to an `active` mentee.
- **FR-044**: System MUST allow a program_admin to bulk-decline all `pending`
  applications for a given term in a single action.
- **FR-045**: System MUST support CSV export of applications filtered by status,
  including the `tasks_submitted` flag.
- **FR-046**: System MUST provide a read-only Past Mentees view scoped to closed
  terms.

### Key Entities

- **Program**: Represents a mentorship program owned by a program_admin and linked to an
  LF project. Key attributes: name (unique), description, repository URL, logo,
  skill tags (≥1), CII project ID (optional), website URL (optional), code of conduct
  (optional), prerequisite task templates (optional). Status lifecycle: `draft` →
  `submitted` → `published` ↔ `hidden` | `rejected` → `archived`.
- **ProgramTerm**: A time-boxed run of a program (max 4 open per program). Key
  attributes: `application_start_date`, `application_end_date`, end date. Status
  lifecycle: `open` ↔ `closed` | `deleted`. Belongs to one Program.
- **ProgramMember**: Represents a mentor's (or program_admin's) relationship to a
  program. Key attributes: `member_type` (`mentor`, etc.), `status` (`invited` |
  `requested` → `active` | `declined` | `withdrawn`; `pending` as a manual hold).
  Belongs to one Program.
- **UserProfile**: A mentee's (`profile_type = mentee`) or other user's profile.
  Key attributes: identity, phone, address, GitHub URL (validated), LinkedIn, resume,
  current skills, skills to improve, optional demographics/socioeconomics.
- **Application**: A mentee's request to join a specific ProgramTerm. Key attributes:
  `tasks_submitted` (boolean), `attendance_type` (`full_time` | `part_time`, set on
  acceptance). Status lifecycle: `pending` → `accepted` → `active` → `graduated` |
  `declined` | `withdrawn` | `hold`. Belongs to one ProgramTerm; linked to one
  UserProfile/mentee.
- **Task**: A unit of work cloned onto or assigned to a mentee. Key attributes:
  `category` (`prerequisite` | `non_prerequisite`). Status lifecycle: `incomplete` →
  `in_progress` → `submitted` → `complete`. Belongs to one Application (when
  prerequisite) or one active mentee relationship (when non-prerequisite).

---

## Success Criteria *(mandatory)*

- **SC-001**: 100% of program status transitions observed in the system match the
  lifecycle `draft → submitted → published ↔ hidden | rejected → archived`; no
  program is ever observed in an undefined status.
- **SC-002**: 0 terms can be closed while an `accepted` application remains open on
  that term.
- **SC-003**: 0 programs can be hidden while a `pending`, `accepted`, or `graduated`
  application remains on that program.
- **SC-004**: 100% of accepted applications carry a non-null `attendance_type`.
- **SC-005**: 0 applications ever transition to `accepted` or `declined` as a direct,
  automatic side effect of a task status change; task completion only sets
  `tasks_submitted = true` and notifies the admin.
- **SC-006**: 0 reapplications succeed from a `declined` application; reapplication
  from `withdrawn` succeeds 100% of the time the application window is still open.
- **SC-007**: 100% of programs with 4 open terms reject a request to open a 5th term.
- **SC-008**: Discovery label shown to prospective mentees matches the term
  status/window mapping (FR-017) in 100% of sampled cases.

---

## Review & Acceptance Checklist

### Content Quality
- [x] No implementation details (languages, frameworks, APIs) beyond field/status
      names already present in the source workflow
- [x] Focused on user-observable behavior and business rules
- [x] Written for a non-implementation audience (product/QA readable)
- [x] All mandatory sections completed

### Requirement Completeness
- [ ] 7 clarification markers remain open — see **Edge Cases** above
      (`[NEEDS CLARIFICATION]`); resolve before moving to `/plan`
- [x] Requirements are testable and unambiguous (each maps to an FR-### id)
- [x] Success criteria are measurable and technology-agnostic
- [x] All acceptance scenarios are written in Given/When/Then form
- [x] Scope is bounded to the core workflow described in the source document
- [x] Dependencies and assumptions identified (LF project linkage, LF user
      directory for mentor search, email delivery for invitations/notifications)

---

## Outstanding Clarifications

| # | Location | Question |
|---|---|---|
| 1 | Edge Cases | What error/message is shown when program submission fails due to zero terms or exceeding the 4-open-term cap? |
| 2 | Edge Cases | Should reviewer actions be blocked on programs not in `submitted` status, and if so, how should that be surfaced? |
| 3 | Edge Cases | Does an application in `hold` also block hiding a program, alongside `pending`, `accepted`, and `graduated`? |
| 4 | Edge Cases | Are `deleted` terms excluded from both the 4-open-term cap and mentee application eligibility? |
| 5 | Mentor Invitation | What happens when an invitation token expires before the mentor responds — is re-invitation automatic or manual? |
| 6 | Mentee Application | What happens to a mentee's other concurrent applications if one of their programs is later hidden or archived? |
| 7 | Bulk Operations | Does bulk decline apply uniformly to all `pending` applications in a term regardless of `tasks_submitted` state? |
