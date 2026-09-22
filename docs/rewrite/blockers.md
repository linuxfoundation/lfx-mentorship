<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Authorization Implementation Blockers

This document records the conditions that must be resolved before enabling
Heimdall/OpenFGA authorization for Mentorship. The backend implementation is
prepared for a staged rollout, but these blockers are deliberately kept
outside the traffic cutover until their owners provide the required evidence.

## Status Summary

| Blocker | Owner | Why it blocks | Current state |
|---|---|---|---|
| Shared OpenFGA model and executable fixtures | `lfx-v2-helm` | RuleSets and emitted tuples reference types and relations that do not exist until the model is deployed | Required before RuleSet activation |
| Project admin relation preservation | `lfx-v2-project-service` | Project full-state sync can delete Mentorship's direct admin tuples | Design is settled; external publisher change required |
| Project and approver roster authority | Mentorship/platform authority decision | FGA cannot distinguish an authoritative roster change from self-granting access | Storage shape exists for approvers; project-wide admin authority still requires ownership/API agreement |
| Complete PostgreSQL data invariants | Mentorship/data migration | Missing parents or project IDs produce incomplete inheritance chains | Migration foundation exists; backfill and enforcement remain |
| Heimdall/ArgoCD environment rollout | `lfx-v2-argocd`, platform owners | A correct chart is inert or unusable without real gateway, JWKS, audience, and model values | Local chart gates and first RuleSet pass exist; environment values are not landed |
| Gateway verification and cutover | Platform/release owners | Enabling a fail-closed RuleSet without seeded tuples denies valid traffic; leaving the interim path live bypasses edge authorization | Not started in a deployed environment |

## 1. Shared OpenFGA Model

**Owner:** `linuxfoundation/lfx-v2-helm`

### What must exist

The platform model must define the exact types and relations used by this
service:

- `mentorship_approver_team`
- `mentorship_program`
- `mentorship_application`
- `mentorship_task`
- `project#mentorship_program_admin`
- computed `project#mentorship_program_creator`

