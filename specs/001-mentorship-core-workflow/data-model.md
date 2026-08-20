# Data Model: LFX Mentorship Core Workflow

**Phase**: 1 | **Date**: 2026-08-20 | **Plan**: [plan.md](plan.md)

Authoritative entity reference. Schema source of truth: `backend/db/migrations/001_initial.up.sql`.

---

## Entity: `programs`

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | UUID | PK | |
| `name` | TEXT | NOT NULL | unique enforced in application layer |
| `slug` | TEXT | NOT NULL UNIQUE | URL-safe identifier |
| `status` | VARCHAR(20) | NOT NULL DEFAULT `draft` | see lifecycle below |
| `is_paid` | BOOLEAN | NOT NULL DEFAULT false | stipend flag |
| `description` | TEXT | nullable | required before submission |
| `logo_url` | TEXT | nullable | required before submission |
| `repo_link` | TEXT | nullable | required before submission |
| `website_url` | TEXT | nullable | optional |
| `code_of_conduct` | TEXT | nullable | optional |
| `industry` | TEXT | nullable | comma-separated skill tags (raw) |
| `lfid` | TEXT | nullable | owner LFID |
| `cii_project_id` | TEXT | nullable | optional |
| `accept_applications` | BOOLEAN | DEFAULT false | denormalised flag |
| `terms_and_conditions` | BOOLEAN | DEFAULT false | |
| `program_term_status` | VARCHAR(20) | nullable | denormalised summary |
| `task_templates` | JSONB | nullable | prerequisite task template list |
| `created_on` | TIMESTAMPTZ | DEFAULT NOW() | |
| `updated_on` | TIMESTAMPTZ | DEFAULT NOW() | trigger-maintained |

### Status Lifecycle

```
draft ──submit──► submitted ──approve──► published ◄──unhide──┐
                             └──reject──► rejected             │
                                         rejected ──resubmit──► submitted
published ──hide──► hidden ──────────────────────────────────►─┘
published │ hidden ──archive──► archived
```

**Guard: hide blocked while any application on the program has status ∈ {`pending`, `accepted`, `graduated`, `hold`}**

**Submission guard: program must have ≥ 1 open term and all required fields present.**

---

## Entity: `program_terms`

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | UUID | PK | |
| `program_id` | UUID | FK → programs | CASCADE delete |
| `name` | TEXT | NOT NULL | |
| `status` | VARCHAR(20) | NOT NULL DEFAULT `open` | `open \| closed \| deleted` |
| `active_users` | INTEGER | DEFAULT 0 | denormalised |
| `start_date_time` | TIMESTAMPTZ | nullable | program start |
| `end_date_time` | TIMESTAMPTZ | nullable | program end |
| `application_start_date` | TIMESTAMPTZ | nullable | application window open |
| `application_end_date` | TIMESTAMPTZ | nullable | application window close |
| `created_on` | TIMESTAMPTZ | DEFAULT NOW() | |
| `updated_on` | TIMESTAMPTZ | DEFAULT NOW() | |

### Status Lifecycle

```
open ◄──reopen (if end_date > now)──► closed
open │ closed ──soft-delete──► deleted
```

**Close guard: blocked while any application on this term has `status = accepted`.**

**Open-term cap: a program may have at most 4 terms with `status = open`. `closed` and `deleted` terms are excluded from the count.**

### Computed: Discovery Label

| Condition | Label |
|-----------|-------|
| `status = open` AND `now < application_start_date` | Coming Soon |
| `status = open` AND `now` within window | Apply Now |
| `status = open` AND `now > application_end_date` | In Progress |
| `status = closed` (or `deleted`) | Completed |

---

## Entity: `program_members`

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | UUID | PK | |
| `program_id` | UUID | FK → programs | CASCADE delete |
| `user_id` | UUID | FK → users | |
| `member_type` | VARCHAR(20) | NOT NULL | `maintainer \| mentor` |
| `status` | VARCHAR(20) | nullable | see lifecycle below |
| `email` | TEXT | nullable | invitation address |
| `created_on` | TIMESTAMPTZ | DEFAULT NOW() | |
| `updated_on` | TIMESTAMPTZ | DEFAULT NOW() | |

**Unique**: `(program_id, user_id, member_type)`

### Status Lifecycle

```
── invite ──► invited ──accept──► active
             invited ──decline──► declined
── self-request ──► requested ──approve──► active
                    requested ──decline──► declined
active ──remove──► withdrawn
invited │ requested │ active ──manual-hold──► pending
```

**Maintainer records (`member_type = maintainer`) carry no status (NULL).**

---

## Entity: `user_profiles`

