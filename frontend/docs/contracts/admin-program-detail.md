<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Admin — Program detail

**Page:** `/mentorship/admin/:programId` (`ProgramDetailComponent`)
**Tabs:** Current Mentees, Past Mentees, Applicants, Mentors, Terms

The header (title, season line, status, **Edit Program**, tab labels + counts)
loads first. Each underline tab has its own list GET. Do not return all four
lists from the header GET.

The current mock BFF returns a fat `MentorshipProgramDetail` from
`GET /api/mentorship/programs/:programId`. Split that into the endpoints below
when wiring the real service.

Row actions that only change lifecycle status share **one PATCH per resource**.
Invite, remove, notes, create/list tasks, and term create/edit/delete are
separate. File downloads (CSV export, task view/download) are handled by the
UI — no export or submission endpoints.

---

## Page bootstrap

### 1. Get program header

```http
GET /api/mentorship/programs/:programId
```

**Backend:** `GET /v1/programs/{uid}` (program `viewer`).

`:programId` is `id`.

Tab visibility is driven by **terms**, not by program status:

| Condition                                | Tab                                                   |
| ---------------------------------------- | ----------------------------------------------------- |
| At least one **open** (in-progress) term | show **Current Mentees**                              |
| At least one **closed** term             | show **Past Mentees**                                 |
| Program `status` is `pending`            | people lists are empty (no mentees / applicants data) |

A program can show **both** Current and Past when it has open terms and closed
terms at the same time.

#### Success `200`

```json
{
  "program": {
    "id": "mp_gridflow_fall26",
    "slug": "gridflow-time-series-ingestion-pipeline",
    "name": "GridFlow: Time-Series Ingestion Pipeline",
    "projectName": "LF Energy",
    "activeTerm": { "id": "trm_gridflow_fall26", "name": "Fall 2026", "status": "open" },
    "status": "published",
    "stats": { "mentors": 4, "mentees": 2, "graduated": 6 },
    "logoUrl": "https://...",
    "createdOn": "2026-06-01T00:00:00.000Z",
    "updatedOn": "2026-08-15T00:00:00.000Z"
  },
  "hasOpenTerm": true,
  "hasClosedTerm": true,
  "tabCounts": {
    "mentees": 2,
    "pastMentees": 5,
    "applicants": 9,
    "mentors": 4,
    "terms": 4
  }
}
```

`hasOpenTerm` / `hasClosedTerm` tell the UI which mentee tabs to render.
`tabCounts.mentees` is `type=current`, `pastMentees` is `type=past`,
`applicants` is `type=all`. Do **not** include people or term arrays here.

`activeTerm` is a term object (not a string). When there is no open
term it will be the latest closed term.

For `pending`, `hasOpenTerm` / `hasClosedTerm` may still reflect terms,
but `tabCounts` people counts are `0` and the mentees list returns empty.

> **BFF composition:** The BFF calls `GET /v1/programs/{uid}` for the program
> and `GET /v1/programs/{uid}/management-summary` (program `manager`) for the
> tab counts. It merges the two responses into the shape above.

#### Errors

| Status | When                                           |
| ------ | ---------------------------------------------- |
| `404`  | Unknown id/slug. UI shows "Program not found". |

### 2. Edit program

Header button **Edit Program**. Same fields as enroll (details + setup +
prerequisites), minus `termsAccepted`. Terms on this page are edited on the
Terms tab, not here.

```http
PATCH /api/mentorship/programs/:programId
```

**Backend:** `PATCH /v1/programs/{uid}` (program `writer`). Must reject
`status` — status transitions use dedicated routes.

