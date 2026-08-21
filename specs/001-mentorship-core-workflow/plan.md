# Implementation Plan: LFX Mentorship Core Workflow

**Branch**: `001-mentorship-core-workflow` | **Date**: 2026-08-20 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/001-mentorship-core-workflow/spec.md`

## Summary

Implement the LFX Mentorship v2 core workflow as a Go REST microservice backed by
PostgreSQL. The existing scaffold provides CRUD for all entities; this plan covers the
state-machine business rules, guard conditions, actor-based access control, notification
hooks, computed fields (discovery labels, `tasks_submitted`), and the new API endpoints
(bulk decline, CSV export, invitation token redemption) required to make the scaffold
fully spec-compliant.

## Technical Context

**Language/Version**: Go 1.24.0

**Primary Dependencies**: `github.com/go-chi/chi/v5` (HTTP router), `github.com/jackc/pgx/v5`
(PostgreSQL), `github.com/auth0/go-jwt-middleware/v2` (JWKS JWT), `go.opentelemetry.io/otel`
(tracing)

**Storage**: PostgreSQL; Docker on port 5433; schema in `backend/db/migrations/001_initial.up.sql`

**Testing**: `go test ./...` (standard library + table-driven tests)

**Target Platform**: Linux server (containerised Go binary)

**Project Type**: Go REST microservice

**Performance Goals**: <200ms p95 on all CRUD endpoints; no caching layer required at this scale

**Constraints**: All mutating endpoints require JWT auth; public read endpoints are open;
every DB interaction carries an OTel span; no external message bus — notifications are
in-process hooks for now

**Scale/Scope**: Initial release; small-to-medium concurrent load; no sharding required

## Constitution Check

*No ratified constitution — no gates to enforce. Re-check when constitution is ratified.*

## Project Structure

### Documentation (this feature)

```text
specs/001-mentorship-core-workflow/
├── plan.md              ← this file
├── research.md          ← Phase 0 output
├── data-model.md        ← Phase 1 output
├── quickstart.md        ← Phase 1 output
├── contracts/
│   └── api.md           ← Phase 1 output
└── tasks.md             ← Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
backend/
├── cmd/mentorship-api/
│   └── server.go                       # extend: new routes (bulk-decline, export, invite token)
├── db/migrations/
│   └── 001_initial.up.sql              # schema (already updated for this feature)
├── internal/
│   ├── domain/
│   │   ├── models/                     # extend: state-transition helpers, constants
│   │   ├── errors.go                   # extend: ErrInvalidStateTransition, ErrStateLocked
│   │   └── repository.go              # extend: guard-query methods
│   ├── infrastructure/db/              # extend: guard queries (count accepted apps, etc.)
│   ├── service/                        # extend: state machines, side effects, bulk ops
│   └── handler/                        # extend: new handler methods for new endpoints
└── WORKFLOW.md                         # (already updated)
```

**Structure Decision**: Existing 4-layer architecture retained. No new packages. All new
functionality slots into existing layers alongside current code.

## Complexity Tracking

No complexity violations — no new packages, no infrastructure additions beyond what's
already present.