| Column | Type | Notes |
|--------|------|-------|
| `id` | UUID | PK |
| `user_id` | UUID | FK → users, CASCADE |
| `profile_type` | TEXT | `mentor \| apprentice` |
| `slug` | TEXT | UNIQUE |
| `first_name`, `last_name` | TEXT | |
| `email`, `phone` | TEXT | |
| `logo_url` | TEXT | |
| `introduction` | TEXT | |
| `terms_and_conditions` | BOOLEAN | |
| `address` | JSONB | `{country, city, address1, zipCode}` |
| `demographics` | JSONB | optional; `{gender, race, age}` |
| `socioeconomics` | JSONB | optional; `{income, educationLevel}` |
| `skill_set` | JSONB | `{skills[], improvementSkills[], comments}` |
| `profile_links` | JSONB | `{resumeLink, linkedinProfileLink, githubProfileLink}` |

**Eligibility gate (enforced before `apprentice` profile creation)**:
1. Will be ≥ 18 by program start date
2. Eligible to work in country of residence for program duration
3. No existing active `apprentice` profile in another LF mentorship

---

## Entity: `applications`

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | UUID | PK | |
| `program_term_id` | UUID | FK → program_terms | CASCADE delete |
| `user_id` | UUID | FK → users | |
| `role` | VARCHAR(20) | NOT NULL DEFAULT `mentee` | `mentor \| mentee` |
| `status` | VARCHAR(20) | NOT NULL DEFAULT `pending` | see lifecycle |
| `program_term_status` | VARCHAR(20) | nullable | denormalised |
| `start_date_time` | TIMESTAMPTZ | nullable | accepted start |
| `end_date_time` | TIMESTAMPTZ | nullable | accepted end |
| `attendance_type` | VARCHAR(20) | nullable | **required on accept**: `full_time \| part_time` |
| `tasks_submitted` | BOOLEAN | DEFAULT false | set when all prerequisite tasks submitted |
| `admin_notified` | BOOLEAN | DEFAULT false | notification sent flag |

**Unique**: `(program_term_id, user_id, role)`

### Status Lifecycle

```
pending ──accept (+ attendance_type)──► accepted ──begin──► active ──graduate──► graduated
pending ──decline──► declined
pending ──withdraw (mentee)──► withdrawn
pending ──hold──► hold ──resume──► pending
```

**Application window guard**: apply only when `program_terms.status = open` AND
`now` is within `[application_start_date, application_end_date]`.

**Re-apply**: allowed from `withdrawn` (window still open); blocked from `declined`.

**Computed: `tasks_submitted`**: set `true` when all tasks on this application with
`category = prerequisite` are in state `submitted` or `complete`.

---

## Entity: `tasks`

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | UUID | PK | |
| `application_id` | UUID | FK → applications, SET NULL | |
| `program_term_id` | UUID | FK → program_terms | denormalised |
| `assignee_id` | UUID | FK → users | NOT NULL |
| `owner_id` | UUID | FK → users | nullable |
| `name` | TEXT | nullable | |
| `description` | TEXT | nullable | |
| `category` | VARCHAR(50) | nullable | `prerequisite \| non_prerequisite` |
| `status` | VARCHAR(30) | NOT NULL DEFAULT `incomplete` | see lifecycle |
| `application_status` | VARCHAR(20) | nullable | denormalised |
| `program_term_status` | VARCHAR(20) | nullable | denormalised |
| `custom` | BOOLEAN | DEFAULT false | |
| `submit_file` | TEXT | nullable | `null \| 'required' \| URL` |
| `file` | TEXT | nullable | uploaded file URL |
| `due_date` | DATE | nullable | |
| `created_by` | TEXT | nullable | creator LFID |

### Status Lifecycle & Actor Permissions

```
incomplete ──mentee──► in_progress ──mentee──► submitted ──admin/mentor──► complete
any state ──admin/mentor──► incomplete  (reset)
```

**Category semantics**:
- `prerequisite` — cloned from program templates at application time; gate application review
- `non_prerequisite` — assigned by maintainer/mentor after application is `accepted`

---

## Error Catalog

| Symbol | HTTP | Trigger |
|--------|------|---------|
| `ErrInvalidStateTransition` | 409 | Illegal `(from, to)` status pair |
| `ErrStateLocked` | 409 | Guard condition blocks transition (e.g. close-with-accepted, hide-with-active-apps) |
| `ErrInvalidInput` | 422 | Validation failure (missing required field, bad enum) |
| `ErrProgramNotFound` | 404 | Program ID does not exist |
| `ErrTermNotFound` | 404 | Term ID does not exist |
| `ErrApplicationNotFound` | 404 | Application ID does not exist |
| `ErrMemberNotFound` | 404 | Member ID does not exist |
| `ErrTaskNotFound` | 404 | Task ID does not exist |
| `ErrInviteExpired` | 410 | Mentor invitation token expired or invalid |
| `ErrForbidden` | 403 | Caller is not permitted for this action |
