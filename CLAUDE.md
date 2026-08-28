<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This repo hosts the LFX Mentorship platform — a Kubernetes-native rewrite of the legacy serverless Mentorship system ([jobspring](https://github.com/linuxfoundation/jobspring) backend + [lfx-mentorship-upgrade](https://github.com/linuxfoundation/lfx-mentorship-upgrade) frontend).

**Read `docs/rewrite/` before making changes** — it is the approved architecture:

- `docs/rewrite/01-current-system.md` — the legacy platform being replaced
- `docs/rewrite/02-target-architecture.md` — target architecture (start here)
- `docs/rewrite/03-migration-plan.md` — migration phases and open questions

## Conventions

- **Follow [lfx-crowdfunding](https://github.com/linuxfoundation/lfx-crowdfunding)** — it is the explicit template for this rewrite: same monorepo layout (`backend/` Go + Chi, `frontend/` Nuxt 4 BFF), same layered backend design (domain/service/handler/infrastructure), same deployment model (Helm charts in-repo, ArgoCD GitOps). When unsure how to structure something, look at how lfx-crowdfunding does it.
- **License headers**: every file needs the MIT/SPDX header (`Copyright The Linux Foundation and each contributor to LFX.` / `SPDX-License-Identifier: MIT`) — enforced by CI.
- **DCO**: sign off every commit (`git commit --signoff`) — enforced by CI.
- **Scope**: the goal is feature parity with the legacy platform. Scope exclusions (employer portal, Elasticsearch, SES, and others) are listed in `docs/rewrite/02-target-architecture.md` — do not reintroduce them.

## Architecture

Monorepo: Go backend + Nuxt frontend, following the lfx-crowdfunding layout.

```
backend/          Go 1.25 · Chi v5 · pgx v5 (Postgres) · OpenTelemetry
  cmd/            entrypoint
  internal/
    domain/       models + repository interfaces + sentinel errors
    service/      business logic and validation
    handler/      HTTP handlers, routing, error mapping
    infrastructure/  Postgres repository implementations
  db/migrations/  SQL schema (source of truth for CHECK constraints)
  charts/         Helm chart
frontend/         Nuxt 4 · Vue 3 · TypeScript
specs/            feature specs and task lists
```

**Layering rule**: `handler` → `service` → `domain` (interfaces) ← `infrastructure`. Handlers do no business logic; services never touch SQL; repositories return domain models, not driver types. `domain` imports nothing from the other layers. Services reach `infrastructure` only through the interfaces `domain` declares — the one exception is `infrastructure/auth`, a stateless token helper.

**Errors**: services return sentinel errors from `internal/domain` wrapped with `%w`. Not-found is per entity (`ErrUserNotFound`, `ErrProgramNotFound`, `ErrApplicationNotFound`, …); the rest are shared (`ErrInvalidInput`, `ErrIneligible`, `ErrConflict`, `ErrInvalidStateTransition`, `ErrStateLocked`, `ErrForbidden`, `ErrUnauthorized`, `ErrUpstreamUnavailable`). `handler.Error` maps them, plus Postgres SQLSTATEs, to HTTP status codes — `internal/handler/respond.go` is the single place status codes are decided, so add new mappings there rather than writing status codes in handlers.

**Commands** — from `backend/`: `make build`, `make test`, `make lint`, `make license-check`, `make db-migrate`. From `frontend/`: `npm run dev`, `npm run build`, `npm run lint`, `npm run format`.

## Terminology

The legacy platform used inconsistent vocabulary for the same concepts. The rewrite standardises it. **These terms are banned in new code, comments, docs, tests, and UI copy:**

| Do not use | Use instead | Why |
|---|---|---|
| `maintainer` | **Program Admin** (`program_admin` in code/schema) | "Maintainer" means an upstream project maintainer elsewhere in LFX; here the role administers a mentorship program. The `program_members.member_type` column already encodes `program_admin`. |
| `apprentice` | **mentee** | Legacy synonym for the same role. The schema, API, and product copy all use "mentee". |

Two further rules on status vocabulary:

- **`declined` is the shared term for a turned-down application**, for both mentor and mentee roles — `applications` unifies them via a `role` column, so the status vocabulary must not fork by role.
- **`rejected` is reserved for `ProgramStatus`** (program moderation) and must not be reused for applications or members.

The one exception: `docs/rewrite/01-current-system.md` and other passages *describing the legacy system* may use the old terms where they are historically accurate. Do not rewrite history — the ban applies to anything describing the new system.

## Code Style

Status, role, and category fields are **named string types with constants and an `IsValid()` method**, not plain `string` (see `internal/domain/models/`). When adding a field backed by a fixed set of values:

- Define the type and its constants alongside the model, mirroring `ProgramStatus` in `program.go`.
- Keep every constant in sync with the column's `CHECK` constraint in `backend/db/migrations/`.
- **Validate external input at the service boundary** with `IsValid()`, returning `domain.ErrInvalidInput`. Do not rely on the database constraint alone — several denormalised columns have no constraint, and a constraint violation yields a generic message rather than one naming the field.

## PR Self-Review Checklist

Before creating or updating a PR, review the entire diff and verify:

- **Clean code** — no dead code, no debug artifacts, no unnecessary complexity
- **No over-engineering** — solve the problem at hand, not hypothetical future problems
- **Best practices** — idiomatic Go, consistent with existing patterns in this repo; use named constants for repeated string/numeric values (error codes, status values, source names, event types) rather than inline literals — see **Code Style** above
- **Tests** — every non-trivial behavior is covered; error paths tested alongside happy paths
- **Clear architecture** — interfaces, packages, and responsibilities are well-defined and easy to follow
- **Terminology** — no banned terms (see **Terminology** above)

## Status

Backend implementation in progress: Go + Chi under `backend/` (`internal/{domain,service,handler,infrastructure}`), Postgres schema in `backend/db/migrations/`, Helm chart in `backend/charts/`. Run `make build`, `make test`, `make lint`, and `make license-check` from `backend/`. Tracking: [linuxfoundation/lfx-self-serve#1526](https://github.com/linuxfoundation/lfx-self-serve/issues/1526).
