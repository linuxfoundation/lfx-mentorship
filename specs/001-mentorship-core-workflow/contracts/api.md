# API Contracts: LFX Mentorship Core Workflow

**Phase**: 1 | **Date**: 2026-08-20 | **Plan**: [plan.md](plan.md)

Documents new and changed endpoints introduced by this feature. Existing CRUD endpoints
not changed by the state-machine work are omitted. All paths are prefixed `/v1`.
Auth required unless marked **public**.

---

## Program State Transitions

The existing `PATCH /v1/programs/{id}` is extended with state-machine validation.

### Existing endpoint — now with state guards

```
PATCH /v1/programs/{id}
Authorization: Bearer {token}
Content-Type: application/json

{ "status": "submitted" | "published" | "rejected" | "hidden" | "archived" | "draft" }
```

**Valid transitions** (all others → 409):

| From | To | Notes |
|------|----|-------|
| `draft` | `submitted` | Requires ≥1 open term and all required fields |
| `submitted` | `published` | Reviewer only |
| `submitted` | `rejected` | Reviewer only |
| `rejected` | `submitted` | Maintainer resubmit |
| `published` | `hidden` | Guard: no blocking apps (pending/accepted/graduated/hold) |
| `hidden` | `published` | |
| `published` \| `hidden` | `archived` | |

**Responses**:
- `200 OK` — updated program object
- `409 Conflict` — invalid transition or guard blocks
- `422 Unprocessable Entity` — submission guard (missing required fields or no open terms)

---

## Program Term State Transitions

### Existing endpoint — now with close guard

```
PATCH /v1/program-terms/{id}
Authorization: Bearer {token}
Content-Type: application/json

{ "status": "open" | "closed" | "deleted" }
```

**Close guard**: if `status = closed` and any application on this term has `status = accepted`
→ 409 `"term cannot be closed while accepted applications remain"`.

**Reopen guard**: if `status = open` and term `end_date_time < now` → 409 `"term end date
is in the past; update end_date_time before reopening"`.

**Open-term cap**: if `status = open` and program already has 4 open terms → 422 `"a program
may not have more than 4 open terms"`.

**Responses**: `200 OK` | `409 Conflict` | `422 Unprocessable Entity`

---

## Program Term — Discovery Label

### Existing endpoint — response extended

```
GET /v1/program-terms/{id}
```

Response now includes computed `discovery_label` field:

```json
{
  "id": "...",
  "status": "open",
  "application_start_date": "2026-09-01T00:00:00Z",
  "application_end_date": "2026-09-30T00:00:00Z",
  "discovery_label": "Coming Soon" | "Apply Now" | "In Progress" | "Completed"
}
```

Same field also present in items returned by `GET /v1/programs/{id}/terms`.

---

## Mentor Invitation Token Endpoints

Two public, token-authenticated endpoints for email link redemption.

### Accept invitation

```
POST /v1/mentor-invites/{token}/accept   [public]
```

- Validates JWT `token` (issued at invite time, 7-day TTL, contains `program_id` + `user_id`)
- Sets `program_members.status = active` for the matching record
- Notifies the program maintainer (in-process hook)

**Responses**:
- `200 OK` — `{ "status": "active", "program_id": "..." }`
- `410 Gone` — token expired or already redeemed
- `404 Not Found` — no matching `program_members` record

### Decline invitation

```
POST /v1/mentor-invites/{token}/decline  [public]
```

- Validates JWT token
- Sets `program_members.status = declined`
- Notifies the program admin (in-process hook)

**Responses**: same as accept

---

## Application State Transitions

### Existing endpoint — now with state guards and attendance_type

```
PATCH /v1/applications/{id}
Authorization: Bearer {token}
Content-Type: application/json

{
  "status": "accepted" | "declined" | "active" | "graduated" | "withdrawn" | "hold" | "pending",
  "attendance_type": "full_time" | "part_time"   // required when status = "accepted"
}
```

**Guards**:
- `status = accepted` requires `attendance_type` to be present → 422 if absent
- `status = withdrawn` is only allowed when the caller is the application's owner (mentee)
- `status = active | graduated` requires the application currently be `accepted | active`

**Side effects on `accepted`**:
1. `attendance_type` written to the record
2. Notification hook fired: mentee notified + HR paperwork triggered

**Responses**: `200 OK` | `409 Conflict` | `422 Unprocessable Entity` | `403 Forbidden`

---

## Bulk Decline

### New endpoint

```
POST /v1/program-terms/{id}/applications/bulk-decline
Authorization: Bearer {token}
Content-Type: application/json

{}   // no body; declines all pending applications on the term
```

**Behaviour**: Sets `status = declined` on every application for the term where
`status = pending`, regardless of `tasks_submitted` state. Returns count of affected rows.

**Response**:
```json
{ "declined_count": 12 }
```

`200 OK` | `404 Not Found` (term does not exist)

---

## CSV Export

### New endpoint

```
GET /v1/program-terms/{id}/applications/export?status=pending&tasks_submitted=true
Authorization: Bearer {token}
```

**Query parameters** (all optional):
- `status` — filter by application status
- `tasks_submitted` — `true | false`
- `role` — `mentor | mentee`

**Response**: `200 OK` with `Content-Type: text/csv`, streamed.

CSV columns:
`id, user_id, role, status, attendance_type, tasks_submitted, start_date_time, end_date_time, created_on`

---

## Task Status Transitions

### Existing endpoint — now with actor-based validation

```
PATCH /v1/tasks/{id}
Authorization: Bearer {token}
Content-Type: application/json

{ "status": "incomplete" | "in_progress" | "submitted" | "complete" }
```

**Actor rules**:
- **Mentee** (caller is the task's `assignee_id`): may transition `incomplete → in_progress`
  and `in_progress → submitted` only.
- **Maintainer / Mentor** (caller is the program's member with `member_type = maintainer |
  mentor` and `status = active`): may transition `submitted → complete` or reset any state
  to `incomplete`.

**Side effect when `submitted` or `complete`**: if all `prerequisite` tasks on the same
`application_id` are now `submitted` or `complete`, the service sets
`applications.tasks_submitted = true` and fires the admin notification hook.

**Responses**: `200 OK` | `403 Forbidden` | `422 Unprocessable Entity`

---

## Past Mentees View

### New endpoint

```
GET /v1/program-terms/{id}/past-mentees   [public]
```

Returns a read-only list of graduated and declined mentees for a `closed` term.
Only returns results when `program_terms.status = closed`; returns empty list for open terms.

**Response**:
```json
{
  "items": [
    {
      "application_id": "...",
      "user_id": "...",
      "status": "graduated" | "declined" | "withdrawn",
      "attendance_type": "full_time" | "part_time",
      "graduated_on": "2026-12-01T00:00:00Z"
    }
  ],
  "meta": { "total": 8, "limit": 20, "offset": 0 }
}
```
