# LFX Mentorship API — Developer Reference

**Base URL**: `http(s)://<host>/v1`  
**Content-Type**: `application/json` for all request and response bodies  
**Module**: `github.com/linuxfoundation/lfx-v2-mentorship-service`

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Authentication](#2-authentication)
3. [Common Patterns](#3-common-patterns)
4. [Error Reference](#4-error-reference)
5. [Health Probes](#5-health-probes)
6. [Users](#6-users)
7. [User Profiles](#7-user-profiles)
8. [Programs](#8-programs)
9. [Program Terms](#9-program-terms)
10. [Program Members (Mentors)](#10-program-members-mentors)
11. [Mentor Invite Tokens](#11-mentor-invite-tokens)
12. [Applications](#12-applications)
13. [Tasks](#13-tasks)
14. [Domain State Machines](#14-domain-state-machines)
15. [Business Rule Reference](#15-business-rule-reference)
16. [Frontend Integration Guide](#16-frontend-integration-guide)

---

## 1. Architecture Overview

The service is a single Go HTTP binary backed by a PostgreSQL database.  
It follows a 4-layer architecture:

```
Handler  →  Service  →  Repository  →  PostgreSQL (pgx/v5)
```

- **Handlers** (`internal/handler/`) decode HTTP requests, call one service method, encode the response.
- **Services** (`internal/service/`) enforce business rules, state machines, and guard conditions.
- **Repositories** (`internal/infrastructure/db/`) execute SQL; return typed domain objects.
- **Domain** (`internal/domain/`) defines model structs, repository interfaces, and sentinel errors.

### Entity Relationship (summary)

```
users
  └── user_profiles           (1:many; profile_type = mentor | mentee)

programs
  ├── program_skills           (many:1)
  ├── program_funding_stats    (1:1)
  ├── program_terms            (1:many)
  │     └── applications       (1:many; per term per user)
  │           └── tasks        (1:many; category = prerequisite)
  └── program_members          (1:many; program admins + mentors)

tasks                          (also created directly by program admins:
                                category = non_prerequisite)
```

---

## 2. Authentication

### JWT Bearer Token

Protected endpoints require an `Authorization: Bearer <token>` header.  
Tokens are Auth0-issued JWTs validated against the JWKS URL configured via environment variables.

| Env var | Description |
|---|---|
| `JWT_JWKS_URL` | Auth0 JWKS endpoint |
| `JWT_AUDIENCE` | Expected `aud` claim |
| `JWT_ISSUER` | Expected `iss` claim |

The JWT must contain the LFX SSO custom claims:

- `https://sso.linuxfoundation.org/claims/username` → `principal.Username` (the LF ID)
- `https://sso.linuxfoundation.org/claims/email` → `principal.Email`
- Standard `sub` claim → `principal.UserID`

#### Local Development Bypass

Set `ALLOW_MOCK_PRINCIPAL_BYPASS=true` and `MOCK_LOCAL_PRINCIPAL=<user-id>` to inject a static principal without a real JWT. **Never set these in production.**

### Public vs. Authenticated Endpoints

| Symbol | Meaning |
|---|---|
| 🔓 | No JWT required |
| 🔒 | `Authorization: Bearer <token>` required |
| 🪙 | Signed invite token in request body (no JWT) |

---

## 3. Common Patterns

### Pagination

All list endpoints accept optional query parameters:

| Parameter | Type | Default | Max | Description |
|---|---|---|---|---|
| `limit` | integer | 20 | 100 | Number of items per page |
| `offset` | integer | 0 | — | Zero-based row offset |

All list responses include a `meta` object:

```json
{
  "data": [...],
  "meta": {
    "total": 42,
    "limit": 20,
    "offset": 0
  }
}
```

### PATCH Semantics

All `PATCH` endpoints use **partial update semantics**: only fields present in the request body are updated. Omitted fields retain their current database value. Fields with `null` JSON values explicitly clear the column where the column is nullable.

### Timestamps

All timestamps are ISO-8601 strings with UTC timezone (e.g. `"2026-08-20T10:00:00Z"`).  
Date-only fields use `"YYYY-MM-DD"` format.

### Request Body Limit

All request bodies are capped at **1 MB**.

---

## 4. Error Reference

All errors return a JSON body:

```json
{ "error": "<human-readable message>" }
```

| HTTP Status | Condition |
|---|---|
| `400 Bad Request` | Missing required field, invalid enum value, malformed JSON |
| `401 Unauthorized` | Missing or invalid JWT |
| `403 Forbidden` | Authenticated but not permitted (e.g. wrong actor for task submission) |
| `404 Not Found` | Resource does not exist (or hidden program for non-owner) |
| `409 Conflict` | Duplicate resource, invalid state transition, guard blocked transition |
| `422 Unprocessable Entity` | Eligibility or business constraint failure |
| `503 Service Unavailable` | Database unavailable (`/readyz`) |
| `500 Internal Server Error` | Unexpected server fault |

---

## 5. Health Probes

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/livez` | 🔓 | Liveness — always 200 |
| `GET` | `/healthz` | 🔓 | Alias for livez |
| `GET` | `/readyz` | 🔓 | Readiness — pings DB; 503 if unreachable |

---

## 6. Users

Users represent LFX SSO identities.  

### User Object

```json
{
  "id":          "uuid",
  "email":       "user@example.com",
  "lfid":        "lf-username",
  "name":        "Alice Smith",
  "given_name":  "Alice",
  "family_name": "Smith",
  "avatar_url":  "https://...",
  "created_on":  "2026-01-01T00:00:00Z",
  "updated_on":  "2026-01-01T00:00:00Z"
}
```

All fields except `id`, `created_on`, and `updated_on` are optional.

### Endpoints

#### `GET /v1/users` 🔓

List users with optional search.

**Query parameters**

| Parameter | Description |
|---|---|
| `search` | Case-insensitive substring match on `name`, `email`, or `lfid` |
| `limit` / `offset` | Pagination |

**Response** `200`
```json
{ "data": [<User>, ...], "meta": { "total": 1, "limit": 20, "offset": 0 } }
```

---

#### `GET /v1/users/{id}` 🔓

Get a single user by UUID.

**Response** `200` → `<User>`  
**Errors** `404`

---

#### `POST /v1/users` 🔒

Create a user record. The caller must supply an `id` (UUID from the SSO system).

**Request body**
```json
{
  "id":          "uuid",           // required
  "email":       "user@example.com",
  "lfid":        "lf-username",
  "name":        "Alice Smith",
  "given_name":  "Alice",
  "family_name": "Smith",
  "avatar_url":  "https://..."
}
```

**Response** `201` → `<User>`  
**Errors** `400`, `409` (duplicate id/email/lfid)

---

#### `PATCH /v1/users/{id}` 🔒

Update mutable user fields.

**Request body** (all optional)
```json
{
  "email":       "new@example.com",
  "lfid":        "new-lfid",
  "name":        "New Name",
  "given_name":  "New",
  "family_name": "Name",
  "avatar_url":  "https://..."
}
```

**Response** `200` → `<User>`  
**Errors** `400`, `404`

---

#### `DELETE /v1/users/{id}` 🔒

Hard-delete a user record.

**Response** `204`  
**Errors** `404`

---

#### `GET /v1/users/{userId}/applications` 🔒

List all applications submitted by a user across all programs.

**Query parameters**

| Parameter | Values | Description |
|---|---|---|
| `status` | `pending\|accepted\|active\|declined\|withdrawn\|graduated\|hold` | Filter by status |
| `role` | `mentor\|mentee` | Filter by role |
| `limit` / `offset` | — | Pagination |

**Response** `200`
```json
{ "data": [<Application>, ...], "meta": {...} }
```

---

## 7. User Profiles

User profiles represent a participant's mentorship identity. `profile_type = mentee` or `mentor`.

### UserProfile Object

```json
{
  "id":                  "uuid",
  "user_id":             "uuid",
  "profile_type":        "mentee",
  "slug":                "alice-smith",
  "first_name":          "Alice",
  "last_name":           "Smith",
  "email":               "alice@example.com",
  "phone":               "+1-555-0100",
  "logo_url":            "https://...",
  "introduction":        "I am a software engineer...",
  "terms_and_conditions": true,
  "number_of_projects":  0,
  "address": {
    "country": "US",
    "city": "San Francisco",
    "address1": "123 Main St",
    "zipCode": "94105"
  },
  "demographics": {
    "gender": "female",
    "race": "Asian",
    "age": 25
  },
  "socioeconomics": {
    "income": "50000-75000",
    "educationLevel": "bachelor"
  },
  "skill_set": {
    "skills": ["Go", "Python"],
    "improvementSkills": ["Kubernetes"],
    "comments": "Eager to learn cloud-native"
  },
  "profile_links": {
    "resumeLink":     "https://...",
    "linkedinProfileLink": "https://linkedin.com/in/alice",
    "githubProfileLink":   "https://github.com/alice"
  },
  "created_on": "2026-01-01T00:00:00Z",
  "updated_on": "2026-01-01T00:00:00Z"
}
```

The `address`, `demographics`, `socioeconomics`, `skill_set`, and `profile_links` fields are free-form JSON objects stored as JSONB.

### Endpoints

#### `GET /v1/user-profiles` 🔓

**Query parameters**

| Parameter | Values | Description |
|---|---|---|
| `user_id` | UUID | Filter to one user's profiles |
| `profile_type` | `mentor\|mentee` | Filter by type |
| `limit` / `offset` | — | Pagination |

**Response** `200`
```json
{ "data": [<UserProfile>, ...], "meta": {...} }
```

---

#### `GET /v1/user-profiles/{id}` 🔓

**Response** `200` → `<UserProfile>`  
**Errors** `404`

---

#### `GET /v1/user-profiles/slug/{slug}` 🔓

Look up a profile by its unique slug.

**Response** `200` → `<UserProfile>`  
**Errors** `404`

---

#### `POST /v1/user-profiles` 🔒

Create a user profile.

**Eligibility gate (mentee only)**: A user may not hold more than one active `mentee` profile. The request is rejected with `422` if the user already has one.

**Request body**
```json
{
  "id":           "uuid",           // required; caller-supplied UUID
  "user_id":      "uuid",           // required
  "profile_type": "mentee",         // required; "mentor" | "mentee"
  "slug":         "alice-smith",
  "first_name":   "Alice",
  "last_name":    "Smith",
  "email":        "alice@example.com",
  "phone":        "+1-555-0100",
  "logo_url":     "https://...",
  "introduction": "...",
  "terms_and_conditions": true,
  "address":       { ... },
  "skill_set":     { "skills": ["Go"], "improvementSkills": [], "comments": "" },
  "profile_links": { "githubProfileLink": "https://github.com/alice", ... },
  "demographics":  { ... },
  "socioeconomics":{ ... }
}
```

**Response** `201` → `<UserProfile>`  
**Errors** `400`, `409` (duplicate id/slug), `422` (eligibility gate)

---

#### `PATCH /v1/user-profiles/{id}` 🔒

Update mutable profile fields (all optional).

**Response** `200` → `<UserProfile>`  
**Errors** `400`, `404`

---

#### `DELETE /v1/user-profiles/{id}` 🔒

Hard-delete a profile.

**Response** `204`  
**Errors** `404`

---

## 8. Programs

Programs are the top-level entity for a mentorship offering.

### Program Object

```json
{
  "id":                  "uuid",
  "name":                "CNCF Mentorship 2026",
  "slug":                "cncf-mentorship-2026",
  "status":              "published",
  "is_paid":             true,
  "description":         "...",
  "logo_url":            "https://...",
  "website_url":         "https://...",
  "repo_link":           "https://github.com/cncf/mentorship",
  "code_of_conduct":     "https://...",
  "industry":            "Cloud Native",
  "color":               "#0078D7",
  "lfid":                "alice",
  "cii_project_id":      "12345",
  "accept_applications": true,
  "terms_and_conditions": true,
  "program_term_status": "open",
  "discover_sort_rank":  1,
  "amount_raised":       50000.00,
  "mentee_needs":        { ... },
  "task_templates": [
    {
      "name": "Contribution PR",
      "description": "Submit a pull request to the project repository",
      "submitFile": null,
      "dueDate": null
    }
  ],
  "created_on": "2026-01-01T00:00:00Z",
  "updated_on": "2026-01-01T00:00:00Z"
}
```

**Status lifecycle**: `draft → submitted → published ↔ hidden | rejected → archived`

| Status | Meaning |
|---|---|
| `draft` | Being configured; not visible to public |
| `submitted` | Under reviewer inspection |
| `published` | Live; accepts applications |
| `hidden` | Soft-hidden; only visible to owner |
| `rejected` | Reviewer declined; program_admin may revise and resubmit |
| `archived` | Completed; read-only |

### Program Skill Object

```json
{
  "id":         "uuid",
  "program_id": "uuid",
  "skill":      "Go",
  "created_on": "2026-01-01T00:00:00Z",
  "updated_on": "2026-01-01T00:00:00Z"
}
```

### Program Funding Stats Object

```json
{
  "id":            "uuid",
  "program_id":    "uuid",
  "amount_raised": 50000.00,
  "created_on":    "2026-01-01T00:00:00Z",
  "updated_on":    "2026-01-01T00:00:00Z"
}
```

### Endpoints

#### `GET /v1/programs` 🔓

**Query parameters**

| Parameter | Values | Description |
|---|---|---|
| `status` | `draft\|submitted\|published\|hidden\|rejected\|archived` | Filter by status |
| `search` | string | Case-insensitive match on program name |
| `limit` / `offset` | — | Pagination |

**Response** `200`
```json
{ "data": [<Program>, ...], "meta": {...} }
```

---

#### `GET /v1/programs/catalog` 🔓

Paginated public catalog of programs with nested skills, terms, and active mentors in a single response. The existing `GET /v1/programs`, `/skills`, `/terms`, and `/members` endpoints are unchanged.

**Query parameters**

| Parameter | Values | Description |
|---|---|---|
| `search` | string | Case-insensitive match on program name|
| `skill` | string | Case-insensitive exact match on a program skill (`all` is ignored) |
| `status` | `acceptance\|in-progress\|completed` | Public discovery status derived from terms. Omit or `all` for every published program. |
| `sort_by` / `sortBy` | `accepting_first\|completed_first\|name_asc\|name_desc\|updated_oldest\|updated_newest` | Sort order. Defaults to `accepting_first`. |
| `limit` / `offset` | — | Pagination |

Always returns `status = published` programs. Draft, hidden, and other statuses are omitted.

**Response** `200`
```json
{
  "data": [
    {
      "id": "uuid",
      "name": "Kubernetes Contributors",
      "slug": "kubernetes-contributors",
      "status": "published",
      "is_paid": true,
      "description": "...",
      "logo_url": "https://...",
      "repo_link": "https://github.com/...",
      "created_on": "2026-01-01T00:00:00Z",
      "updated_on": "2026-01-01T00:00:00Z",
      "skills": ["Go", "Kubernetes"],
      "terms": [
        {
          "id": "uuid",
          "program_id": "uuid",
          "name": "Spring 2026",
          "status": "open",
          "start_date_time": "2026-03-02T00:00:00Z",
          "end_date_time": "2026-05-25T00:00:00Z",
          "application_start_date": "2025-11-03T00:00:00Z",
          "application_end_date": "2026-01-15T00:00:00Z",
          "discovery_label": "Apply Now"
        }
      ],
      "mentors": [
        {
          "id": "uuid",
          "user_id": "uuid",
          "name": "Jane Mentor",
          "avatar_url": "https://...",
          "introduction": "I mentor kernel contributors..."
        }
      ]
    }
  ],
  "meta": { "total": 42, "limit": 20, "offset": 0 }
}
```

Nested `terms` omit soft-deleted terms. Nested `mentors` are `member_type = mentor` and `status = active`, joined to `users` for name and avatar, and to the mentor `user_profiles` row for `introduction`.

LF project / foundation is not included yet — `programs.lfid` remains the owner username.

---

#### `GET /v1/programs/{id}/catalog` 🔓

Same catalog shape as `GET /v1/programs/catalog` for a single program (UUID or slug). Hidden programs follow the same FR-009 404 rule as `GET /v1/programs/{id}`.

**Response** `200` → `<ProgramCatalogItem>`  
**Errors** `404`

---

#### `GET /v1/programs/{id}/mentees` 🔓

Public list of accepted, active, and graduated mentees for a program (UUID or slug). Hidden programs follow the same FR-009 404 rule as `GET /v1/programs/{id}`. Pending, declined, withdrawn, and hold applications are omitted.

**Response** `200`
```json
{
  "data": [
    {
      "user_id": "uuid",
      "name": "Alex Mentee",
      "avatar_url": "https://...",
      "introduction": "I contribute to Kubernetes...",
      "email": "alex@example.com",
      "status": "active",
      "term_id": "uuid",
      "term_name": "Spring 2026"
    }
  ]
}
```

`status` is the application status: `accepted`, `active`, or `graduated`. Display fields come from `users` and the mentee `user_profiles` row. `term_name` is `program_terms.name`.

**Errors** `404`

---

#### `GET /v1/mentees` 🔓

Paginated public directory of mentees on **published** programs. Includes `accepted`, `active`, and `graduated` only. Pending, hold, declined, and withdrawn applications are omitted, as are mentees with no enrollment. The list is one row per mentee.

`GET /v1/user-profiles` and `GET /v1/programs/{id}/mentees` are unchanged.

**Query parameters**

| Parameter | Values | Description |
|---|---|---|
| `search` | string | Case-insensitive match on mentee name |
| `skill` | string | Case-insensitive exact match on a mentee profile skill (`all` is ignored) |
| `status` | `active\|graduated` | `active` includes `accepted` and `active`. Omit or `all` for every listed mentee. Other values return `400` |
| `limit` / `offset` | — | Pagination |

**Response** `200`
```json
{
  "data": [
    {
      "user_id": "uuid",
      "name": "Alex Mentee",
      "avatar_url": "https://...",
      "introduction": "I contribute to Kubernetes...",
      "skills": ["Go", "Kubernetes"],
      "status": "active",
      "joined_at": "2024-01-15T00:00:00Z",
      "program": {
        "id": "uuid",
        "name": "Kubernetes Contributors",
        "slug": "kubernetes-contributors",
        "logo_url": "https://..."
      },
      "mentors": [
        {
          "id": "uuid",
          "user_id": "uuid",
          "name": "Jane Mentor",
          "avatar_url": "https://...",
          "introduction": "I mentor kernel contributors..."
        }
      ]
    }
  ],
  "meta": { "total": 12, "limit": 20, "offset": 0 }
}
```

`meta.total` is the filtered count. Unfiltered header totals live on `GET /v1/mentees/summary`. Email is not included.

---

#### `GET /v1/mentees/summary` 🔓

Unfiltered directory totals for the header (“18 mentees across 7 programs”). Ignores search, skill, and status. Call once; it does not change when the list is filtered.

**Response** `200`
```json
{
  "mentee_count": 18,
  "program_count": 7
}
```

---

#### `GET /v1/mentees/{id}` 🔓

Public mentee profile by **user ID**. Programs, skills, terms, and mentors are loaded in separate queries and returned in one response.

**Response** `200` → list item fields plus:
```json
{
  "user_id": "uuid",
  "name": "Alex Mentee",
  "avatar_url": "https://...",
  "introduction": "...",
  "skills": ["Go"],
  "status": "active",
  "joined_at": "2024-01-15T00:00:00Z",
  "program": { "id": "uuid", "name": "Kubernetes Contributors", "slug": "kubernetes-contributors" },
  "mentors": [],
  "github_url": "https://github.com/alex",
  "linkedin_url": "https://linkedin.com/in/alex",
  "programs": [
    {
      "id": "uuid",
      "name": "Kubernetes Contributors",
      "slug": "kubernetes-contributors",
      "description": "...",
      "logo_url": "https://...",
      "status": "active",
      "skills": ["Go", "Kubernetes"],
      "terms": [
        {
          "id": "uuid",
          "name": "Spring 2026",
          "start_date_time": "2026-03-02T00:00:00Z",
          "end_date_time": "2026-05-25T00:00:00Z",
          "application_status": "active"
        }
      ],
      "mentors": []
    }
  ]
}
```

**Errors** `400` when `{id}` is not a UUID. `404` when the user has no accepted, active, or graduated mentee application on a published program.

---

#### `GET /v1/mentors` 🔓

Paginated public directory of **active** mentors on **published** programs. Invited, pending, declined, and withdrawn memberships are omitted, as are mentor-profile-only users with no membership. The list is one row per mentor.

`GET /v1/user-profiles` and `GET /v1/programs/{id}/members` are unchanged.

**Query parameters**

| Parameter | Values | Description |
|---|---|---|
| `search` | string | Case-insensitive match on mentor name |
| `skill` | string | Case-insensitive exact match on a mentor profile skill (`all` is ignored) |
| `limit` / `offset` | — | Pagination |

**Response** `200`
```json
{
  "data": [
    {
      "user_id": "uuid",
      "name": "Jane Mentor",
      "avatar_url": "https://...",
      "introduction": "I mentor kernel contributors...",
      "skills": ["Go", "Kubernetes"],
      "joined_at": "2024-01-15T00:00:00Z"
    }
  ],
  "meta": { "total": 8, "limit": 20, "offset": 0 }
}
```

`meta.total` is the filtered count. Unfiltered header totals live on `GET /v1/mentors/summary`. Email is not included.

---

#### `GET /v1/mentors/summary` 🔓

Unfiltered directory totals for the header (“8 mentors across 7 programs”). Ignores search and skill. Call once; it does not change when the list is filtered.

**Response** `200`
```json
{
  "mentor_count": 8,
  "program_count": 7
}
```

---

#### `GET /v1/mentors/{id}` 🔓

Public mentor profile by **user ID**. Programs, mentees, and profile links are loaded in separate queries and returned in one response.

**Response** `200` → list item fields plus:
```json
{
  "user_id": "uuid",
  "name": "Jane Mentor",
  "avatar_url": "https://...",
  "introduction": "...",
  "skills": ["Go"],
  "joined_at": "2024-01-15T00:00:00Z",
  "github_url": "https://github.com/jane",
  "linkedin_url": "https://linkedin.com/in/jane",
  "stats": { "programs_mentoring": 2, "current_mentees": 3, "mentees_graduated": 1 },
  "programs": [
    {
      "id": "uuid",
      "name": "Kubernetes Contributors",
      "slug": "kubernetes-contributors",
      "description": "...",
      "logo_url": "https://...",
      "skills": ["Go", "Kubernetes"],
      "terms": [
        {
          "id": "uuid",
          "name": "Spring 2026",
          "status": "open",
          "start_date_time": "2026-03-02T00:00:00Z",
          "end_date_time": "2026-05-25T00:00:00Z",
          "application_start_date": "2026-01-15T00:00:00Z",
          "application_end_date": "2026-02-28T00:00:00Z"
        }
      ],
      "mentors": []
    }
  ],
  "current_mentees": [
    {
      "user_id": "uuid",
      "name": "Alex Mentee",
      "introduction": "...",
      "program_name": "Kubernetes Contributors",
      "term_name": "Spring 2026",
      "status": "active"
    }
  ],
  "graduated_mentees": []
}
```

**Errors** `400` when `{id}` is not a UUID. `404` when the user is not an active mentor on a published program.

---

#### `GET /v1/programs/{id}` 🔓

Fetch a program by UUID or slug.

> **FR-009**: If the program has `status = "hidden"`, the endpoint returns `404` for all callers whose `principal.Username` does not match `program.lfid` (the owner's LF ID). Unauthenticated callers always receive `404` for hidden programs.

**Response** `200` → `<Program>`  
**Errors** `404`

---

#### `POST /v1/programs` 🔒

Create a program. New programs start in `draft` status.

**Request body**
```json
{
  "name":        "CNCF Mentorship 2026",  // required; must be unique
  "slug":        "cncf-mentorship-2026",  // required; must be unique
  "description": "...",
  "logo_url":    "https://...",
  "website_url": "https://...",
  "repo_link":   "https://github.com/cncf/mentorship",
  "code_of_conduct": "https://...",
  "lfid":        "alice",
  "cii_project_id": "12345",
  "is_paid":     true,
  "task_templates": [
    { "name": "Contribution PR", "description": "...", "submitFile": null, "dueDate": null }
  ]
}
```

**Response** `201` → `<Program>`  
**Errors** `400`, `409` (duplicate name or slug)

---

#### `PATCH /v1/programs/{id}` 🔒

Update program fields and/or transition status.

**Request body** (all optional)
```json
{
  "name":        "Updated Name",
  "description": "Updated description",
  "logo_url":    "https://...",
  "repo_link":   "https://...",
  "lfid":        "alice",
  "status":      "submitted",
  "task_templates": [...]
}
```

**Status transition rules** — see [§14 State Machines](#14-domain-state-machines).

| Transition | Guard condition |
|---|---|
| `draft → submitted` | `lfid`, `description`, `repo_link`, and `logo_url` must all be non-empty; at least 1 skill tag; at least 1 open term |
| `published → hidden` | No `pending`, `accepted`, or `graduated` applications on the program |

**Response** `200` → `<Program>`  
**Errors** `400`, `404`, `409` (invalid transition or guard blocked)

---

#### `DELETE /v1/programs/{id}` 🔒

Hard-delete a program and all child records.

**Response** `204`  
**Errors** `404`

---

#### `GET /v1/programs/{id}/skills` 🔓

**Response** `200`
```json
{ "data": [<ProgramSkill>, ...] }
```

---

#### `POST /v1/programs/{id}/skills` 🔒

Add a skill tag to a program.

**Request body**
```json
{ "skill": "Kubernetes" }
```

**Response** `201` → `<ProgramSkill>`  
**Errors** `400`, `409` (duplicate skill)

---

#### `DELETE /v1/programs/{id}/skills/{skillId}` 🔒

Remove a skill tag.

**Response** `204`  
**Errors** `404`

---

#### `GET /v1/programs/{id}/funding-stats` 🔓

**Response** `200` → `<ProgramFundingStats>`  
**Errors** `404`

---

## 9. Program Terms

A term is a time-bounded run of a program. Programs may have at most **4 open terms** simultaneously.

### ProgramTerm Object

```json
{
  "id":                    "uuid",
  "program_id":            "uuid",
  "name":                  "Spring 2026",
  "status":                "open",
  "active_users":          3,
  "start_date_time":       "2026-03-01T00:00:00Z",
  "end_date_time":         "2026-06-30T23:59:59Z",
  "application_start_date": "2026-01-15T00:00:00Z",
  "application_end_date":   "2026-02-15T23:59:59Z",
  "discovery_label":       "Apply Now",
  "created_on":            "2026-01-01T00:00:00Z",
  "updated_on":            "2026-01-01T00:00:00Z"
}
```

The `discovery_label` field is **computed on read** and not stored in the database:

| Condition | Label |
|---|---|
| `status != "open"` | `"Completed"` |
| `status == "open"` and no application dates set | `"In Progress"` |
| `status == "open"` and `now < application_start_date` | `"Coming Soon"` |
| `status == "open"` and `now` within window | `"Apply Now"` |
| `status == "open"` and `now > application_end_date` | `"In Progress"` |

**Status lifecycle**: `open ↔ closed | deleted`

### Endpoints

#### `GET /v1/programs/{id}/terms` 🔓

**Query parameters**

| Parameter | Values | Description |
|---|---|---|
| `status` | `open\|closed\|deleted` | Filter by status |
| `limit` / `offset` | — | Pagination |

**Response** `200`
```json
{ "data": [<ProgramTermWithLabel>, ...], "meta": {...} }
```

---

#### `GET /v1/program-terms/{id}` 🔓

**Response** `200` → `<ProgramTermWithLabel>`  
**Errors** `404`

---

#### `POST /v1/programs/{id}/terms` 🔒

Create a term under a program. Defaults to `status = "open"`.

**Request body**
```json
{
  "name":                  "Spring 2026",    // required
  "status":                "open",
  "start_date_time":       "2026-03-01T00:00:00Z",
  "end_date_time":         "2026-06-30T23:59:59Z",
  "application_start_date": "2026-01-15T00:00:00Z",
  "application_end_date":   "2026-02-15T23:59:59Z"
}
```

**Guard**: Creating an `open` term fails with `409` if the program already has 4 open terms.

**Response** `201` → `<ProgramTerm>`  
**Errors** `400`, `409` (open-term cap exceeded)

---

#### `PATCH /v1/program-terms/{id}` 🔒

Update term fields and/or status.

**Request body** (all optional)
```json
{
  "name":                  "Spring 2026 Updated",
  "status":                "closed",
  "start_date_time":       "2026-03-01T00:00:00Z",
  "end_date_time":         "2026-06-30T23:59:59Z",
  "application_start_date": "2026-01-15T00:00:00Z",
  "application_end_date":   "2026-02-15T23:59:59Z"
}
```

**Status transition guards**:

| Transition | Guard condition |
|---|---|
| `closed → open` (reopen) | `end_date_time` must still be in the future; open-term cap must not be reached |
| `open → closed` | No application on this term has `status = "accepted"` or `"active"` |

**Response** `200` → `<ProgramTerm>`  
**Errors** `400`, `404`, `409`

---

#### `DELETE /v1/program-terms/{id}` 🔒

Soft-delete a term by setting its `status = "deleted"`.

**Response** `204`  
**Errors** `404`

---

## 10. Program Members (Mentors)

Tracks the relationship between a user and a program as either `program_admin` or `mentor`.

### ProgramMember Object

```json
{
  "id":          "uuid",
  "program_id":  "uuid",
  "user_id":     "uuid",
  "member_type": "mentor",
  "status":      "invited",
  "email":       "mentor@example.com",
  "created_on":  "2026-01-01T00:00:00Z",
  "updated_on":  "2026-01-01T00:00:00Z"
}
```

**`member_type` values**: `program_admin`, `mentor`

**`status` values**: `invited`, `requested`, `pending`, `active`, `declined`, `withdrawn`

| Status | Meaning |
|---|---|
| `invited` | Program Admin sent an invitation; awaiting mentor response |
| `requested` | Mentor self-requested participation; awaiting program_admin approval |
| `pending` | Manual hold set by program_admin |
| `active` | Member is confirmed and participating |
| `declined` | Invitation or request was declined |
| `withdrawn` | Removed from the program |

### Endpoints

#### `GET /v1/programs/{id}/members` 🔓

**Query parameters**

| Parameter | Values | Description |
|---|---|---|
| `member_type` | `program_admin\|mentor` | Filter by type |
| `status` | See status values | Filter by status |
| `limit` / `offset` | — | Pagination |

**Response** `200`
```json
{ "data": [<ProgramMember>, ...], "meta": {...} }
```

---

#### `POST /v1/programs/{id}/members` 🔒

Add a member to a program.

**Invitation flow** (`member_type = "mentor"`, no status supplied):
- Record is created with `status = "invited"`.
- A signed invite token is generated and dispatched via `NotifyMentorInvited`.

**Self-request flow** (`member_type = "mentor"`, `status = "requested"` explicitly supplied):
- Record is created with `status = "requested"`.

**Program Admin flow** (`member_type = "program_admin"`):
- Record is created with `status = "active"`.

**Request body**
```json
{
  "user_id":     "uuid",       // required
  "member_type": "mentor",     // required; "program_admin" | "mentor"
  "status":      "requested",  // optional; if omitted, defaults per member_type above
  "email":       "mentor@example.com"
}
```

**Response** `201` → `<ProgramMember>`  
**Errors** `400`, `409`

---

#### `PATCH /v1/programs/{id}/members/{memberId}` 🔒

Update a member's status or email.

**Request body**
```json
{
  "status": "active",
  "email":  "new@example.com"
}
```

When `status = "declined"` is set via this endpoint, `NotifyMentorDeclined` is triggered.

**Response** `200` → `<ProgramMember>`  
**Errors** `400`, `404`

---

#### `DELETE /v1/programs/{id}/members/{memberId}` 🔒

> **FR-022**: This endpoint does **not** delete the record. It sets `status = "withdrawn"` and returns `204`.

**Response** `204`  
**Errors** `404`

---

## 11. Mentor Invite Tokens

These endpoints are called from the tokenised link in an invite email. The signed token acts as the credential — no JWT is required.

### Token Format

Tokens are HMAC-SHA256 signed strings encoding `programID:userID`. The signing secret is set via the `INVITE_SECRET` environment variable.

---

#### `POST /v1/mentor-invites/accept` 🪙

Accept a mentor invitation.

**Request body**
```json
{ "token": "<signed-invite-token>" }
```

**Effect**: Sets the matching `program_members` record's `status` from `invited` to `active`.

**Response** `200` → `<ProgramMember>`  
**Errors** `400` (invalid/expired token or no pending invite found)

---

#### `POST /v1/mentor-invites/decline` 🪙

Decline a mentor invitation.

**Request body**
```json
{ "token": "<signed-invite-token>" }
```

**Effect**: Sets `status` to `declined` and triggers `NotifyMentorDeclined`.

**Response** `204`  
**Errors** `400`

---

## 12. Applications

An application represents a mentee's (or mentor's) request to join a specific program term.

### Application Object

```json
{
  "id":                "uuid",
  "program_term_id":   "uuid",
  "user_id":           "uuid",
  "role":              "mentee",
  "status":            "pending",
  "program_term_status": "open",
  "start_date_time":   "2026-03-01T00:00:00Z",
  "end_date_time":     "2026-06-30T23:59:59Z",
  "attendance_type":   null,
  "tasks_submitted":   false,
  "admin_notified":    false,
  "created_on":        "2026-01-15T00:00:00Z",
  "updated_on":        "2026-01-15T00:00:00Z"
}
```

**`role` values**: `mentee`, `mentor`

**`status` lifecycle**: `pending → accepted → active → graduated | declined | withdrawn | hold`

| Status | Set by | Meaning |
|---|---|---|
| `pending` | System (on create) | Awaiting program_admin review |
| `accepted` | Program Admin | Mentee selected; `attendance_type` required |
| `active` | Program Admin | Program period has begun |
| `graduated` | Program Admin | Mentee completed the program |
| `declined` | Program Admin / bulk-decline | Not selected |
| `withdrawn` | Mentee (self) | Mentee voluntarily exited |
| `hold` | Program Admin | Pending additional information |

**`attendance_type` values**: `full_time`, `part_time` — **required** when `status = "accepted"`

**`tasks_submitted`**: Automatically set to `true` by the system when all `prerequisite` tasks on this application reach `submitted` or `complete`.

### Endpoints

#### `GET /v1/program-terms/{id}/applications` 🔓

List applications for a term.

**Query parameters**

| Parameter | Values | Description |
|---|---|---|
| `status` | Any application status | Filter |
| `role` | `mentor\|mentee` | Filter |
| `user_id` | UUID | Filter to a specific applicant |
| `limit` / `offset` | — | Pagination |

**Response** `200`
```json
{ "data": [<Application>, ...], "meta": {...} }
```

---

#### `GET /v1/applications/{id}` 🔓

**Response** `200` → `<Application>`  
**Errors** `404`

---

#### `POST /v1/program-terms/{id}/applications` 🔒

Submit an application to a term.

**Guards enforced**:
1. Term must have `status = "open"`.
2. Current date must fall within `application_start_date` and `application_end_date`.
3. No existing non-withdrawn application for this user+term (reapplication from `declined` is permanently blocked; reapplication from `withdrawn` is allowed while the window is open).

**After creation**: The program's `task_templates` JSONB array is cloned as individual `prerequisite` tasks linked to the new application.

**Request body**
```json
{
  "user_id": "uuid",     // required
  "role":    "mentee"    // required; "mentor" | "mentee"
}
```

**Response** `201` → `<Application>`  
**Errors** `400`, `401`, `409` (duplicate / blocked reapplication), `422` (window closed, term not open)

---

#### `PATCH /v1/applications/{id}` 🔒

Transition an application's status or update fields.

**Request body** (all optional)
```json
{
  "status":          "accepted",
  "attendance_type": "full_time",
  "start_date_time": "2026-03-01T00:00:00Z",
  "end_date_time":   "2026-06-30T23:59:59Z"
}
```

**Key rules**:

| Transition | Rule |
|---|---|
| Any status → `accepted` | `attendance_type` must be supplied (`full_time` or `part_time`) |
| `pending` → `withdrawn` | Only the applicant (`actor_id == user_id`) may self-withdraw |
| All others | Enforced by state machine; invalid transitions return `409` |

When status is set to `accepted`, `NotifyMenteeAccepted` is triggered.

**Response** `200` → `<Application>`  
**Errors** `400`, `401`, `403` (wrong actor for withdrawal), `404`, `409`

---

#### `DELETE /v1/applications/{id}` 🔒

> **FR-039**: This endpoint does **not** delete the record. It sets `status = "withdrawn"` and returns `204`. Only the applicant can withdraw their own application.

**Response** `204`  
**Errors** `401`, `403`, `404`

---

#### `POST /v1/program-terms/{id}/applications/bulk-decline` 🔒

Decline all `pending` applications for a term in one action.

**Response** `200`
```json
{ "declined": 7 }
```

**Errors** `401`, `404`

---

#### `GET /v1/program-terms/{id}/applications/export` 🔒

Export applications as a CSV file, optionally filtered by status.

**Query parameters**

| Parameter | Description |
|---|---|
| `status` | Filter to a specific application status |
| `limit` / `offset` | Pagination (default limit 20) |

**Response** `200`  
`Content-Type: text/csv`  
`Content-Disposition: attachment; filename="applications.csv"`

**CSV columns**: `id`, `user_id`, `role`, `status`, `attendance_type`, `tasks_submitted`, `created_on`

---

#### `GET /v1/program-terms/{id}/past-mentees` 🔒

Read-only list of accepted/active/graduated mentees for a (typically closed) term.

**Response** `200`
```json
{ "data": [<Application>, ...] }
```

---

## 13. Tasks

Tasks represent units of work assigned to a mentee. They are either:

- **prerequisite** — cloned from `program.task_templates` when an application is created; must all reach `submitted` or `complete` before `tasks_submitted` is set.
- **non_prerequisite** — assigned manually by a program_admin or mentor to an active mentee.

### Task Object

```json
{
  "id":                  "uuid",
  "application_id":      "uuid",
  "program_term_id":     "uuid",
  "assignee_id":         "uuid",
  "owner_id":            "uuid",
  "name":                "Submit a contribution PR",
  "description":         "Open a PR in the project repo fixing a good-first-issue",
  "category":            "prerequisite",
  "status":              "incomplete",
  "application_status":  "pending",
  "program_term_status": "open",
  "custom":              false,
  "submit_file":         null,
  "file":                null,
  "due_date":            "2026-02-10",
  "created_by":          "alice",
  "created_on":          "2026-01-15T00:00:00Z",
  "updated_on":          "2026-01-15T00:00:00Z"
}
```

**`category` values**: `prerequisite`, `non_prerequisite`

**`status` lifecycle**: `incomplete → in_progress → submitted → complete`

Backward reset to `incomplete` is always possible (by a reviewer only).

### Endpoints

#### `GET /v1/applications/{id}/tasks` 🔓

**Query parameters**

| Parameter | Description |
|---|---|
| `status` | Filter by task status |
| `assignee_id` | Filter by assignee UUID |
| `limit` / `offset` | Pagination |

**Response** `200`
```json
{ "data": [<Task>, ...], "meta": {...} }
```

---

#### `GET /v1/program-terms/{id}/tasks` 🔓

List all tasks for a program term across all applications.

**Query parameters**: same as above.

**Response** `200`
```json
{ "data": [<Task>, ...], "meta": {...} }
```

---

#### `GET /v1/tasks/{id}` 🔓

**Response** `200` → `<Task>`  
**Errors** `404`

---

#### `POST /v1/applications/{id}/tasks` 🔒

Create a non-prerequisite task and assign it to an active mentee.

**Request body**
```json
{
  "assignee_id":    "uuid",             // required
  "name":           "Write a blog post",
  "description":    "Summarise your learning",
  "category":       "non_prerequisite",
  "status":         "incomplete",       // defaults to "incomplete"
  "custom":         true,
  "submit_file":    "required",
  "due_date":       "2026-05-01",
  "program_term_id": "uuid",
  "owner_id":       "uuid"
}
```

**Response** `201` → `<Task>`  
**Errors** `400`, `401`, `404`

---

#### `PATCH /v1/tasks/{id}` 🔒

Update a task's status or metadata.

**Request body** (all optional)
```json
{
  "name":        "Updated task name",
  "description": "Updated description",
  "status":      "in_progress",
  "due_date":    "2026-05-15",
  "file":        "https://storage.example.com/submission.pdf"
}
```

**Actor permission rules (enforced when JWT is present)**:

| Transition | Required actor |
|---|---|
| `incomplete → in_progress` | Task **assignee** (mentee) only |
| `in_progress → submitted` | Task **assignee** (mentee) only |
| `submitted → complete` | **Non-assignee** (program_admin or mentor) only |
| Any state → `incomplete` (reset) | **Non-assignee** (program_admin or mentor) only |

Invalid forward transitions (e.g. `incomplete → complete`) return `409`.

**Side effect**: When the last `prerequisite` task for an application reaches `submitted` or `complete`, the system:
1. Sets `applications.tasks_submitted = true`.
2. Fires `NotifyAdminTasksSubmitted` to notify the program admin.

**Response** `200` → `<Task>`  
**Errors** `400`, `401`, `403` (wrong actor), `404`, `409` (invalid transition)

---

#### `DELETE /v1/tasks/{id}` 🔒

Hard-delete a task.

**Response** `204`  
**Errors** `401`, `404`

---

## 14. Domain State Machines

### Program Status

```
draft ──────────────────────────────────► submitted
                                              │
                              ┌───────────────┼─────────────────┐
                              ▼               ▼                 │
                          rejected        published             │
                              │           /       \             │
                              │      hidden    archived         │
                              │      /   \                      │
                              │  published archived             │
                              │                                 │
                              └──►submitted                     │
                                                                │
                              (rejected → resubmit directly)
```

| From | To | Notes |
|---|---|---|
| `draft` | `submitted` | All required fields present (lfid, description, repo_link, logo_url, ≥1 skill, ≥1 open term) |
| `submitted` | `published` | Reviewer approves |
| `submitted` | `rejected` | Reviewer declines |
| `published` | `hidden` | No pending/accepted/graduated applications |
| `published` | `archived` | Program complete |
| `hidden` | `published` | Unhide |
| `hidden` | `archived` | Program complete while hidden |
| `rejected` | `submitted` | Program Admin resubmits |

### Program Term Status

```
open ◄──── closed
  │
  ▼
deleted
```

| From | To | Guard |
|---|---|---|
| `open` | `closed` | No `accepted` or `active` applications on this term |
| `closed` | `open` | `end_date_time` is still in the future; fewer than 4 open terms on program |
| `open` | `deleted` | (soft delete) |

### Application Status

```
              ┌──────────────┐
              ▼              │
pending ──► accepted ──► active ──► graduated
  │   │        │
  │   │        └──► declined
  │   │
  │   └──► hold ──► accepted
  │         └──► declined
  │         └──► pending
  │
  └──► declined
  └──► withdrawn
```

| From | To | Actor | Notes |
|---|---|---|---|
| `pending` | `accepted` | Program Admin | `attendance_type` required |
| `pending` | `declined` | Program Admin | |
| `pending` | `hold` | Program Admin | Needs more info |
| `pending` | `withdrawn` | **Applicant only** | Self-withdrawal |
| `hold` | `accepted` | Program Admin | |
| `hold` | `declined` | Program Admin | |
| `hold` | `pending` | Program Admin | |
| `accepted` | `active` | Program Admin | Program period begins |
| `accepted` | `declined` | Program Admin | |
| `active` | `graduated` | Program Admin | Manual; never automatic |
| `active` | `declined` | Program Admin | |

### Task Status

```
incomplete ──► in_progress ──► submitted ──► complete
    ▲               │               │
    │               └───────────────┘ (reset by reviewer)
    └─────────────────────────────────
```

| From | To | Actor |
|---|---|---|
| `incomplete` | `in_progress` | Assignee (mentee) |
| `in_progress` | `submitted` | Assignee (mentee) |
| `submitted` | `complete` | Reviewer (non-assignee) |
| Any | `incomplete` | Reviewer (non-assignee) — reset |

---

## 15. Business Rule Reference

| ID | Rule | Where enforced |
|---|---|---|
| FR-003 | Max 4 open terms per program | `ProgramTermService.Create`, `.Update` |
| FR-004 | Submission requires all required fields + ≥1 open term | `ProgramService.Update` |
| FR-008 | Hide blocked while pending/accepted/graduated apps exist | `ProgramService.Update` |
| FR-009 | Hidden programs return 404 to non-owners | `ProgramHandler.GetByID` |
| FR-013 | Close term blocked while accepted/active apps exist | `ProgramTermService.Update` |
| FR-014 | Reopen term only if end_date in the future | `ProgramTermService.Update` |
| FR-016 | Apply only when term is open AND within window | `ApplicationService.Create` |
| FR-017 | Discovery label derived from status + window | `ProgramTerm.DiscoveryLabel()` |
| FR-022 | Member removal sets status=withdrawn (no hard delete) | `ProgramMemberHandler.Delete` |
| FR-025 | One active mentee profile per user max | `UserProfileService.Create` |
| FR-029 | New applications start at status=pending | `ApplicationService.Create` |
| FR-030 | No reapplication from declined; withdrawn OK while window open | `ApplicationService.Create` |
| — | Mentee accepted/active/graduated on one program cannot apply to another | `ApplicationService.Create` |
| FR-032 | Task templates cloned on application create | `ApplicationService.Create` |
| FR-033 | Task status transitions restricted by actor role | `TaskService.Update` |
| FR-034 | tasks_submitted auto-set + admin notified when all prereqs done | `TaskService.Update` |
| FR-035 | Task completion never changes application status | `TaskService.Update` |
| FR-036 | attendance_type required to accept application | `ApplicationService.Update` |
| FR-039 | Only applicant can withdraw own application | `ApplicationService.Update` |
| FR-044 | Bulk decline affects only pending applications | `ApplicationRepository.BulkDeclineByTerm` |

---

## 16. Frontend Integration Guide

### Authentication Flow

1. Obtain an Auth0 JWT via the LFX SSO login flow.
2. Store the token securely (memory or secure cookie).
3. Include `Authorization: Bearer <token>` on all 🔒 requests.
4. On `401` response, refresh the token or redirect to login.

### Suggested Page Flows

#### Discovery / Program Listing

```
GET /v1/programs?status=published&limit=20
→ Render list of cards with discovery_label from each term
```

For each program card, fetch its open terms:
```
GET /v1/programs/{id}/terms?status=open
→ Use term.discovery_label to show "Apply Now", "Coming Soon", etc.
```

Alternatively, a single catalog request includes skills, terms, and active mentors:
```
GET /v1/programs/catalog?limit=20
```

#### Program Detail Page

```
GET /v1/programs/{id}
GET /v1/programs/{id}/terms
GET /v1/programs/{id}/skills
GET /v1/programs/{id}/members?member_type=mentor&status=active
```

Alternatively, the same nested shape in one request:
```
GET /v1/programs/{id}/catalog
```

#### Mentees Directory

```
GET /v1/mentees/summary
→ Header: mentee_count and program_count (call once; not affected by filters)

GET /v1/mentees?search=&skill=&status=&limit=20&offset=0
→ Card list: name, introduction, skills, featured program, mentors, joined_at
```

```
GET /v1/mentees/{user_id}
→ Profile: same card fields plus github_url, linkedin_url, and programs[]
```

Do not compose the directory from `GET /v1/user-profiles` or by calling `GET /v1/programs/{id}/mentees` for every program.

#### Mentors Directory

```
GET /v1/mentors/summary
→ Header: mentor_count and program_count (call once; not affected by filters)

GET /v1/mentors?search=&skill=&limit=20&offset=0
→ Card list: name, introduction, skills, joined_at
```

```
GET /v1/mentors/{user_id}
→ Profile: same card fields plus github_url, linkedin_url, stats, programs[], current_mentees[], and graduated_mentees[]
```

Do not compose the directory from `GET /v1/user-profiles` or by calling `GET /v1/programs/{id}/members` for every program.

#### Applying to a Term (Mentee)

1. Check that the user has a mentee profile:
   ```
   GET /v1/user-profiles?user_id=<uid>&profile_type=mentee
   ```
2. If no profile exists, create one (enforce eligibility checks client-side before calling):
   ```
   POST /v1/user-profiles
   ```
3. Submit the application:
   ```
   POST /v1/program-terms/{termId}/applications
   Body: { "user_id": "<uid>", "role": "mentee" }
   ```
4. Poll / display the returned `status` and `tasks_submitted` flag.

#### Mentee Task Workflow

```
GET /v1/applications/{appId}/tasks

# Start work
PATCH /v1/tasks/{taskId}  Body: { "status": "in_progress" }

# Submit
PATCH /v1/tasks/{taskId}  Body: { "status": "submitted", "file": "<upload-url>" }
```

When all prerequisite tasks reach `submitted`/`complete`, the application's `tasks_submitted` flag is set to `true` — poll `GET /v1/applications/{id}` to detect this change.

#### Program Admin: Accept / Decline Applications

```
# List pending applications for a term
GET /v1/program-terms/{termId}/applications?status=pending

# Accept
PATCH /v1/applications/{id}
Body: { "status": "accepted", "attendance_type": "full_time" }

# Decline
PATCH /v1/applications/{id}
Body: { "status": "declined" }

# Bulk decline all pending
POST /v1/program-terms/{termId}/applications/bulk-decline
```

#### Program Admin: Invite a Mentor

```
POST /v1/programs/{programId}/members
Body: { "user_id": "<mentorUserId>", "member_type": "mentor" }
```

The system sends an email containing a link like:
```
https://mentorship.lfx.linuxfoundation.org/mentor-invite?token=<signed-token>
```

The frontend's invite landing page calls:
```
POST /v1/mentor-invites/accept   Body: { "token": "<token>" }
POST /v1/mentor-invites/decline  Body: { "token": "<token>" }
```

#### Mentor Self-Request

```
POST /v1/programs/{programId}/members
Body: { "user_id": "<uid>", "member_type": "mentor", "status": "requested" }
```

The program_admin then approves or declines:
```
PATCH /v1/programs/{programId}/members/{memberId}
Body: { "status": "active" }   // approve
Body: { "status": "declined" } // decline
```

#### Program Submission Workflow (Program Admin)

1. Create program in draft:
   ```
   POST /v1/programs
   ```
2. Add skill tags:
   ```
   POST /v1/programs/{id}/skills  Body: { "skill": "Go" }
   ```
3. Create at least one open term:
   ```
   POST /v1/programs/{id}/terms
   Body: { "name": "Spring 2026", "application_start_date": "...", "application_end_date": "..." }
   ```
4. Ensure `lfid`, `description`, `repo_link`, `logo_url` are set on the program:
   ```
   PATCH /v1/programs/{id}
   Body: { "lfid": "alice", "description": "...", "repo_link": "...", "logo_url": "..." }
   ```
5. Submit for review:
   ```
   PATCH /v1/programs/{id}  Body: { "status": "submitted" }
   ```
   Returns `409` with a descriptive error if any required field or guard condition is not met.

#### CSV Export

```
GET /v1/program-terms/{termId}/applications/export?status=accepted
→ Triggers CSV download with columns:
  id, user_id, role, status, attendance_type, tasks_submitted, created_on
```

### Optimistic UI and Polling

The API does not support WebSocket or SSE. For reactive UI:

- After mutating state (PATCH application, PATCH task), re-fetch the resource to show the latest state.
- For `tasks_submitted`, poll `GET /v1/applications/{id}` after each task update.
- The `meta.total` from list endpoints provides accurate counts for progress indicators.

### Pagination Conventions

```typescript
// TypeScript helper
interface PagedResponse<T> {
  data: T[];
  meta: { total: number; limit: number; offset: number };
}

function nextOffset(meta: PagedResponse<unknown>['meta']): number | null {
  return meta.offset + meta.limit < meta.total
    ? meta.offset + meta.limit
    : null;
}
```

### Error Handling Conventions

```typescript
async function apiFetch(url: string, opts?: RequestInit) {
  const res = await fetch(url, opts);
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new ApiError(res.status, body.error);
  }
  return res.json();
}

class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
  get isNotFound()     { return this.status === 404; }
  get isConflict()     { return this.status === 409; }
  get isIneligible()   { return this.status === 422; }
  get isUnauthorized() { return this.status === 401; }
}
```

### Environment Variables Reference

| Variable | Required | Default | Description |
|---|---|---|---|
| `PORT` | No | `8080` | HTTP listen port |
| `PG_DSN` | Yes | — | PostgreSQL connection string |
| `DB_MAX_CONNS` | No | `10` | pgxpool max connections |
| `DB_MIN_CONNS` | No | `2` | pgxpool min connections |
| `JWT_JWKS_URL` | Yes | — | Auth0 JWKS endpoint |
| `JWT_AUDIENCE` | Yes | — | Expected JWT `aud` claim |
| `JWT_ISSUER` | Yes | — | Expected JWT `iss` claim |
| `INVITE_SECRET` | Yes | — | HMAC secret for mentor invite tokens |
| `OTEL_ENDPOINT` | No | — | OpenTelemetry collector endpoint |
| `ALLOW_MOCK_PRINCIPAL_BYPASS` | No | `false` | Enable local dev JWT bypass |
| `MOCK_LOCAL_PRINCIPAL` | No | — | Static user ID for bypass mode |
