<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Response to the Architecture Review of the Authorization Guide

This document records how we will address the architecture feedback from Eric
Searcy and Jordan Evans on
[lfx-mentorship#157](https://github.com/linuxfoundation/lfx-mentorship/pull/157)
(the authorization implementation guide). It is the decision record; the
follow-up PRs listed at the end carry the actual doc and code changes. Written
for the reviewers to confirm before we rework the guide and the backend.

## What the review asked

1. **One service owns all tuples on an object** (Eric and Jordan, hard
   pushback). Project-service should own storage and FGA syncing of project
   roles — including a new assignable project-wide mentorship admin role — the
   way it already owns `meeting_coordinator` and `executive_director`. Two
   services must not manage relations on the same object and rely on
   `exclude_relations` to avoid stepping on each other.
2. **Collection filtering belongs in the Query Service** (Eric, raised as a
   risk, not a hard block). Services *define* access rules; *enforcement*
   happens centrally. Backend-side FGA queries and filtering decisions push
   authorization out of transparent, auditable RuleSets into opaque service
   code. Public reads may also be redundant with query-service's
   `public: true` bypass (anonymous access, `Cache-Control: public`).
3. **The interim topology is underspecified** (Eric). The guide does not say
   whether the parallel phase is two API gateways in front of one service or
   two service deployments over a shared cross-account Postgres, or how
   cross-account routing works.
4. Smaller items: no RuleSet checks the program `auditor` relation the model
   defines; Heimdall slug contextualizers should be unnecessary when every
   checked route is UID-only; presigned upload URLs are disallowed by the
   object-storage spec.

## Decision 1 — project-service owns `project#mentorship_program_admin`

**We accept the pushback and reverse AQ-4's `exclude_relations` design.**
The merged platform model
([lfx-v2-helm#177](https://github.com/linuxfoundation/lfx-v2-helm/pull/177))
already defines `mentorship_program_admin: [user]` and
`mentorship_program_creator: writer_guard or mentorship_program_admin` on
`project`, structurally the twin of `meeting_coordinator` /
`meetings_creator`. Ownership follows the model: whoever publishes the
`project` full-state `update_access` owns every `[user]` relation on it, and
that is project-service.

**In [lfx-v2-project-service](https://github.com/linuxfoundation/lfx-v2-project-service)**
(one PR, following the `meeting_coordinator` precedent exactly):

- `MentorshipProgramAdmins []UserInfo` on `ProjectSettings`
  (`internal/domain/models/project.go`). KV is schema-free JSON — no
  migration.
- A `ProjectMentorshipProgramAdminsAttribute()` in the Goa design, wired into
  `PUT /projects/{uid}/settings` payload and result; regenerate.
- The slice added to `enrichAllRoleFields` (email → LFID resolution) and to
  the converters, preserving already-stored LFIDs on lookup misses.
- One block in `buildFGAUpdateAccessMessage` emitting
  `relations["mentorship_program_admin"]`.
- `docs/fga-contract.md` and `docs/indexer-contract.md` rows in the same PR.
- No fga-sync changes — the relation already exists in the deployed model.

**In this repo**, the mentorship-owned project-admin surface is removed:

- The `mentorship_program_admins` table, the
  `/v1/projects/{projectUID}/mentorship-program-admins` routes, the
  project-admin paths in `roster_service.go` and `roster_repository.go`, the
  `project` special case in `fga/database_builder.go` (`buildMembership`),
  `reconcileProjectAdmins` in `cmd/fga-reconcile`, and the project-admin
  tombstones.
- Mentorship emits tuples **only** on its four owned types
  (`mentorship_program`, `mentorship_application`, `mentorship_task`,
  `mentorship_approver_team`). The never-populated `ExcludeRelations` field
  in `fga/payload.go` is deleted.
- `04-authorization-model.md` (AQ-4), `blockers.md` §2–3, and
  `docs/fga-contract.md` are rewritten to the ownership design;
  [05-heimdall-gateway.md](./05-heimdall-gateway.md) line 108 already states
  it and stays as-is.

`mentorship_approver_team:global` stays with mentorship — it is a
mentorship-owned type, consistent with "the service is responsible for all
tuples on its own types." AQ-8 (which authority administers that roster)
remains open and unchanged by this decision.

Side benefit: with no membership markers on foreign objects, the outbox loses
the cross-kind race between whole-object and membership markers on the same
key for `project` — the roster relay and its tombstones shrink to the
approver team only.

**Sequencing:** the backend removal can land now (nothing is enforced yet;
`heimdall.enabled` is false everywhere). The project-service PR replaces the
old "exclude_relations PR" as the external cutover prerequisite: it must be
deployed and its tuples verified before any environment enables Heimdall,
or cross-program admin checks fail closed.

## Decision 2 — collections move to the Query Service

We adopt the platform read path for everything that lists across
independently authorized objects, and keep the service-side surface minimal:

- **Index `mentorship_program`** via `lfx.index.mentorship_program` with an
  `IndexingConfig` of `access_check_object: mentorship_program:{uid}`,
  `access_check_relation: viewer`, `public` set from the program's published
  state, `parent_refs: ["project:{project_uid}"]`, and tags
  (`project_uid`, `program_status`, …). A mentorship
  `docs/indexer-contract.md` and the catalog registration land with it.
- **Public catalog, directory, and search/typeahead reads** are served by
  query-service (`type=mentorship_program`), where `public: true` skips FGA
  entirely, supports anonymous callers, and returns
  `Cache-Control: public` — answering the redundancy Eric flagged on the
  anonymous `allow_all` + Postgres-filter routes.
- **"Programs I manage"** (the one FGA-filtered collection in the guide,
  `GET /me/managed-programs`) is served through query-service instead of
  service-side FGA calls. Because `filter_grants=direct` does not expand
  inheritance, programs will carry a direct `writer` tuple for their admins
  as they already do for program creators, and inherited-only cases resolve
  via the caller's `project` grants plus `parent=project:{uid}` — the exact
  mechanics go in the reworked guide for review.
- **What stays in the service:** nested lists under a checked parent
  (`GET /programs/{uid}/applications` authorized as program `manager` at the
  edge — no per-object filtering), and `/me` self-scoped rows ("my
  applications", "my profile"), which filter by the resolved principal's own
  records only. Per the review, the `/me` pattern is acceptable; the rule we
  adopt is stronger and simpler: **the backend makes no FGA queries at all.**
  If an audience-scoped attribute split is ever needed (e.g. contact fields
  for auditors vs viewers), it is done as separate indexed attribute sets
  with different `access_check_relation`s, as committee-service does — not
  as service-side field filtering.

Note on scope: `02-target-architecture.md` excluded a Mentorship-owned
Elasticsearch. Publishing to the shared platform indexer/OpenSearch is a
different thing — it is how every v2 resource service serves list reads —
and does not reintroduce that exclusion. We start with `mentorship_program`
only; applications and tasks stay on nested, edge-authorized routes.

## Decision 3 — document the interim topology

The parallel phase is much smaller than the guide lets readers infer, and
the reworked guide will say so with a diagram:

- **One deployment, one Postgres.** The rewrite runs only in the LFX v2
  cluster against its own RDS instance in the v2 account. There is no legacy
  deployment to peer with: the legacy serverless system is retired by data
  migration ([03-migration-plan.md](./03-migration-plan.md)), not run in
  parallel behind the new gateway. No VPC peering, no cross-account routing,
  no shared cross-account Postgres.
- **Two ingress paths to the same Service** during steps 6–7 only: the
  interim hostname (`mentorship-api.*`, Auth0-validated, no edge
  authorization) and the gateway path (`lfx-api.*/mentorship/`,
  Heimdall-authorized). The service mounts both path prefixes and
  dual-accepts both token shapes; the interim path is retired in the same
  change window as cutover per environment, precisely because it is an
  authorization bypass while it exists.

## Decision 4 — smaller items

- **`auditor` on `mentorship_program`:** it *is* exercised by RuleSets,
  transitively — every program read checks `viewer`, and
  `viewer: [user:*] or auditor`, so approvers and project auditors reach
  non-public programs through the `viewer` check (there is no wildcard tuple
  until a program is published). The reworked guide will state this
  explicitly instead of leaving `auditor` looking dead. Whether the direct
  `[user]` grant on `auditor` earns its keep (privileged read-only LF staff)
  is flagged to the reviewers; if there is no product need we will propose
  dropping it from the model in a minor version bump.
- **Slug contextualizers: dropped.** Every route feeding an `openfga_check`
  is UID-only. The public slug→UID resolver route remains for the frontend;
  any service-to-service lookup need is met with a NATS request/reply, per
  the review.
- **Presigned upload URLs: removed from the guide.** Task submission uploads
  will follow the platform object-storage spec; the design goes through that
  spec's review rather than this guide.

## Follow-up PRs

| # | Repo | Change |
|---|---|---|
| 1 | lfx-mentorship | Rework the #157 guide: ownership section, outbox steps, RuleSet catalog notes, query-service read path, interim-topology diagram; revert its flip of `05` line 108 / `06` |
| 2 | lfx-mentorship | Docs: rewrite AQ-4, `blockers.md` §2–3, `docs/fga-contract.md` to the ownership design |
| 3 | lfx-mentorship | Backend: remove the project-admin roster/routes/emission and `ExcludeRelations`; approver-team roster unchanged |
| 4 | [lfx-v2-project-service](https://github.com/linuxfoundation/lfx-v2-project-service) | `mentorship_program_admins` in `ProjectSettings` + settings API + FGA emission + contract docs |
| 5 | lfx-mentorship | Indexer publication for `mentorship_program` + `docs/indexer-contract.md`; catalog reads and `managed-programs` via query-service |

PRs 1–3 are internal and can land immediately. PR 4 replaces the previous
external ask on project-service and joins the cutover prerequisites in
`blockers.md`. PR 5 follows once the reviewers confirm Decision 2's shape.