[lfx-v2-helm#177](https://github.com/linuxfoundation/lfx-v2-helm/pull/177)
introduces `mentorship_program#global_mentorship_approver` (userset-only) and
the computed `mentorship_program#approver`. The approver-team userset is
stamped on `global_mentorship_approver`, not on `auditor`, which now accepts
only `[user]` directly. `backend/internal/infrastructure/fga/payload.go`
currently emits `references.auditor` and must be changed before tuple emission
is enabled, or OpenFGA rejects the tuple as invalid and approvers cannot read
submitted programs.

The model must also include executable positive, negative, and inheritance
fixtures. The guide calls for the reviewed 14-scenario suite with checks for
program writers, inherited project writers, mentors, applicants, assignees,
approvers, cross-project denial, reviewer privacy, and parent-chain
inheritance.

### Why this blocks

The Heimdall RuleSet performs checks against concrete object types and relation
names. A RuleSet referencing an absent type or relation fails closed. A tuple
publisher can successfully enqueue JSON while the downstream authorization
system silently cannot evaluate the tuple against the deployed model. In that
state, the API appears healthy but every protected request is denied.

The opposite failure is also dangerous: a relation definition that is broader
than the approved model can authorize callers who should be denied. Parsing the
DSL is therefore insufficient; the model fixture suite is the acceptance
criterion.

### Required resolution evidence

1. `model.fga` is merged in `lfx-v2-helm`.
2. The authorization model version is bumped according to platform rules.
3. `fga model test` passes against the reviewed fixtures.
4. `helm lint` and the documented platform chart render pass.
5. The deployed `AuthorizationModelRequest` has the expected model version and
   ID.
6. The deployed model contains every type and relation referenced by the
   Mentorship RuleSet and contract.

## 2. Project Admin Relation Preservation

**Owner:** `linuxfoundation/lfx-v2-project-service`

### Required change

Project-service must include `mentorship_program_admin` in the
`exclude_relations` field of its full-state `project` `update_access` payload.
Mentorship owns the direct tuple:

```text
project:{project_uid}#mentorship_program_admin@user:{program_admin_lfid}
```

Project-service preserves that relation but does not derive, store, or manage
it.

### Why this blocks

`update_access` is a full synchronization operation. The fga-sync consumer
reads the live tuples for a project and removes publisher-managed relations
that are absent from the new payload. A Mentorship admin tuple is a direct
`user:` grant, not a `team:` grant, and therefore is deleted by the next
project update unless the relation is explicitly excluded.

The failure is silent and persistent:

1. Mentorship emits a valid `member_put`.
2. Cross-program administration works temporarily.
3. Project-service publishes an unrelated project update.
4. fga-sync removes the omitted Mentorship relation.
5. `mentorship_program.writer` inheritance fails closed for that admin.

Adding the relation to the OpenFGA model does not solve this lifecycle race.
The publisher that performs full-state synchronization must preserve the
relation.

### Required resolution evidence

1. Project-service ships the `exclude_relations` change.
2. A project update containing unrelated changes is exercised in an
   integration environment.
3. The Mentorship admin tuple remains after that update.
4. A cross-program `writer` check succeeds before and after the project update.
5. The project-service deployment version containing the change is recorded in
   the environment rollout checklist.

## 3. Roster Authority and Separation of Duties

**Owners:** Mentorship plus the platform authority designated by AQ-8

### Project-wide program admins

The existing `program_members` table is program-scoped and cannot represent a
user who administers every program under one LF project. The design requires a
Mentorship-owned roster keyed by `(project_uid, user_id)`.

That roster needs:

- a Postgres source-of-truth table;
- an LF-staff-authorized management API;
- removal history or durable outbox tombstones;
- `member_put`/`member_remove` emission for
  `project#mentorship_program_admin`;
- seed and reconciliation support.

### Global approvers

The `mentorship_approver_team:global` membership controls who can publish or
reject programs. Program admins must not be able to add themselves to this
roster. The owning authority must therefore be explicit and external to the
ordinary program-admin permission set.

### Why this blocks

Without an authoritative roster, FGA membership becomes an access-control
write path that can grant its own authority. A program admin who can insert an
approver tuple can approve their own program, defeating the separation of
publishing and approval encoded by the model.

A roster that exists only in OpenFGA also cannot be reliably reconciled. If a
member removal is lost after the source row is deleted, a later scan of
PostgreSQL cannot discover which tuple must be removed. This creates durable
stale access.

### Required resolution evidence

1. The authority allowed to administer the global approver roster is named.
2. The project-wide admin roster owner and API authorization are named.
3. Both rosters have relational source-of-truth storage.
4. Insert and removal transactions create generation-guarded outbox markers.
5. Seed and reconciliation cover both `member_put` and precise
   `member_remove` operations.
6. Self-add and cross-authority escalation tests are denied.

## 4. PostgreSQL Data Invariants and Backfill

**Owner:** Mentorship migration/data owners

### Required invariants

Before tuple emission is treated as complete:

- Every program has a non-null canonical `project_uid`.
- Every task has a non-null application parent.
- `tasks.application_id` uses `NOT NULL` and `ON DELETE CASCADE` after orphan
  repair.
- Every nested route verifies its child belongs to the path parent.
- Every emitted object ID is a canonical UID, never a slug.
- Every human principal used in a tuple resolves to an LFID.

### Why this blocks

The OpenFGA model relies on the inheritance chain:

```text
project -> mentorship_program -> mentorship_application -> mentorship_task
```

A missing project UID breaks project-admin inheritance. A task without an
application parent breaks reviewer/manager inheritance. A task that is
orphaned by `ON DELETE SET NULL` leaves an object that cannot be represented
correctly in the model. Publishing partial tuples is worse than failing: the
object may appear to exist while the intended inherited permissions do not.

### Required resolution evidence

1. Backfill reports all unmapped programs and unresolved task parents.
2. Unresolved rows are repaired or explicitly quarantined before enforcement.
3. Constraints are tightened only after the repair report is clean.
4. Seed refuses to continue on missing required identities or parents.
5. Nested-route tests prove a child from program B cannot be accessed through
   program A's path.

## 5. Heimdall and Environment Configuration

**Owners:** Mentorship chart owners, platform owners, `lfx-v2-argocd`

### Required configuration

Each environment must provide, consistently and atomically:

- Heimdall JWKS URL;
- Heimdall issuer `heimdall`;
- Heimdall audience `lfx-mentorship-backend`;
- shared gateway name and namespace;
- `lfx.domain`;
- Heimdall authorization endpoint;
- the deployed OpenFGA model configuration;
- NATS connectivity for the outbox relay.

The frontend/BFF also needs the shared gateway URL and gateway audience before
traffic moves.

### Why this blocks

The chart deliberately defaults to inert resources. Enabling only one piece
creates a broken or bypassable deployment:

- RuleSet without the model fails closed.
- HTTPRoute without Middleware references a missing extension.
- Middleware without HTTPRoute changes nothing.
- Heimdall JWT validation without the correct cluster JWKS rejects all gateway
  traffic.
- Gateway traffic without the relay and seed reaches a backend whose protected
  objects have no FGA tuples.
- Frontend traffic left on the interim backend host bypasses the shared gateway.

These values must be reviewed as one environment change, not copied separately
across releases.

### Required resolution evidence

1. `lfx-v2-argocd` values render successfully for each environment.
2. The chart renders the intended Middleware, HTTPRoute, RuleSet, and relay
   configuration.
3. The backend can validate a real Heimdall PS256 token from the cluster JWKS.
4. NATS JetStream publish acknowledgements are observed.
5. The frontend/BFF points at the shared gateway URL and requests the gateway
   audience.
6. Direct interim-host access is disabled or otherwise prevented at cutover.

## 6. Seed, Reconciliation, and Verification

**Owners:** Mentorship operators and platform authorization owners

### Why this remains a blocker

The current reconciliation command re-dirties current objects and current
memberships, but a current-row scan alone cannot discover a stale tuple after
the source membership row has been deleted. Deletion transactions are still
mandatory, and stale-removal history must be retained until delivery is
confirmed.

Relay success also means only that a message reached the JetStream stream. It
does not prove fga-sync applied the message or that OpenFGA contains the
expected tuple set.

### Required resolution evidence

1. Relay is deployed before seed begins.
2. Seed completes without missing-parent, missing-LFID, or invalid-data
   errors.
3. Post-seed checks compare PostgreSQL-derived expectations with OpenFGA.
4. Checks cover direct writers, inherited writers, mentors, applicants,
   assignees, parent references, approvers, public wildcard grants, revocations,
   and deletions.
5. Operators monitor oldest pending outbox age, retry exhaustion, publication
   failures, reconciliation mismatches, and revocation lag.

## 7. Cutover Gate

Gateway enforcement must remain disabled until all blockers above have evidence.
The cutover is not just a Helm flag:

1. Deploy the shared model and verify its live ID.
2. Deploy backend dual JWT acceptance, outbox relay, and reconciliation.
3. Seed and verify FGA coverage.
4. Deploy Middleware, HTTPRoute, and RuleSet without traffic first.
5. Run authenticated, anonymous, denied, cross-program, and stale-tuple smoke
   tests.
6. Point the frontend/BFF at the shared gateway.
7. Enable gateway traffic and retire the interim bypass path in the same change
   window.

Until then, keep `heimdall.enabled=false` in environment values. 