Body: `MentorshipEnrollRequest` without `termsAccepted` (and typically without
`terms` — those stay on the Terms tab). Logo may reuse [enroll logo upload](./admin-enroll-program.md#5-upload-program-logo).

#### Success `200`

Updated `MentorshipProgram` (header shape).

Name uniqueness still applies (ignore the program's own current name). Status
stays as-is unless a later review workflow changes it.

---

## Shared term object

Anywhere a person row used to return `termName`, return a **term object**:

```json
{
  "id": "trm_gridflow_fall26",
  "name": "Fall 2026",
  "status": "open",
  "startDate": "2026-09-01",
  "endDate": "2026-11-01",
  "applicationStartDate": "2026-05-01",
  "applicationEndDate": "2026-07-15"
}
```

Filter query `term` is the term **id**, not the name.

---

## Shared task object

```json
{
  "id": "tsk_alex_resume",
  "name": "Resume",
  "description": "Upload the most recent version of your resume.",
  "status": "submitted",
  "prerequisite": false,
  "createdOn": "2026-07-01",
  "updatedOn": "2026-08-15",
  "dueOn": "2026-10-01",
  "hasSubmission": true,
  "custom": false,
  "file": "http://..."
}
```

| Field           | Notes                                                                                                              |
| --------------- | ------------------------------------------------------------------------------------------------------------------ |
| `status`        | `pending` \| `in_progress` \| `submitted` \| `completed`. Read-only for admins.                                   |
| `hasSubmission` | Gates the eye / download icons in the UI.                                                                          |
| `file`          | URL of the uploaded file when `hasSubmission` is true. The UI uses this to view and download — no submission APIs.  |
| `custom`        | Admin-authored extra task.                                                                                         |
| `dueOn`         | ISO `YYYY-MM-DD`. Omit for prerequisite tasks with no calendar due date (UI shows "Prerequisite Task").            |

Do **not** nest `tasks[]` on the mentee list row. Keep `tasksSubmitted` and
`tasksTotal` on the mentee; load the array from the standalone tasks GET when
the row expands.

---

## Tab 1 — Current Mentees / Past Mentees / Applicants (one list)

### 3. List applications

```http
GET /api/mentorship/programs/:programId/applications
```

**Backend:** `GET /v1/programs/{uid}/applications` (program `manager`).
Program-wide applications across all terms.

| Query              | Type                          | Notes                                                                                                                  |
| ------------------ | ----------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `type`             | `current` \| `past` \| `all` | Required. See table below.                                                                                             |
| `search`           | string                        | Name or email, case-insensitive.                                                                                       |
| `status`           | `ApplicationStatus`           | `pending` \| `accepted` \| `declined` \| `withdrawn` \| `graduated` \| `hold`. Omit = all statuses that `type` allows. |
| `term`             | string                        | Term **id**.                                                                                                           |
| `offset` / `limit` | number                        | UI page size default `10`, options `10, 25, 50`.                                                                       |

| `type`    | Who is in `data`                                                                                                     | UI tab          |
| --------- | -------------------------------------------------------------------------------------------------------------------- | --------------- |
| `current` | People on **open** (in-progress) terms. Typical statuses: `accepted`, `graduated`.                                   | Current Mentees |
| `past`    | **All** people on **closed** terms, **whatever status**.                                                             | Past Mentees    |
| `all`     | Every person on the program (open + closed terms, any status), plus applicant fields. Replaces `GET .../applicants`. | Applicants      |

`pending` programs always return `{ "data": [], "total": 0 }`.

#### Success `200`

```json
{
  "type": "current",
  "data": [
    {
      "userId": "user_122",
      "applicationId":"app_445454",
      "name": "Alex Rivera",
      "email": "alex.rivera@example.com",
      "avatarUrl": "https://…",
      "status": "accepted",
      "term": {
        "id": "trm_gridflow_fall26",
        "name": "Fall 2026",
        "status": "open",
        "startDate": "2026-09-01",
        "endDate": "2026-11-01",
        "applicationStartDate": "2026-05-01",
        "applicationEndDate": "2026-07-15"
      },
      "tasksSubmitted": 7,
      "tasksTotal": 12,
      "note": "Strong Go background; paired well during the screening exercise.",
      "createdOn": "2026-06-28",
      "updatedOn": "2026-07-02",
      "otherApplications": [
        {
          "programId": "mp_apicurio_winter26",
          "programName": "Apicurio Registry",
          "status": "pending"
        }
      ]
    }
  ],
  "total": 2
}
```

No `tasks` array on the row. `tasksSubmitted` / `tasksTotal` stay here so the
Tasks column can render `7 of 12 submitted` without the extra fetch.

`createdOn` / `updatedOn` and `otherApplications` are the former Applicants
fields. Always include them on `type=all`. Optional on `current` / `past`.

`otherApplications` are this person's rows on **other** programs that are still
active: wire status `pending`, `accepted`, or `graduated`. Omit `declined` and
`withdrawn`. `programId` is the navigation target.

Display-status mapping (Applicants tab / `type=all`):

| Wire `status`                                                                            | Display           |
| ---------------------------------------------------------------------------------------- | ----------------- |
| `pending` and not all tasks submitted                                                    | `applied`         |
| `pending` and every task submitted (`tasksTotal > 0` and `tasksSubmitted >= tasksTotal`) | `tasks-completed` |
| `hold`                                                                                   | `hold`            |
| `accepted` / `declined` / `withdrawn` / `graduated`                                      | same              |

A person with no tasks assigned is never `tasks-completed`.

**Current columns:** person, status, task progress, Create Task, View Tasks,
row actions, reviewer note.

**Past columns:** person, `term`, status. No tasks, notes, or row actions.

**Applicants (`type=all`) columns:** person, `term`, status, application dates,
other applications, View Tasks, row actions, reviewer note.

### 4. List application tasks

Fired when **View Tasks** expands a row. Not nested in the applications list.

```http
GET /api/mentorship/applications/:applicationId/tasks
```

**Backend:** `GET /v1/applications/{uid}/tasks` (application `auditor`).

#### Success `200`

```json
{
  "data": [
    {
      "id": "tsk_alex_resume",
      "name": "Resume",
      "description": "Upload the most recent version of your resume.",
      "status": "submitted",
      "prerequisite": false,
      "createdOn": "2026-07-01",
      "updatedOn": "2026-08-15",
      "dueOn": "2026-10-01",
      "hasSubmission": true,
      "custom": false,
      "file": "http://..."
    }
  ],
  "total": 1
}
```

The UI views / downloads a submission from `file`. No task-file APIs.

### 5. Update application status

One endpoint for Current Mentees **and** Applicants row menus.

```http
PATCH /api/mentorship/applications/:applicationId/status
```

**Backend:** `PATCH /v1/applications/{uid}/status` (application `manager`).

```json
{ "status": "withdrawn" }
```

`status` is `accepted` \| `declined` \| `withdrawn` \| `graduated` \| `hold`.

| Current status | Allowed next                                |
| -------------- | ------------------------------------------- |
| `pending`      | `accepted`, `declined`, `hold`, `withdrawn` |
| `hold`         | `accepted`, `declined`, `pending`           |
| `accepted`     | `graduated`, `declined`                     |
| `declined`     | `pending`                                   |
| `withdrawn`    | *(terminal)*                                |
| `graduated`    | *(terminal)*                                |

Only `accepted` may move to `graduated`. Accepting a `pending` row enrolls them
as a current mentee on that term. `400` if the transition is illegal.

There is **no** `PATCH .../applicants/:applicantId`.

#### Success `200`

The updated application row (same shape as list, still no nested `tasks`).

### 6. Create task

**Create Task** may assign the same task to **multiple** applications.

```http
POST /api/mentorship/applications/tasks
```

```json
{
  "applicationIds": ["app_alex_rivera_fall26", "app_priya_shah_fall26"],
  "name": "Midterm Report",
  "description": "Summarize progress on your mentorship project goals.",
  "prerequisite": false,
  "custom": false,
  "dueOn": "2026-10-01",
  "requireFile": true
}
```

`applicationIds` is required and must contain at least one application id.

#### Success `201`

```json
{
  "data": [
    {
      "id": "tsk_alex_midterm",
      "name": "Midterm Report",
      "description": "Summarize progress on your mentorship project goals.",
      "status": "pending",
      "prerequisite": false,
      "createdOn": "2026-09-16",
      "updatedOn": "2026-09-16",
      "dueOn": "2026-10-01",
      "hasSubmission": false,
      "custom": false,
      "file": ""
    }
  ]
}
```

Increment each assignee's `tasksTotal`.

### 7. Decline by term

Toolbar **Decline by Term**. Bulk status update, so it is its own endpoint
rather than N row PATCHes.

```http
POST /api/mentorship/programs/:programId/terms/:termId/bulk-decline
```

**Backend:** `POST /v1/programs/{uid}/terms/{termId}/applications/bulk-decline`
(program `writer`).

Declines every `pending` row on that term. Does not touch accepted /
graduated / already declined / withdrawn rows.

#### Success `200`

```json
{ "declined": 3 }
```

---

## Tab 3 — Mentors

### 8. List mentors

```http
GET /api/mentorship/programs/:programId/mentors
```

**Backend:** `GET /v1/programs/{uid}/member-management` (program `writer`).
Administrative roster including pending/history rows, invitation metadata,
profile state, and email.

| Query              | Type   | Notes                                                             |
| ------------------ | ------ | ----------------------------------------------------------------- |
| `search`           | string | Name, email, or username.                                         |
| `offset` / `limit` | number | The current UI does not paginate this table; still accept paging. |

#### Success `200`

```json
{
  "data": [
    {
      "id": "mtr_dana_kovacs",
      "name": "Dana Kovacs",
      "email": "dana.kovacs@example.com",
      "username": "dana.kovacs",
      "avatarUrl": "https://…",
      "status": "approved",
      "createdOn": "2026-06-12T00:00:00.000Z",
      "updatedOn": "2026-06-20T00:00:00.000Z",
      "profileCreated": true
    }
  ],
  "total": 4
}
```

`createdOn` / `updatedOn` replace `invitedOn`. The Invitation Date column uses
`createdOn`.

`status`: `pending` \| `approved` \| `declined` \| `withdrawn`.
Mentors do not graduate.

Row action visibility:

| Status      | Approve | Decline | Remove |
| ----------- | ------- | ------- | ------ |
| `pending`   | yes     | yes     | yes    |
| `approved`  | no      | yes     | yes    |
| `declined`  | yes     | no      | yes    |
| `withdrawn` | no      | no      | yes    |

### 9. Search people to invite

Not program-scoped. The UI subtracts usernames already on this program's mentor
list.

```http
GET /api/mentorship/invitable-users?search=&offset=0&limit=50
```

`limit` default `50` (`MENTORSHIP_INVITABLE_USER_PAGE_SIZE`).

#### Success `200` — `MentorshipInvitableUsersResponse`

```json
{
  "data": [
    {
      "id": "usr_ada_lovelace",
      "name": "Ada Lovelace",
      "email": "ada.lovelace@example.com",
      "username": "ada.lovelace",
      "avatarUrl": "https://…"
    }
  ],
  "total": 12
}
```

Search matches `name`, `email`, or `username`. `username` is required — it is
what Invite sends.

### 10. Invite mentor

Toolbar **Invite**. Not a status change.

```http
POST /api/mentorship/programs/:programId/mentors
```

**Backend:** `POST /v1/programs/{uid}/members` (program `writer`). The BFF
resolves the username to a `user_id` before forwarding, and sets
`member_type=mentor`.

```json
{ "username": "ada.lovelace" }
```

Creates a `pending` mentor with `createdOn` = now. `409` if that username is
already on the program.

#### Success `201`

The new `MentorshipProgramMentor`.

### 11. Update mentor status

Row **Approve** / **Decline**. One endpoint.

```http
PATCH /api/mentorship/programs/:programId/mentors/:mentorId
```

**Backend:** `PATCH /v1/programs/{uid}/members/{memberId}` (program `writer`).

```json
{ "status": "approved" }
```

`status` is `approved` \| `declined`. Honor the visibility table in endpoint 8.
`400` on an illegal transition.

#### Success `200`

The updated `MentorshipProgramMentor`.

### 12. Remove mentor

Row trash icon. Deletes the program membership; it is not a `withdrawn` status
write.

```http
DELETE /api/mentorship/programs/:programId/mentors/:mentorId
```

**Backend:** `DELETE /v1/programs/{uid}/members/{memberId}` (program `writer`).

#### Success `204`

Empty body.

---

## Tab 4 — Terms

### 13. List terms

```http
GET /api/mentorship/programs/:programId/terms
```

**Backend:** `GET /v1/programs/{uid}/term-management` (program `writer`).
Administrative term list with per-term application counts and action
eligibility.

#### Success `200`

```json
{
  "data": [
    {
      "id": "trm_gridflow_fall26",
      "name": "Fall 2026",
      "status": "open",
      "pending": 3,
      "declined": 1,
      "accepted": 2,
      "graduated": 0,
      "startDate": "2026-09-01",
      "endDate": "2026-11-01",
      "applicationStartDate": "2026-05-01",
      "applicationEndDate": "2026-07-15"
    }
  ],
  "total": 4
}
```

Counters are application counts for that term. `startDate` / `endDate` are
`YYYY-MM-01`. Application dates are `YYYY-MM-DD`.

A term **should close** when `status === open` and its end month is in the past.
The UI shows a warning icon; closing still goes through endpoint 16.

A term **cannot close** while `accepted > 0`. Return `409` from the close
endpoint with the message in `MENTORSHIP_TERM_CANNOT_CLOSE_MESSAGE`.

Max **4 open** terms (`MENTORSHIP_MAX_OPEN_TERMS`). Create and re-open must
enforce that.

### 14. Create term

Toolbar **Create Term**. Disabled when 4 terms are already open.

```http
POST /api/mentorship/programs/:programId/terms
```

**Backend:** `POST /v1/programs/{uid}/terms` (program `writer`).

```json
{
  "name": "Spring 2027",
  "startDate": "2027-03-01",
  "endDate": "2027-05-01",
  "applicationStartDate": "2027-01-06",
  "applicationEndDate": "2027-02-15"
}
```

Same date rules as enroll. New row starts `open` with zero counters.

#### Success `201`

`MentorshipProgramTermRow`.

#### Errors

| Status | When                  |
| ------ | --------------------- |
| `409`  | Already 4 open terms. |

### 15. Edit term

Row menu **Edit**. Allowed unless the term is both `closed` **and** past its
end date (historical terms are locked).

```http
PATCH /api/mentorship/programs/:programId/terms/:termId
```

**Backend:** `PATCH /v1/programs/{uid}/terms/{termId}` (program `writer`).

Body is the same date/name payload as create. This is not a status write.

#### Success `200`

Updated `MentorshipProgramTermRow`.

### 16. Close term

Row **Close**. Dedicated action.

```http
POST /api/mentorship/programs/:programId/terms/:termId/close
```

**Backend:** `POST /v1/programs/{uid}/terms/{termId}/close` (program `writer`).

**Close** (`open` → `closed`):

- `409` if `accepted > 0`.
- On success, auto-decline every `pending` application on this term (`pending`
  moves to `declined` on the term counters). Confirm copy is
  `MENTORSHIP_TERM_CLOSE_CONFIRM`.

#### Success `200`

Updated `MentorshipProgramTermRow` (including adjusted counters after close).

#### Errors

| Status | When                                                       |
| ------ | ---------------------------------------------------------- |
| `409`  | Term has accepted applications that haven't graduated yet. |

### 17. Re-open term

Row **Re-Open**. Dedicated action.

```http
POST /api/mentorship/programs/:programId/terms/:termId/reopen
```

**Backend:** `POST /v1/programs/{uid}/terms/{termId}/reopen` (program `writer`).

**Re-open** (`closed` → `open`):

- Only if the term has **not** ended.
- `409` if it would exceed 4 open terms.

#### Success `200`

Updated `MentorshipProgramTermRow`.

#### Errors

| Status | When                                                  |
| ------ | ----------------------------------------------------- |
| `409`  | Term has ended, or would exceed 4 open terms. |

### 18. Delete term

Row **Delete**. Allowed only when the term has **no** applications
(`pending + declined + accepted + graduated === 0`).

```http
DELETE /api/mentorship/programs/:programId/terms/:termId
```

**Backend:** `DELETE /v1/programs/{uid}/terms/{termId}` (program `writer`).

#### Success `204`

#### Errors

| Status | When                       |
| ------ | -------------------------- |
| `409`  | Term has any applications. |

---

## Shared actions

### 19. Save reviewer note

Note dialog on a mentee row (Current Mentees or Applicants). Local-only today.

```http
PUT /api/mentorship/applications/:applicationId/note
```

**Backend:** `PUT /v1/applications/{uid}/note` (application `reviewer`).
The note is scoped to the application, not to a program+person pair.

```json
{ "note": "Strong Go background; paired well during the screening exercise." }
```

Max 2000 characters (`MENTORSHIP_MENTEE_NOTE_MAX`). Empty string clears the
note. The note is visible to the program's admins and mentors.

#### Success `200`

```json
{
  "applicationId": "app_alex_rivera_fall26",
  "note": "Strong Go background; paired well during the screening exercise."
}
```

---

## Not an API on this page

- **Download By Status** (mentees / applicants) — UI-only export from the loaded rows.
- **View / download task submission** — UI opens `task.file`.
- **Other Active Applications** — deferred. See authorization guide for rationale.

---

## Endpoint index

| #   | Method   | BFF path                                                | Backend route                                            | Why                         |
| --- | -------- | ------------------------------------------------------- | -------------------------------------------------------- | --------------------------- |
| 1   | `GET`    | `/programs/:programId`                                  | `GET /v1/programs/{uid}` + `GET /v1/programs/{uid}/management-summary` | Page header + tab counts    |
| 2   | `PATCH`  | `/programs/:programId`                                  | `PATCH /v1/programs/{uid}`                               | Edit Program                |
| 3   | `GET`    | `/programs/:programId/applications`                     | `GET /v1/programs/{uid}/applications`                    | Current / past / all (`type`) |
| 4   | `GET`    | `/applications/:applicationId/tasks`                    | `GET /v1/applications/{uid}/tasks`                       | View Tasks expansion        |
| 5   | `PATCH`  | `/applications/:applicationId/status`                   | `PATCH /v1/applications/{uid}/status`                    | Accept / Decline / Graduate / Hold |
| 6   | `POST`   | `/applications/tasks`                                   | `POST /v1/applications/{uid}/tasks` × N                  | Create Task (BFF loops applicationIds) |
| 7   | `POST`   | `/programs/:programId/terms/:termId/bulk-decline`       | `POST /v1/programs/{uid}/terms/{termId}/applications/bulk-decline` | Decline by Term |
| 8   | `GET`    | `/programs/:programId/mentors`                          | `GET /v1/programs/{uid}/member-management`               | Mentors tab list            |
| 9   | `GET`    | `/invitable-users`                                      | BFF-only (LF platform)                                   | Invite picker               |
| 10  | `POST`   | `/programs/:programId/mentors`                          | `POST /v1/programs/{uid}/members`                        | Invite (username → user_id) |
| 11  | `PATCH`  | `/programs/:programId/mentors/:mentorId`                | `PATCH /v1/programs/{uid}/members/{memberId}`            | Approve / Decline           |
| 12  | `DELETE` | `/programs/:programId/mentors/:mentorId`                | `DELETE /v1/programs/{uid}/members/{memberId}`            | Remove                      |
| 13  | `GET`    | `/programs/:programId/terms`                            | `GET /v1/programs/{uid}/term-management`                 | Terms tab list (with counts) |
| 14  | `POST`   | `/programs/:programId/terms`                            | `POST /v1/programs/{uid}/terms`                          | Create Term                 |
| 15  | `PATCH`  | `/programs/:programId/terms/:termId`                    | `PATCH /v1/programs/{uid}/terms/{termId}`                | Edit Term                   |
| 16  | `POST`   | `/programs/:programId/terms/:termId/close`              | `POST /v1/programs/{uid}/terms/{termId}/close`           | Close Term                  |
| 17  | `POST`   | `/programs/:programId/terms/:termId/reopen`             | `POST /v1/programs/{uid}/terms/{termId}/reopen`          | Re-open Term                |
| 18  | `DELETE` | `/programs/:programId/terms/:termId`                    | `DELETE /v1/programs/{uid}/terms/{termId}`                | Delete Term                 |
| 19  | `PUT`    | `/applications/:applicationId/note`                     | `PUT /v1/applications/{uid}/note`                        | Reviewer note               |

All BFF paths are under `/api/mentorship`.
