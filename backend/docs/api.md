# LFX Mentorship API — Developer Reference

## Status Mapping

Program creation persists the canonical backend status `pending`. The BFF catalog
mapping shows a `pending` program in its pending-review display state.

**Base URL**: `https://lfx-api.<environment>/mentorship/v1` through the Heimdall gateway.
**Content-Type**: `application/json` for all request and response bodies  
**Module**: `github.com/linuxfoundation/lfx-v2-mentorship-service`

Gateway traffic uses Heimdall-signed JWTs and receives object authorization
from the gateway RuleSet/OpenFGA model.

### Gateway route changes

The gateway-authorized API uses canonical resource paths:

- Self-service user and profile mutations use `/me` and `/me/profiles`.
- User application reads use `/me/applications`.
- Mentor self-service program requests use `/me/program-memberships`.
- Program admins list their programs with `/me/programs`.
- Program-admin collection reads use Query Service
  `/query/resources?v=1&type=mentorship_program&filter_grants=direct`.
- Term-scoped routes use `/programs/{programUID}/terms/{termID}`.
- Profile records with duplicate profile types use `/me/profiles/by-id/{id}`.
- Application lifecycle operations are split into dedicated status, withdrawal,
  reapply, note, and evaluation routes.
- Task submission and reviewer operations use dedicated `/submission` and
  `/review` routes.

The legacy ID-addressed identity writes and un-nested term paths are not part of
the gateway contract. Program slugs must first be resolved through
`GET /programs/resolve/{id}` before calling an FGA-checked UID route.

### Authorization roster management

These cluster-local platform-management routes retain backend scope checks as
defense in depth:

- `GET/POST /admin/approver-team/members` requires
  `manage:mentorship:approvers`.
- `DELETE /admin/approver-team/members/{userID}` requires the same scope.

Global approver changes are persisted with their FGA membership marker
transactionally. The outbox relay publishes precise membership additions and
removals directly to the platform FGA stream.

These routes are intentionally absent from the Heimdall RuleSet until platform
owners approve an edge relation for LF-staff roster administration. A backend
scope is not an edge authorization decision and Heimdall does not synthesize
these scopes unless a rule explicitly configures them.

For local PostgreSQL-backed outbox tests, start `docker compose up -d`, create
an isolated `mentorship_test` database, and run:

