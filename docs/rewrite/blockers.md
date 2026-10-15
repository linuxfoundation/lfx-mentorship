<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Authorization Implementation Blockers

This document records the conditions for Heimdall/OpenFGA authorization in
Mentorship. Authorization is enabled in dev and prod; the summary below shows
what is done and what is still open.

## Status Summary

| Blocker | Owner | Why it blocks | Current state |
|---|---|---|---|
| Shared OpenFGA model and executable fixtures | `lfx-v2-helm` | RuleSets and emitted tuples reference types and relations that do not exist until the model is deployed | Done: merged in [`lfx-v2-helm#177`](https://github.com/linuxfoundation/lfx-v2-helm/pull/177); prod runs `lfx-platform` 0.3.52, which includes it, and dev tracks `HEAD` |
| Project admin relation ownership | `lfx-v2-project-service` plus Mentorship/platform owners | Project full-state sync must not delete a relation owned by the project service; the project-wide mentorship admin role must be authoritative and live-verified before cutover | Deployed: [`lfx-v2-project-service#127`](https://github.com/linuxfoundation/lfx-v2-project-service/pull/127) is in prod (0.9.18) and dev. Live verification through an unrelated project update remains (§2) |
| Project and approver roster authority | Mentorship/platform authority decision | FGA cannot distinguish an authoritative roster change from self-granting access | Project-wide admin authority is settled (project-service, §2). The approver roster is stored in Postgres, but its admin routes stay cluster-local until a platform LF-staff relation is approved (§3) |
| Complete PostgreSQL data invariants | Mentorship/data migration | Missing parents or project IDs produce incomplete inheritance chains | Tasks are enforced (`NOT NULL`; parentless rows quarantined). `programs.lf_project_uid` stays nullable until unmapped programs are resolved |
| Heimdall/ArgoCD environment rollout | `lfx-v2-argocd`, platform owners | A correct chart is inert or unusable without real gateway, JWKS, audience, and model values | Done: dev and prod set `HEIMDALL_*` and `heimdall.enabled: true`; the backend accepts only Heimdall JWTs |
| Gateway verification and cutover | Platform/release owners | Enabling a fail-closed RuleSet without seeded tuples denies valid traffic; leaving the interim path live bypasses edge authorization | Self Serve traffic goes through the gateway in dev and prod, and the interim ingress is disabled. Open: the Nuxt server still calls the backend Service directly, bypassing Heimdall (§5, evidence item 5) |

## 1. Shared OpenFGA Model

**Owner:** `linuxfoundation/lfx-v2-helm`

The model landed in
[`lfx-v2-helm#177`](https://github.com/linuxfoundation/lfx-v2-helm/pull/177)
and has been refined by later PRs there.

### What must exist

The platform model must define the exact types and relations used by this
service:

- `mentorship_approver_team`
- `mentorship_program`
- `mentorship_application`
- `mentorship_task`
- `project#mentorship_program_admin`
- computed `project#mentorship_program_creator`

The `mentorship_program` type must also define the named
`global_mentorship_approver` relation as a userset of
`mentorship_approver_team#member`. Mentorship emits that exact key in
`references.global_mentorship_approver`; it is not interchangeable with the
program's `auditor` relation. The `auditor` relation must include
`global_mentorship_approver` so approvers can read pending programs without
receiving program-management permissions.

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
   Mentorship RuleSet and contract, including
   `mentorship_program#global_mentorship_approver` and
   `project#auditor_guard`.
7. `ProgramAccess` emits the userset under the separate payload key
   `references.global_mentorship_approver`, and the FGA contract test verifies
   that key independently from the deployed model relation check.

## 2. Project Admin Relation

**Owner:** `linuxfoundation/lfx-v2-project-service`

### Ownership

Project-service owns `project#mentorship_program_admin` end to end
([`lfx-v2-project-service#127`](https://github.com/linuxfoundation/lfx-v2-project-service/pull/127)):
the roster lives in project settings, is managed through project-service's API,
and is emitted in its full-state `update_access` for the project:

```text
project:{project_uid}#mentorship_program_admin@user:{program_admin_lfid}
```

Mentorship writes no tuple on `project`; it only consumes the inherited relation
for cross-program access checks.

### Why this blocked

`update_access` is a full synchronization operation: fga-sync removes
publisher-managed relations that are absent from the new payload. A tuple written
by any publisher other than project-service would be deleted by the next project
update, and `mentorship_program.writer` inheritance would fail closed for that
admin. Having project-service own the relation removes that race.

### Required resolution evidence

Done: the ownership is adopted and deployed — prod runs project-service 0.9.18,
which includes #127, and dev tracks `development`. Still to verify:

1. A project update containing unrelated changes is exercised in an
   integration environment.
2. The admin tuple remains after that update.
3. A cross-program `writer` check succeeds before and after the project update.

## 3. Global Approver Roster Authority and Separation of Duties

**Owners:** Mentorship plus the platform authority designated by AQ-8

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
2. The approver roster has relational source-of-truth storage.
3. Insert and removal transactions create generation-guarded outbox markers.
4. Transactional emission and the explicit seed cover `member_put` and precise
   `member_remove`; exact dead-letter markers can be requeued after repair.
5. Self-add and cross-authority escalation tests are denied.

## 4. PostgreSQL Data Invariants and Backfill

**Owner:** Mentorship migration/data owners

### Required invariants

Before tuple emission is treated as complete:

- Every program has a non-null canonical `project_uid`.
- Every task has a non-null application parent (enforced by `001_initial.up.sql`;
  parentless rows are held in `quarantined_tasks`).
- `tasks.application_id` uses `NOT NULL` and `ON DELETE CASCADE`.
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
- Service-owned caller-owned collections bypass Query Service's standard access
   filtering and direct-grant query pattern. Public collections (programs,
   catalog, mentor and mentee directories, and aggregates) are the exception:
   they return only published or publicly listable data, so there is nothing to
   filter.

These values must be reviewed as one environment change, not copied separately
across releases.

### Required resolution evidence

1. `lfx-v2-argocd` values render successfully for each environment.
2. The chart renders the intended Middleware, HTTPRoute, RuleSet, and relay
   configuration.
3. The backend can validate a real Heimdall PS256 token from the cluster JWKS.
4. FGA and index relay JetStream publish acknowledgements are observed.
5. The frontend/BFF points at the shared gateway URL and requests the gateway
   audience.
6. Direct interim-host access is disabled or otherwise prevented at cutover.
7. All resource collections other than the public program catalog are served by
   Query Service; caller-owned views use `filter_grants=direct` with an explicit
   resource `type`.

### Dual-gateway topology decision

Before enabling the shared gateway, document and verify the interim topology:

- whether both gateways route to one Mentorship deployment or to separate
   deployments;
- which gateway-to-Service path is used in each environment;
- whether the deployments share the same PostgreSQL instance and schema;
- how cross-account traffic reaches the gateway, Service, and database; and
- how the interim hostname is disabled so it cannot bypass Heimdall.

The current code supports two URL prefixes on one backend deployment. That is a
temporary migration shape, not evidence that cross-account routing or database
ownership has been solved.

## 6. Seed, Repair, and Verification

**Owners:** Mentorship operators and platform authorization owners

### Why this remains a blocker

The importer queues current-state object and index markers through the normal
outbox relays. A current-row scan cannot discover a stale tuple after its source
row has been deleted, so deletion transactions remain mandatory and failed
deletion markers stay retained until delivery is confirmed. Operators repair
the dependency and requeue one exact dead-letter marker with `outbox-repair`;
the service does not run a periodic reconciler.

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
   failures, derived-state verification mismatches, and revocation lag.

## 7. Acceptance Checks

Gateway enforcement is live in dev and prod. Do not set `heimdall.enabled=false`
to run these checks: that removes the HTTPRoute, so the API is no longer
reachable through the gateway.

Done: the Middleware, HTTPRoute and RuleSet serve traffic, the backend accepts
only Heimdall JWTs, the FGA and index relays and `outbox-repair` are deployed,
and the interim ingress is disabled.

Still to check against the live deployment:

1. The live OpenFGA model ID includes every Mentorship type and relation.
2. FGA coverage after the seed matches PostgreSQL-derived expectations.
3. Authenticated, anonymous, denied, cross-program, and stale-tuple smoke tests
   pass through the gateway.
4. The Nuxt server calls the gateway's cluster-internal address instead of the
   backend Service (§5).