```bash
TEST_DATABASE_DSN='postgres://mentorship:mentorship@localhost:5433/mentorship_test?sslmode=disable' make test-integration
```

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
14. [Files](#14-files)
15. [Domain State Machines](#15-domain-state-machines)
16. [Business Rule Reference](#16-business-rule-reference)
17. [Frontend Integration Guide](#17-frontend-integration-guide)

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

Clients send their identity-provider bearer token to the shared gateway. After
authorizing the request, Heimdall forwards a service-audience JWT to this
backend. Backend consumers do not obtain or submit the Heimdall JWT directly.

| Env var | Description |
|---|---|
| `HEIMDALL_JWKS_URL` | Heimdall JWKS endpoint |
| `HEIMDALL_JWT_AUDIENCE` | Expected `aud` claim |
| `HEIMDALL_JWT_ISSUER` | Expected `iss` claim |

The JWT must contain the `principal` claim. The service resolves human
principals to its local user record where workflow behavior needs a local ID.

#### Local Development Bypass

Set `ALLOW_MOCK_LOCAL_PRINCIPAL_BYPASS=true` and
`DISABLED_MOCK_LOCAL_PRINCIPAL=<user-id>` to inject a static principal without a
real JWT. **Never set these in production.**

### Public vs. Authenticated Endpoints

| Symbol | Meaning |
|---|---|
| 🔓 | No JWT required |
| 🔒 | `Authorization: Bearer <token>` required |
| 🪙 | Signed invite token in the path, plus `Authorization: Bearer <token>` for the invited user |

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
| `409 Conflict` | Duplicate resource, invalid state transition, guard blocked transition, or a concurrent write won |
| `413 Payload Too Large` | Upload over the file class's size cap |
| `415 Unsupported Media Type` | Upload whose bytes are not an allowed type for its file class |
| `416 Range Not Satisfiable` | Download `Range` outside the object |
| `422 Unprocessable Entity` | Eligibility or business constraint failure |
| `503 Service Unavailable` | Database unavailable (`/readyz`), or object storage unavailable or not configured (file routes only) |
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

#### `GET /v1/users` 🔒

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

#### `GET /v1/users/{id}` 🔒

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
  "family_name": "Name"
}
```

`avatar_url` is rejected: the service sets it from the login identity and the
profile-logo routes ([Files](#14-files)).

**Response** `200` → `<User>`  
**Errors** `400`, `404`

---

#### `DELETE /v1/users/{id}` 🔒

Hard-delete a user record.

**Response** `204`  
**Errors** `403`, `404`

---

#### `GET /v1/users/{userId}/applications` 🔒

List all applications submitted by a user across all programs.

**Query parameters**

| Parameter | Values | Description |
|---|---|---|
| `status` | `pending\|accepted\|declined\|withdrawn\|graduated\|hold` | Filter by status |
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
    "linkedinProfileLink": "https://linkedin.com/in/alice",
    "githubProfileLink":   "https://github.com/alice"
  },
  "created_on": "2026-01-01T00:00:00Z",
  "updated_on": "2026-01-01T00:00:00Z"
}
```

The `address`, `demographics`, `socioeconomics`, `skill_set`, and `profile_links` fields are free-form JSON objects stored as JSONB. `skill_set` must be an object or `null`, and `skill_set.skills` and `skill_set.improvementSkills` must each be absent, `null`, or an array of non-empty strings; any other shape is rejected with `400`.

### Endpoints

#### `GET /v1/user-profiles` 🔒

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

#### `GET /v1/user-profiles/{id}` 🔒

**Response** `200` → `<UserProfile>`  
**Errors** `404`

---

#### `GET /v1/user-profiles/slug/{slug}` 🔒

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

**Status lifecycle**: `pending → submitted → published | rejected`; `rejected → submitted`;
`published ↔ hidden`; `published | hidden → archived`. See [§15](#15-domain-state-machines).

| Status | Meaning |
|---|---|
| `pending` | Being configured; not visible to public |
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
  "amount_spent":  0.00,
  "created_on":    "2026-01-01T00:00:00Z",
  "updated_on":    "2026-01-01T00:00:00Z"
}
```

### Program Transaction Object

```json
{
  "id": "txn-1",
  "type": "donation",
  "amount_cents": 5000,
  "date": "2026-01-01T00:00:00Z",
  "category": "Mentorship",
  "recurring": false,
  "initiative_name": "Kubernetes Contributors",
  "donor_name": "Open Source Org",
  "donor_type": "organization",
  "donor_logo_url": "https://example.com/logo.png",
  "donor_username": ""
}
```

### Program Categorized Transactions Object

```json
{
  "individual_transactions": [<ProgramTransaction>, ...],
  "organization_transactions": [<ProgramTransaction>, ...],
  "total_count": 42,
  "limit": 10,
  "offset": 0
}
```

### Program Sponsor Object

```json
{
  "id": "gmc",
  "name": "GMC",
  "logo_url": "https://example.com/logo.png",
  "amount_cents": 250000
}
```

### Endpoints

#### `GET /v1/programs` 🔓

Always returns `published` programs.

**Query parameters**

| Parameter | Values | Description |
|---|---|---|
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

Always returns `status = published` programs. Pending, hidden, and other statuses are omitted.

Through the gateway this is the one service-owned collection route: Heimdall authenticates optionally and applies `allow_all`, so the published pin in the service is the only filter.

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

Public list of accepted and graduated mentees for a program (UUID or slug). Hidden programs follow the same FR-009 404 rule as `GET /v1/programs/{id}`. Pending, declined, withdrawn, and hold applications are omitted.

**Response** `200`
```json
{
  "data": [
    {
      "user_id": "uuid",
      "name": "Alex Mentee",
      "avatar_url": "https://...",
      "introduction": "I contribute to Kubernetes...",
      "status": "accepted",
      "term_id": "uuid",
      "term_name": "Spring 2026"
    }
  ]
}
```

`status` is the application status: `accepted` or `graduated`. Display fields come from `users` and the mentee `user_profiles` row. `term_name` is `program_terms.name`.

**Errors** `404`

---

#### `GET /v1/mentees` 🔓

Paginated public directory of mentees on **published** programs. Includes `accepted` and `graduated` applications only. Pending, hold, declined, and withdrawn applications are omitted, as are mentees with no enrollment. The list is one row per mentee.

The response `status` is the stored application status — `accepted` or `graduated`. Note the asymmetry with the `status` **query parameter** below, which accepts `active` as a filter alias selecting `accepted` rows; `active` is never returned in a response body.

`GET /v1/user-profiles` and `GET /v1/programs/{id}/mentees` are unchanged.

**Query parameters**

| Parameter | Values | Description |
|---|---|---|
| `search` | string | Case-insensitive match on mentee name |
| `skill` | string | Case-insensitive exact match on a mentee profile skill (`all` is ignored) |
| `status` | `active\|graduated` | `active` selects `accepted` applications. Omit or `all` for every listed mentee. Other values return `400` |
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
      "status": "accepted",
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
  "status": "accepted",
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
      "status": "accepted",
      "skills": ["Go", "Kubernetes"],
      "terms": [
        {
          "id": "uuid",
          "name": "Spring 2026",
          "start_date_time": "2026-03-02T00:00:00Z",
          "end_date_time": "2026-05-25T00:00:00Z",
          "application_status": "accepted"
        }
      ],
      "mentors": []
    }
  ]
}
```

**Errors** `400` when `{id}` is not a UUID. `404` when the user has no accepted or graduated mentee application on a published program.

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

#### `GET /v1/summary` 🔓

Aggregated marketing/landing counts plus a small graduated-mentee preview. All counts are scoped to programs with `status = "published"`.

- `program_count` — number of published programs.
- `accepting_program_count` — subset of published programs with at least one open term whose `application_start_date` ≤ `NOW()` ≤ `application_end_date`.
- `mentor_count` — distinct users who are an `active` `mentor` member of any published program.
- `graduated_mentee_count` — distinct users with at least one mentee application in status `graduated` on a non-deleted term of a published program.
- `stipends_paid` — total amount spent by published programs.
- `graduated_mentee_users` — up to four most recently graduated mentees (`name`, `avatar_url`) for the landing hero.

**Response** `200`
```json
{
  "program_count": 18,
  "accepting_program_count": 3,
  "mentor_count": 7,
  "graduated_mentee_count": 42,
  "stipends_paid": 12500.00,
  "graduated_mentee_users": [
    { "name": "Alex Mentee", "avatar_url": "https://..." },
    { "name": "Sam Graduate", "avatar_url": "https://..." }
  ]
}
```

---

#### `GET /v1/programs/{id}` 🔓

Fetch a program by UUID or slug.

> **FR-009**: If the program has `status = "hidden"`, the endpoint returns `404` for all callers whose `principal.Username` does not match `program.lfid` (the owner's LF ID). Unauthenticated callers always receive `404` for hidden programs.
>
> **Known gap**: the visibility-gated routes are registered without auth middleware, so no principal is ever present on them and the owner exception is currently unreachable — hidden programs return `404` to everyone, owner included. Tracked separately.

**Response** `200` → `<Program>`  
**Errors** `404`

---

#### `GET /v1/programs/resolve/{id}` 🔓

Resolve a program UUID or slug to the canonical program UUID.

> Only `published` programs resolve, except for the program's LFID owner; everyone else receives `404`.

**Response** `200`
```json
{ "id": "program-uuid" }
```
**Errors** `404`

---

#### `POST /v1/programs` 🔒

Create a program with its first terms, skills, and prerequisites in one transaction. New programs start in `pending` status and the slug is derived from `name`.

The caller resolves the LF project from Project Service and passes its UID, slug, name, and logo. They are persisted with the program and feed its search index snapshot (`project_slug`, `project_name`, `project_logo_url`).

**Request body**
```json
{
  "projectId":        "7cad5a8d-19d0-41a4-81a6-043453daf9ee", // required; Project Service UUID
  "projectSlug":      "cncf",                                 // required
  "projectName":      "Cloud Native Computing Foundation",   // required
  "projectLogoUrl":   "https://...",                          // optional; http(s)
  "name":             "CNCF Mentorship 2026",                 // required; must be unique
  "description":      "...",
  "repositoryUrl":    "https://github.com/cncf/mentorship",
  "websiteUrl":       "https://...",
  "codeOfConductUrl": "https://...",
  "industry":         "Cloud Native",                         // optional
  "ciiProjectId":     "12345",
  "skills":           ["Go"],                                 // required; at least one
  "terms": [                                                  // required; 1–4 terms
    { "name": "Spring 2026", "startDate": "2026-03-01", "endDate": "2026-05-31",
      "applicationStartDate": "2026-01-15", "applicationEndDate": "2026-02-15" }
  ],
  "prerequisites": [
    { "name": "Contribution PR", "description": "...", "required": true, "requireFile": false, "dueDate": null }
  ],
  "termsAccepted":    true                                    // required; must be true
}
```

**Response** `201` → `<Program>`  
**Errors** `400`, `409` (duplicate name or slug)

---

#### `PATCH /v1/programs/{id}` 🔒

Update program fields. Status cannot be changed here: a body with `status` returns
`400`; use [`POST /v1/programs/{id}/submit`](#post-v1programsidsubmit-) and
[`POST /v1/programs/{id}/decision`](#post-v1programsiddecision-).

**Request body** (all optional)
```json
{
  "name":        "Updated Name",
  "description": "Updated description",
  "repo_link":   "https://...",
  "lfid":        "alice",
  "skills":      ["Go", "Kubernetes"],
  "terms": [
    {
      "id":                     "b3c1...",
      "name":                   "Fall 2026",
      "application_start_date": "2026-07-01T00:00:00Z",
      "application_end_date":   "2026-07-31T00:00:00Z",
      "start_date_time":        "2026-09-01T00:00:00Z",
      "end_date_time":          "2026-11-30T00:00:00Z"
    },
    { "name": "Spring 2027", "application_start_date": "...", "application_end_date": "...", "start_date_time": "...", "end_date_time": "..." }
  ],
  "project_uid":      "7cad5a8d-19d0-41a4-81a6-043453daf9ee",
  "project_slug":     "new-project",
  "project_name":     "New Project",
  "project_logo_url": "https://...",
  "task_templates": [...]
}
```

The `project_*` fields move the program to another LF project and are applied
together: when any is present, `project_uid` (a Project Service UUID),
`project_slug`, and `project_name` are required, and an omitted or blank
`project_logo_url` clears the logo. Omit all four to leave the project unchanged.
Moving a program changes who can manage it, since access is inherited from the
project.

`skills` replaces the program's full skill set: skills not in the list are removed
and new ones are added. Entries are trimmed and de-duplicated case-insensitively; at
least one is required. Omit `skills` to leave them unchanged. The `<Program>`
response does not include skills; read them from
[`GET /v1/programs/{id}/skills`](#get-v1programsidskills-). The program's search
index snapshot is refreshed with the new skills in the same transaction.

`terms` is the program's full set of open terms, applied in the same transaction:
an entry with an `id` updates that open term's name and four dates, an entry
without one creates a new `open` term, and any open term not listed is deleted.
Closed terms are never listed and are left unchanged. Every entry needs a name and
all four dates, with the application window ending after it starts and before the
term starts; 1 to 4 entries are allowed. Term status is not changed here; use the
term close and reopen routes. Omit `terms` to leave them unchanged. The
`<Program>` response does not include terms; read them from
[`GET /v1/programs/{id}/terms`](#get-v1programsidterms-).

**Response** `200` → `<Program>`  
**Errors** `400` (including an `id` that is not an open term of the program), `404`,
`409` (removing an open term that has applications)

---

#### `POST /v1/programs/{id}/submit` 🔒

Submit a `pending` or `rejected` program for review (`→ submitted`). No request body.
See [§15 State Machines](#15-domain-state-machines).

**Guard**: a linked LF project, `description`, `repo_link` and `logo_url` are
non-empty; at least 1 skill tag; at least 1 open term.

**Response** `200` → `<Program>`  
**Errors** `404`, `409` (invalid transition or guard blocked)

---

#### `POST /v1/programs/{id}/decision` 🔒

Publish or reject a `submitted` program. Approver team only.

**Request body**
```json
{ "status": "published" }
```

`status` is required and must be `published` or `rejected`.

**Response** `200` → `<Program>`  
**Errors** `400` (missing or other `status`), `404`, `409` (program is not `submitted`)

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

#### `GET /v1/funding-stats/total` 🔓

Returns the total amount raised and spent across published programs.

**Response** `200`

```json
{
  "amount_raised": 500.00,
  "amount_spent": 165.50
}
```

**Errors** `500`

---

#### `GET /v1/programs/{id}/transactions` 🔓

Returns categorized donation transactions for a mentorship program by proxying the Crowdfunding category-transactions contract.

`{id}` accepts either program UUID or slug.

**Query parameters**

| Parameter | Values | Description |
|---|---|---|
| `categoryType` | string | Donation category filter (default: `mentorship`) |
| `subscriptionOnly` | `true\|false` | When `true`, include recurring-only transactions |
| `aggregate` | `true\|1\|yes\|aggregate` | When present/truthy, pages through all transactions and returns totals across the full dataset; otherwise only the first upstream page is aggregated |
| `aggregate` | `true\|1\|yes\|aggregate` | When present/truthy, pages through all transactions (`limit=2000`) and returns sponsors aggregated by organization plus one combined `Individual donors` row |
| `limit` / `offset` | — | Pagination (default `limit=10`, `offset=0`, max `limit=100`) |

**Response** `200` → `<ProgramCategorizedTransactions>`

**Errors**

- `400` invalid pagination parameters
- `404` program not found
- `503` upstream unavailable (Crowdfunding client not configured or upstream failure)

---

#### `GET /v1/programs/{id}/sponsors` 🔓

Returns sponsor cards aggregated in the mentorship backend from crowdfunding transactions.

`{id}` accepts either program UUID or slug.

**Query parameters**

| Parameter | Values | Description |
|---|---|---|
| `categoryType` | string | Donation category filter (default: `mentorship`) |
| `subscriptionOnly` | `true\|false` | When `true`, include recurring-only transactions |

**Response** `200`

```json
{ "data": [<ProgramSponsor>, ...] }
```

**Errors**

- `404` program not found
- `503` upstream unavailable (Crowdfunding client not configured or upstream failure)

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
| `status` | `open\|closed` | Filter by status; any other value returns `400`. Deleted terms are never listed |
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
| `open → closed` | No application on this term has `status = "accepted"` |

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
| `withdrawn` | Removed from the program, or the mentor withdrew their own request |

### Endpoints

#### `GET /v1/programs/{id}/members` 🔓

Public roster. Returns `active` members only, with `email` omitted from every row.
`{id}` may be a UUID or a slug. A hidden program returns `404`, matching
`GET /v1/programs/{id}`. The owner exception described under FR-009 does not
apply on this route yet — see the note there.

There is no `status` filter: the status is pinned to `active` so an anonymous
caller cannot widen the roster to `invited`, `requested`, `pending`, `declined`,
or `withdrawn` members.

**Query parameters**

| Parameter | Values | Description |
|---|---|---|
| `member_type` | `program_admin\|mentor` | Filter by type |
| `limit` / `offset` | — | Pagination |

**Response** `200`
```json
{ "data": [<ProgramMember>, ...], "meta": {...} }
```

**Errors** `404`

---

#### `POST /v1/programs/{id}/members` 🔒

Add a member to a program.

**Invitation flow** (`member_type = "mentor"`, no status supplied):
- Record is created with `status = "invited"`.
- A signed invite token is generated and dispatched via `NotifyMentorInvited`.

**Self-request flow** (`member_type = "mentor"`, `status = "requested"` explicitly supplied):
- Record is created with `status = "requested"`.
- No invite email is sent: `NotifyMentorInvited` fires only for `invited` rows.
  Mentors requesting for themselves should use
  `POST /v1/me/program-memberships`.

**Program Admin flow** (`member_type = "program_admin"`):
- Record is created with `status = "active"`.

**Request body** (identify the person with exactly one of `user_id`, `lfid` or, when neither is set, `email`; sending `user_id` and `lfid` together is a `400`)
```json
{
  "lfid":        "alice",      // or "user_id": "uuid", or only "email"; surrounding whitespace is trimmed
  "member_type": "mentor",     // required; "program_admin" | "mentor"
  "status":      "requested",  // optional; if omitted, defaults per member_type above
  "email":       "mentor@example.com"
}
```

Anyone with an LF account can be invited, whether or not they have used
Mentorship. An `email` is resolved to its LF account through auth-service,
matching the account's primary or linked emails. An `lfid` must match the LF
username exactly, including case. A person who already has a Mentorship user is
invited as that user and the invite email goes to their stored email, without
calling auth-service; only a user with no stored email, or a blank one, gets the account's
primary email filled in from auth-service. Otherwise a user is created from
auth-service (LFID, name, avatar and primary email), the invite email goes to
that primary email, and their first sign-in updates that same row.

**Response** `201` → `<ProgramMember>`  
**Errors** `400`, `404` (program not found), `409` (the user already has a row of this `member_type` on the program, in any status), `422` (no LF account has that `lfid` or `email`, or another Mentorship user already holds the account's primary email), `503` (auth-service is unreachable when it is needed)

---

#### `GET /v1/programs/{id}/mentor-candidates?search=` 🔒

Typeahead for the invite dialog; requires program `writer`, and the program must
be published. `search` must be at least 2 characters. A whole email is resolved
only through auth-service and returns at most the one LF account that owns it,
because Mentorship users can edit their stored email. Any other `search` returns
up to 10 Mentorship users matching part of a name or an LFID prefix; when none
is an exact LFID match and `search` is an LFID, the matching LF account from
auth-service is listed first. If auth-service fails for a non-email `search`,
the local matches are still returned, or `503` when there are none. Emails are never returned.

**Response** `200`
```json
{ "data": [ { "lfid": "alice", "name": "Alice Example", "avatar_url": "https://…" } ] }
```
**Errors** `400` (search too short, or the program is not published), `401`, `403`, `404` (program not found), `503` (auth-service is unreachable for an email `search`, or for an LFID `search` with no local matches)

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

When this endpoint moves a mentor's request to `declined` (`requested → declined`), `NotifyMentorDeclined` is triggered. Revoking an invite (`invited → declined`) sends no email.

A mentor's request belongs to the mentor: only they can create it or withdraw
it, through the [mentor self-service](#mentor-self-service) routes. A program
admin approves a request (`active`), declines it (`declined`), or deletes it.
This endpoint therefore refuses `requested`/`pending` → `withdrawn` and
`withdrawn` → `requested` with `409`.

**Response** `200` → `<ProgramMember>`  
**Errors** `400`, `403`, `404`, `409` (transition not allowed)

---

#### `DELETE /v1/programs/{id}/members/{memberId}` 🔒

Deletes the member row, in any status, and returns `204`. Removing an active
member also removes their OpenFGA relation. Deleting an invited, declined or
withdrawn mentor frees the user to be invited again. To remove an active mentor
but keep the record, `PATCH` the status to `withdrawn` instead.

**Response** `204`  
**Errors** `403`, `404`

---

#### `POST /v1/programs/{id}/members/{memberId}/resend-invite` 🔒

Signs a fresh 7-day invite token for an `invited` mentor and sends the
`mentor_invited` email again. Earlier tokens stay valid until they expire. No
request body.

**Response** `204`  
**Errors** `403` (not a Program Admin), `404`, `409` (the member is not an
`invited` mentor, or the program is not `published`), `503` (invites are not configured)

---

### Program admin self-service

#### AdministeredProgram Object

One row of the caller's programs list. `term` is a
[ProgramTerm](#programterm-object): the latest open term, else the latest
closed term, and is omitted when the program has no terms. `stats` matches `GET /v1/programs/{id}/header`.

```json
{
  "id":           "uuid",
  "slug":         "gridflow",
  "name":         "GridFlow: Time-Series Ingestion Pipeline",
  "project_uid":  "uuid",
  "project_name": "LF Energy",
  "logo_url":     "https://...",
  "status":       "published",
  "admin_status": "open",
  "term":         <ProgramTerm>,
  "stats":        { "mentors": 2, "mentees": 3, "graduated": 6 },
  "created_on":   "2026-01-01T00:00:00Z",
  "updated_on":   "2026-01-01T00:00:00Z"
}
```

`admin_status` groups `status` with the program's terms:

| `admin_status` | When |
|---|---|
| `pending_review` | `status` is `pending` or `submitted` |
| `open` | `status` is `published` and the program has an open term, or no terms yet |
| `completed` | `status` is `published` and every term is closed |
| `rejected` | `status` is `rejected` |
| `hidden` | `status` is `archived` or `hidden` |

#### `GET /v1/me/programs` 🔒

Lists the programs the caller is an active `program_admin` of, ordered by name.
The user is always the principal. The gateway requires only a signed-in user
(`oidc`); see the [route matrix](../../docs/rewrite/06-route-matrix.md) for why
this is not yet a Query Service collection.

**Query parameters**

| Parameter | Values | Description |
|---|---|---|
| `search` | — | Case-insensitive match on program or project name |
| `status` | `open\|pending_review\|completed\|rejected\|hidden` | Filter by `admin_status` |
| `limit` / `offset` | — | Pagination (default 20, max 100) |

**Response** `200`
```json
{ "data": [<AdministeredProgram>, ...], "meta": {...} }
```

**Errors** `400` (unknown `status`, non-integer `limit`/`offset`), `401` (no principal, or a machine-to-machine client)

---

### Mentor self-service

These routes let a signed-in user ask to mentor a program and track or withdraw
that request. The gateway requires only a signed-in user (`oidc`), with no
OpenFGA program relation: the caller usually has no role on the program yet.
The service scopes every read and write to the caller, taking the user from the
principal and never from the request.

#### ProgramMembership Object

The caller's view of one of their own `program_members` rows. `email` is omitted.

```json
{
  "id":           "uuid",
  "program_id":   "uuid",
  "program_name": "Example Program",
  "member_type":  "mentor",
  "status":       "requested",
  "created_on":   "2026-01-01T00:00:00Z",
  "updated_on":   "2026-01-01T00:00:00Z"
}
```

#### `GET /v1/me/program-memberships` 🔒

Lists the caller's program memberships in any status, newest first.

**Query parameters**

| Parameter | Values | Description |
|---|---|---|
| `member_type` | `program_admin\|mentor` | Filter by type |
| `limit` / `offset` | — | Pagination |

**Response** `200`
```json
{ "data": [<ProgramMembership>, ...], "meta": {...} }
```

**Errors** `400` (unknown `member_type`), `401`

---

#### `POST /v1/me/program-memberships` 🔒

Requests to mentor a program. Creates a `mentor` row for the caller with
`status = "requested"`. If the caller already has a `withdrawn` mentor row for
the program, that row is reset to `requested` instead. No invite email is sent.

**Request body**
```json
{ "program_id": "uuid" }
```

**Response** `201` → `<ProgramMember>`

**Errors**

| Status | When |
|---|---|
| `400` | `program_id` is not a UUID, or the program is `pending` |
| `401` | No signed-in user |
| `404` | The program does not exist, or is not visible to every signed-in user (`submitted`, `rejected`, `archived`, `hidden`) |
| `409` | The caller already has a mentor row in `invited`, `requested`, `pending`, `active`, or `declined`, or the row changed concurrently |

---

#### `POST /v1/me/program-memberships/{id}/withdraw` 🔒

Withdraws the caller's own mentor request, moving it from `requested` or
`pending` to `withdrawn`.

**Response** `204`

**Errors**

| Status | When |
|---|---|
| `401` | No signed-in user |
| `404` | No mentor row with this ID belongs to the caller. Rows owned by other users return `404`, not `403`, so IDs cannot be probed. |
| `409` | The row is in any status other than `requested` or `pending`, including when it changed concurrently |

---

## 11. Mentor Invite Tokens

These endpoints are called by the LFX Self Serve page that the invite email links to (`/mentorship/mentor/invites?token=…`). Both need the signed token **and** the invited user's JWT: the token says which program and user it was issued for, and the caller must be that user.

### Token Format

`base64url(JSON {program_id, user_id, exp}) + "." + base64url(HMAC-SHA256 signature)`, valid for 7 days. The signing secret is set via the `MENTOR_INVITE_SECRET` environment variable. Tokens are not stored, so one stays usable until it expires or the member row leaves `invited`.

---

#### `POST /v1/mentor-invites/{token}/accept` 🪙

Accept a mentor invitation. No request body.

**Effect**: Sets the matching `program_members` record's `status` from `invited` to `active`, and emails the program's active Program Admins (`NotifyAdminMentorAccepted`).

**Response** `200` → `<ProgramMember>`  
**Errors** `400` (invalid or expired token, or no pending invite — including one already answered), `401` (no JWT), `403` (the token belongs to another user), `409` (the row changed concurrently)

---

#### `POST /v1/mentor-invites/{token}/decline` 🪙

Decline a mentor invitation. No request body.

**Effect**: Sets `status` from `invited` to `declined`, and emails the program's active Program Admins (`NotifyAdminMentorDeclined`).

**Response** `204`  
**Errors** as for accept

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

**`status` lifecycle**: `pending → accepted → graduated | declined | withdrawn | hold`

| Status | Set by | Meaning |
|---|---|---|
| `pending` | System (on create) | Awaiting program_admin review |
| `accepted` | Program Admin | Mentee selected and enrolled for the term; `attendance_type` required |
| `graduated` | Program Admin | Mentee completed the program |
| `declined` | Program Admin / bulk-decline | Not selected |
| `withdrawn` | Mentee (self) | Mentee voluntarily exited |
| `hold` | Program Admin | Pending additional information |

**`attendance_type` values**: `full_time`, `part_time` — **required** when `status = "accepted"`

**`tasks_submitted`**: Automatically set to `true` by the system when all `prerequisite` tasks on this application reach `submitted` or `complete`.

### Endpoints

#### `GET /v1/program-terms/{id}/applications` 🔒

List applications for a term.

This endpoint is authenticated and actor-scoped:
- Active `mentor` and `program_admin` members on the owning program can list all term applications.
- Other authenticated callers are restricted to their own applications only.
- For non-reviewers, supplying `user_id` cannot widen access; the service pins filtering to the caller.

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

**Errors** `401`, `403`

---

#### `GET /v1/applications/{id}` 🔒

**Response** `200` → `<Application>`  
**Errors** `401`, `403`, `404`

---

#### `POST /v1/program-terms/{id}/applications` 🔒

Submit an application to a term.

**Guards enforced**:
1. Term must have `status = "open"`.
2. Current date must fall within `application_start_date` and `application_end_date`.
3. No existing non-withdrawn application for this user+term (reapplication from `declined` is permanently blocked; reapplication from `withdrawn` is allowed while the window is open).
4. Fewer than 3 withdrawn applications for this user+term in the requested role — once a user has withdrawn 3, no further application to the term in that role is accepted.

**After creation**: The program's `task_templates` JSONB array is cloned as individual `prerequisite` tasks linked to the new application.

**Reapplication** creates a new `pending` application under a new ID and keeps
the withdrawn one, unchanged, as history — its reviewer note, evaluation, and
tasks stay on it. A user may therefore hold several withdrawn applications for a
term, but at most one that is not withdrawn.

**Request body**
```json
{
  "role": "mentee"    // required; "mentor" | "mentee"
}
```

The applicant is always the caller. `attendance_type` is ignored; a Program
Admin sets it on acceptance.

**Response** `201` → `<Application>`  
**Errors** `400`, `401`, `409` (duplicate / blocked reapplication), `422` (window closed, term not open, reapplication limit reached)

---

#### `PATCH /v1/applications/{id}` 🔒

Update applicant-supplied application content. The gateway admits the applicant
and Program Admins (`writer` on the application).

**Request body** (all optional)
```json
{
  "start_date_time": "2026-03-01T00:00:00Z",
  "end_date_time":   "2026-06-30T23:59:59Z"
}
```

`status`, `attendance_type`, `program_term_status`, `tasks_submitted`,
`admin_notified`, `evaluation` and `reviewer_note` are rejected with `400`.
Status and attendance type change through `PATCH /v1/applications/{id}/status`;
withdrawal, evaluation and the reviewer note have their own routes; the rest are
maintained by the service.

**Response** `200` → `<Application>`  
**Errors** `400`, `401`, `403` (not a `writer` on the application), `404`

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

Read-only list of accepted/graduated mentees for a (typically closed) term.

**Response** `200`
```json
{ "data": [<Application>, ...] }
```

---

## 13. Tasks

Tasks represent units of work assigned to a mentee. They are either:

- **prerequisite** — cloned from `program.task_templates` when an application is created; must all reach `submitted` or `complete` before `tasks_submitted` is set.
- **non_prerequisite** — assigned manually by a program_admin or mentor to an accepted mentee.

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
  "file":                "/mentorship/v1/tasks/{id}/file-download",
  "due_date":            "2026-02-10",
  "created_by":          "alice",
  "created_on":          "2026-01-15T00:00:00Z",
  "updated_on":          "2026-01-15T00:00:00Z"
}
```

**`category` values**: `prerequisite`, `non_prerequisite`

**`file`** is the download route of the submission, present only when one is
uploaded. The stored object key never appears in a response or the search index,
which carries a `has_file` flag instead. See [Files](#14-files).

**`status` lifecycle**: the mentee moves `incomplete → in_progress → submitted`, a
reviewer completes; a mentor or program admin can also set any status directly.

**`due_date`** is an ISO 8601 date string (`YYYY-MM-DD`) for compatibility with
legacy task data. Writes with any other format are rejected with `400`.
Consumers performing date arithmetic should parse it as a
date rather than comparing it to a PostgreSQL timestamp directly.

Backward moves are made by a reviewer only.

### Endpoints

#### `GET /v1/applications/{id}/tasks` 🔒

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

**Errors** `401`, `403`, `404`

---

#### `GET /v1/program-terms/{id}/tasks` 🔒

List all tasks for a program term across all applications.

This endpoint is authenticated and actor-scoped:
- Active `mentor` and `program_admin` members on the owning program can list all term tasks.
- Other authenticated callers are restricted to tasks assigned to themselves.
- For non-reviewers, supplying `assignee_id` cannot widen access; the service pins `assignee_id` to the caller.

**Query parameters**: same as above.

**Response** `200`
```json
{ "data": [<Task>, ...], "meta": {...} }
```

**Errors** `401`, `403`

---

#### `GET /v1/tasks/{id}` 🔒

**Response** `200` → `<Task>`  
**Errors** `401`, `403`, `404`

---

#### `POST /v1/applications/{id}/tasks` 🔒

Create a non-prerequisite task and assign it to an accepted mentee.

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

A mentor's or program admin's full edit of a task: its content, its file
requirement, and its status.

**Request body** (all optional)
```json
{
  "name":        "Updated task name",
  "description": "Updated description",
  "category":    "non_prerequisite",
  "custom":      true,
  "status":      "in_progress",
  "submit_file": "required",
  "due_date":    "2026-05-15"
}
```

An omitted field is left unchanged. An empty string clears `submit_file` (no file
required) or `due_date`.

**Actor**: an active `mentor` or `program_admin` of the task's program, who is not
the task's assignee. Every other caller gets `403`, whatever fields the body holds.

**Status**: the caller may set any of `incomplete`, `in_progress`, `submitted` or
`complete`, from any current status — for example reopen a completed task, or
complete one that was never submitted. The one refusal is a task left `submitted`
while it requires a file and has none: setting `status: "submitted"`, or turning
`submit_file` on for a `submitted` task, returns `400` until the mentee uploads the
file. Moving the task to another status in the same request is allowed.

`file` is rejected with `400`: the submission is written by
`POST /v1/tasks/{id}/file-upload`. `application_status` and `program_term_status`
are rejected with `400`: they are set through `PATCH /v1/tasks/{id}/review`.

**Side effect**: when a new `status` or `category` leaves every `prerequisite` task
on the application `submitted` or `complete`, the system:
1. Sets `applications.tasks_submitted = true`.
2. Fires `NotifyAdminTasksSubmitted` to notify the program admin.

`tasks_submitted` records the first full submission. Moving a task back out of
`submitted` or `complete` leaves it set; the applicants list reads the tasks
themselves.

**Response** `200` → `<Task>`  
**Errors** `400`, `401`, `403`, `404`, `409` (withdrawn application)

---

#### `PATCH /v1/tasks/{id}/submission` 🔒

The assignee (mentee) moves their task forward.

**Request body**: `{ "status": "in_progress" | "submitted" }`

| Transition | Required actor |
|---|---|
| `incomplete → in_progress` | Task **assignee** only |
| `in_progress → submitted` | Task **assignee** only; a required file must be uploaded first |

Any other transition returns `409`. `file` is rejected with `400`. The
`tasks_submitted` side effect above applies.

**Response** `200` → `<Task>`  
**Errors** `400`, `401`, `403`, `404`, `409`

---

#### `PATCH /v1/tasks/{id}/review` 🔒

A reviewer's decision on a submitted task.

**Request body** (at least one field)
```json
{
  "status":              "complete" | "incomplete",
  "application_status":  "accepted",
  "program_term_status": "open"
}
```

The caller must be an active `mentor` or `program_admin` of the task's program, and
not its assignee, for every field.

| Transition | Required actor |
|---|---|
| `submitted → complete` | Reviewer |
| Any state → `incomplete` (reset) | Reviewer |

Any other `status` value returns `400`: the assignee's own steps go through
`PATCH /v1/tasks/{id}/submission`, and `PATCH /v1/tasks/{id}` sets any status.
Completing a task that is not `submitted` returns `409`.

**Response** `200` → `<Task>`  
**Errors** `400`, `401`, `403`, `404`, `409`

---

#### `DELETE /v1/tasks/{id}` 🔒

Hard-delete a task.

**Response** `204`  
**Errors** `401`, `403`, `404`

---

## 14. Files

File payloads live in S3-compatible storage, never in a response body or the
search index. The design is [02 §object storage](../../docs/rewrite/02-target-architecture.md#object-storage)
and the authorization is [06 §file routes](../../docs/rewrite/06-route-matrix.md#file-routes),
both added by #161.

- **Logos** (programs, profiles) go to the public bucket. The column stores the full
  CDN URL, returned as `public_url`. PNG or JPEG only (never SVG), at most 2 MB.
- **Task submissions** go to the private bucket. The column stores the object key,
  which responses replace with the download route. PDF, DOC, DOCX or plain text,
  at most 20 MB.
- The type is identified from the payload bytes, not the declared `Content-Type`.
  An unsupported type returns `415`; an oversized body `413`.
- Every upload writes a fresh `{uuid}-{filename}` key and never overwrites. The
  superseded object, and any upload that never commits, are deleted through the
  `object_deletions` queue.
- These routes are the only writers of `logo_url`, `avatar_url` and `file`. The
  generic create and update routes reject those fields with `400`. They drop
  `profile_links.resumeLink` rather than reject it, since resumes are not a file
  class and a client may echo back a migrated profile.
- An archived program's logo cannot change (`409`); a rejected program's can,
  since resubmitting requires a logo.

| Route | Body | Response |
|---|---|---|
| `POST /v1/programs/{id}/logo-upload` 🔒 | raw image | `201` `{ public_url, filename, content_type, size }` |
| `DELETE /v1/programs/{id}/logo` 🔒 | — | `204` |
| `GET /v1/programs/{id}/logo-download` 🔓 | — | `200` image; a fallback to `public_url` |
| `POST /v1/me/profiles/by-id/{id}/logo-upload` 🔒 | raw image | `201`; also sets the user's `avatar_url` |
| `DELETE /v1/me/profiles/by-id/{id}/logo` 🔒 | — | `204`; clears `avatar_url` while it holds that logo |
| `GET /v1/user-profiles/{id}/logo-download` 🔓 | — | `200` image, publicly listed profiles only |
| `POST /v1/tasks/{id}/file-upload` 🔒 | `multipart/form-data`, part `file` | `201` `{ filename, content_type, size }` |
| `GET /v1/tasks/{id}/file-download` 🔒 | — | `200`/`206` attachment, `Cache-Control: private, no-store`, `Range` supported |
| `DELETE /v1/tasks/{id}/file` 🔒 | — | `204`; only while `incomplete` or `in_progress` |

A task file can be uploaded until the task is `complete`; once `submitted` it is
replaced through `file-upload`, never deleted, so a task that requires a file
always keeps one. A task whose application is `withdrawn` keeps its file: upload and
delete return `409`. A concurrent upload to the same record returns `409`, as does an
upload that took longer than the 15-minute grace period to save. A `Range` outside
the object returns `416`.

---

## 15. Domain State Machines

### Program Status

```
pending ────────────────────────────────► submitted
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
| `pending` | `submitted` | All required fields present (linked LF project, description, repo_link, logo_url, ≥1 skill, ≥1 open term) |
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
| `open` | `closed` | No `accepted` applications on this term |
| `closed` | `open` | `end_date_time` is still in the future; fewer than 4 open terms on program |
| `open` | `deleted` | (soft delete) |

### Application Status

```
pending ──► accepted ──► graduated
  │   │        │
  │   │        └──► declined
  │   │
  │   └──► hold ──► accepted
  │         └──► declined
  │         └──► pending
  │
  └──► declined ──► pending
  └──► withdrawn
```

`accepted` is the enrolled state for the whole term; there is no intermediate
`active` application status.

| From | To | Actor | Notes |
|---|---|---|---|
| `pending` | `accepted` | Program Admin | `attendance_type` required |
| `pending` | `declined` | Program Admin | |
| `pending` | `hold` | Program Admin | Needs more info |
| `pending` | `withdrawn` | **Applicant only** | Self-withdrawal |
| `hold` | `accepted` | Program Admin | |
| `hold` | `declined` | Program Admin | |
| `hold` | `pending` | Program Admin | |
| `accepted` | `graduated` | Program Admin | Manual; never automatic |
| `accepted` | `declined` | Program Admin | |
| `declined` | `pending` | Program Admin | Re-open a declined application |

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
| Any | Any | Reviewer (non-assignee) — full edit through `PATCH /v1/tasks/{id}` |

---

## 16. Business Rule Reference

| ID | Rule | Where enforced |
|---|---|---|
| FR-003 | Max 4 open terms per program | `ProgramTermService.Create`, `.Update` |
| FR-004 | Submission requires all required fields + ≥1 open term | `ProgramService.Update` |
| FR-008 | Hide blocked while pending/accepted/graduated apps exist | `ProgramService.Update` |
| FR-009 | Hidden programs return 404 to non-owners | `handler.resolveVisibleProgram`, used by `ProgramHandler.GetByID` and `ProgramMemberHandler.List` |
| FR-013 | Close term blocked while accepted apps exist | `ProgramTermService.Update` |
| FR-014 | Reopen term only if end_date in the future | `ProgramTermService.Update` |
| FR-016 | Apply only when term is open AND within window | `ApplicationService.Create` |
| FR-017 | Discovery label derived from status + window | `ProgramTerm.DiscoveryLabel()` |
| FR-022 | Admin removal deletes the member row, in any status | `ProgramMemberHandler.Delete` |
| FR-025 | One active mentee profile per user max | `UserProfileService.Create` |
| FR-029 | New applications start at status=pending | `ApplicationService.Create` |
| FR-030 | No reapplication from declined; withdrawn OK while window open | `ApplicationService.Create` |
| — | Mentee accepted/graduated on one program cannot apply to another | `ApplicationService.Create` |
| FR-032 | Task templates cloned on application create | `ApplicationService.Create` |
| FR-033 | Task status transitions restricted by actor role | `TaskService.Update` |
| FR-034 | tasks_submitted auto-set + admin notified when all prereqs done | `TaskService.Update` |
| FR-035 | Task completion never changes application status | `TaskService.Update` |
| FR-036 | attendance_type required to accept application | `ApplicationService.Update` |
| FR-039 | Only applicant can withdraw own application | `ApplicationService.Update` |
| FR-044 | Bulk decline affects only pending applications | `ApplicationRepository.BulkDeclineByTerm` |

---

## 17. Frontend Integration Guide

### Authentication Flow

1. Send API requests through the shared gateway.
2. Include the identity-provider `Authorization: Bearer <token>` on protected requests; Heimdall replaces it before forwarding to the backend.
3. On `401` response, refresh the session or redirect to login.

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
GET /v1/programs/{id}/transactions?categoryType=mentorship&limit=25&offset=0
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

#### Landing Page

```
GET /v1/summary
→ Hero and stats: program_count, accepting_program_count, mentor_count,
  graduated_mentee_count, and graduated_mentee_users (up to four avatars)
```

Foundations and stipend totals are not on this endpoint yet — keep those as static marketing copy until they are modeled.

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
   Body: { "role": "mentee" }
   ```
4. Poll / display the returned `status` and `tasks_submitted` flag.

#### Mentee Task Workflow

```
GET /v1/applications/{appId}/tasks

# Start work
PATCH /v1/tasks/{taskId}/submission  Body: { "status": "in_progress" }

# Submit: upload the file first when the task requires one
POST /v1/tasks/{taskId}/file-upload  Body: multipart/form-data, part "file"
PATCH /v1/tasks/{taskId}/submission  Body: { "status": "submitted" }
```

When all prerequisite tasks reach `submitted`/`complete`, the application's `tasks_submitted` flag is set to `true` — poll `GET /v1/applications/{id}` to detect this change.

#### Program Admin: Accept / Decline Applications

```
# List pending applications for a term
GET /v1/program-terms/{termId}/applications?status=pending

# Accept
PATCH /v1/applications/{id}/status
Body: { "status": "accepted", "attendance_type": "full_time" }

# Decline
PATCH /v1/applications/{id}/status
Body: { "status": "declined" }

# Bulk decline all pending
POST /v1/program-terms/{termId}/applications/bulk-decline
```

#### Program Admin: Invite a Mentor

```
POST /v1/programs/{programId}/members
Body: { "user_id": "<mentorUserId>", "member_type": "mentor" }
```

The system sends an email containing a link to LFX Self Serve like:
```
https://app.lfx.dev/mentorship/mentor/invites?token=<signed-token>
```

The Self Serve invite page calls, as the signed-in mentor:
```
POST /v1/mentor-invites/{token}/accept
POST /v1/mentor-invites/{token}/decline
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

1. Create program in pending:
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
   POST /v1/programs/{id}/submit
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
| `DB_HOST`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` | Yes in deployments | — | Discrete PostgreSQL connection settings |
| `DB_PORT` | No | `5432` | PostgreSQL port |
| `DB_SSLMODE` | No | `require` | PostgreSQL TLS mode |
| `DATABASE_DSN` | Local/CI alternative | — | Used only when discrete `DB_*` values are absent |
| `DB_MAX_CONNS` | No | `10` | pgxpool max connections |
| `DB_MIN_CONNS` | No | `2` | pgxpool min connections |
| `HEIMDALL_JWKS_URL` | Yes | — | Heimdall JWKS endpoint |
| `HEIMDALL_JWT_AUDIENCE` | Yes | — | Expected JWT `aud` claim |
| `HEIMDALL_JWT_ISSUER` | Yes | — | Expected JWT `iss` claim |
| `FGA_NATS_URL` | Yes for relays | — | Shared NATS URL for FGA and index publishing, and notification email via lfx-v2-email-service |
| `PUBLIC_SITE_URL` | When `FGA_NATS_URL` is set | — | Public Mentorship site that user-facing email links point at |
| `SELF_SERVE_URL` | When `FGA_NATS_URL` is set | — | LFX Self Serve base URL for management links in email |
| `EMAIL_HR_INBOX` | When `FGA_NATS_URL` is set | — | LF staff HR inbox sent every mentee acceptance |
| `FGA_RELAY_BATCH_SIZE` | No | `50` | FGA/index claim batch size |
| `FGA_RELAY_INTERVAL` | No | `1s` | Relay polling interval |
| `FGA_RELAY_RETRY_DELAY` | No | `1m` | FGA retry delay |
| `FGA_RELAY_MAX_ATTEMPTS` | No | `10` | FGA attempts before dead letter |
| `INDEXER_SERVICE_TOKEN` | No | — | Service credential stamped on index messages; secret value |
| `INDEX_RELAY_RETRY_DELAY` | No | `1m` | Index publish retry delay |
| `INDEX_RELAY_MAX_ATTEMPTS` | No | `10` | Index attempts before dead letter |
| `MENTOR_INVITE_SECRET` | Yes | — | HMAC secret for mentor invite tokens |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | No | — | OpenTelemetry collector endpoint |
| `ALLOW_MOCK_LOCAL_PRINCIPAL_BYPASS` | No | `false` | Enable local dev JWT bypass |
| `DISABLED_MOCK_LOCAL_PRINCIPAL` | No | — | Static user ID for bypass mode |

Provision `INDEXER_SERVICE_TOKEN` in the backend Secret through Secrets Manager.
Without it the index relay idles and leaves outbox rows pending rather than
publishing messages the indexer would drop.
