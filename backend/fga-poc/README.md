# Mentorship FGA — Authorization Model

Fine-grained authorization model for the LFX Mentorship service, built and
validated against the OpenFGA CLI before integration into the LFX Platform v2
production stack (see [Integration](#integration-into-lfx-platform-v2)).

Reviewed by @Michal Lehotsky — corrections from that review are folded into
the model below (mentors cannot decide applications; withdraw/re-apply
require an ownership check).

## Contents

| File | Purpose |
|---|---|
| `model.fga` | The OpenFGA authorization model (types, relations, permissions) |
| `tuples.yaml` | Sample relationship tuples for local testing |
| `tests.yaml` | Regression test suite, run via the OpenFGA CLI |

## Principle

**PostgreSQL is the system of record for all data — including memberships and
ownership. OpenFGA holds a derived authorization index: only the relations
Heimdall needs to answer "may this caller act on this object" at the edge.**

The service publishes tuples to [fga-sync](https://github.com/linuxfoundation/lfx-v2-fga-sync)
(`GenericFGAMessage` over NATS) at state transitions, and once as a bulk seed
after the DynamoDB → Postgres backfill.

**Delivery is via a transactional outbox, not a bare publish.** A Postgres
commit and a NATS publish cannot be made atomic — a crash between them silently
loses a grant or, worse, a revocation. Each state change therefore records a
dirty-object marker in an `fga_outbox` table in the same transaction; a relay
re-derives the full tuple payload from current Postgres state at send time and
marks the row sent. A periodic reconciliation job re-derives expected relations
from Postgres and repairs drift. This approach is preferred over storing the
outbox payload directly: the `GenericFGAMessage` envelope carries no object
version, so a stale payload retried after a newer revocation would restore
exactly the tuple that was revoked.

## Model overview

This PoC uses short type names (`project`, `program`, `application`, `task`).
Production types in `lfx-v2-helm/charts/lfx-platform/files/model.fga` will
use the `mentorship_` prefix (`mentorship_program`, `mentorship_application`,
`mentorship_task`) following the platform naming convention.

Four types, each scoped to its parent via a relation:

```
project:<slug>
  └── program:<id>
        └── application:<id>
              └── task:<id>
```

| Type | Key relations | Key permissions |
|---|---|---|
| `project` | `super_admin`, `program_approver`, `program_admin` | `can_create_program`, `can_approve_program` |
| `program` | `program_admin`, `mentor` (+ inherited from `project`) | `can_view`, `can_invite_mentor`, `can_create_term`, `can_add_prerequisite_task`, `can_add_task`, `can_view_applications`, `can_decide_application`, `can_approve` |
| `application` | `mentee` (+ inherited from `program`) | `can_view`, `can_decide`, `can_add_task`, `can_withdraw`, `can_reapply` |
| `task` | `assignee` (+ inherited from `application`) | `can_view`, `can_update_status` |

> **Note:** mentors may only add tasks while an application is in the
> `Accepted` state. FGA has no notion of application lifecycle state, so this
> check belongs in the service's business logic layered on top of `can_add_task`.

### Role summary

| Role | Scope | What they can do |
|---|---|---|
| `super_admin` | per-project | everything within that project |
| `program_approver` | per-project | view + approve programs in their project, view applications — deliberately excluded from `admin` so a program cannot approve itself |
| `program_admin` | per-project + per-program | create programs (project-level); full control of their specific program and its applications/tasks, including deciding applications |
| `mentor` | per-program | view program + applications, create terms, add prerequisite and non-prerequisite tasks, update task status — cannot invite mentors and **cannot** accept/reject applications |
| `mentee` | per-application | view, withdraw, and re-apply to their own application and tasks; withdraw/re-apply require ownership — the application ID alone is not a credential |

## Key modeling decisions

1. **No mentee→program relation.** "Accepted mentee" is a *state* of the
   application (Postgres), not an FGA relation. Every mentee-facing surface is
   covered: their applications and tasks carry their own tuples, program pages
   are public, and their dashboard is `/me`-scoped. If enrolled-only program
   content is ever added, the acceptance transition is exactly where a
   `participant` tuple would be emitted.

2. **Tasks parent to the application in Postgres; parent to the program in FGA.**
   In Postgres, tasks belong to the mentee's application journey (`prerequisite`
   tasks gate application consideration; `non_prerequisite` tasks gate
   graduation). In FGA, the task's parent points at the **program** because
   reviewers (mentors/admins) hold their relations there — pointing at the
   application would authorize nobody. One FGA type covers both categories.

3. **Workflow gates are business logic, not access rules.** "All prerequisite
   tasks submitted before the application is considered" and "all
   non-prerequisite tasks submitted to graduate" are state-machine checks in
   Postgres. FGA answers *who may touch*; the service answers *what is allowed
   given state*.

4. **Pending mentor invitations have no FGA presence.** A pending invitation is
   a Postgres row; the `mentor` tuple appears when it is accepted. Applications
   get their tuples on submission so mentors can evaluate them while still
   pending — "pending" describes application *status*, not an absence of tuples.

5. **`approver` is kept separate from `admin`.** A `program_admin` may not
   approve their own program. `program_approver` is granted at the project level
   to LF staff; it is not a superset of admin.

6. **Ownership-gated withdraw/re-apply.** `writer: mentee or admin` — the
   application ID alone is not a credential. Staff-assisted withdrawal
   (support/admins acting on a mentee's behalf) is a real requirement, hence
   `admin` is included in `writer`.

7. **No attribute-level access control — split the endpoints instead.**
   Heimdall authorizes a *route*, not a field. Mentors may evaluate an
   application but not change its status, so there is no single
   `PATCH /applications/{uid}` accepting an arbitrary field set:

   | Route | Relation checked | Who |
   |---|---|---|
   | `PATCH /applications/{uid}/status` | `admin` | admins only (accept / decline) |
   | `POST /applications/{uid}/withdraw` | `writer` | applicant or admin (staff-assisted) |
   | `PUT /applications/{uid}/evaluation` | `mentor` | mentors and admins — not the applicant |
   | `PATCH /tasks/{uid}/submission` | `assignee` | the mentee |
   | `PATCH /tasks/{uid}/review` | `admin` | mentors and admins |

## Lifecycle → FGA emissions

| Transition | FGA action (via fga-sync) |
|---|---|
| Program created (`draft`) | `update_access`: writers, mentors, approvers, `project` reference — not yet public |
| Submitted for approval | No tuple change |
| Approved (`→ published`) | `update_access` **with** `public` (`viewer@user:*`) — only transition that emits the wildcard |
| Rejected / archived / hidden | `update_access` **without** `public` — revokes the wildcard |
| Mentor or admin added | `member_put` mentor→program or writer→program |
| Application submitted | `update_access`: applicant + `mentorship_program` reference |
| Task created | `update_access`: assignee + `mentorship_program` reference |
| Mentee application accepted | — (none; see decision 1) |
| Mentor application accepted | `member_put` mentor→program (same grant as invite-accept) |
| Application withdrawn / declined | — (tuples stay; admins/mentors can still view the record) |
| Mentor / admin removed from program | `member_remove` **naming the relation** being removed — must not silently strip other roles the user holds on the same object |
| Program deleted | `delete_access` for the program **and** every application and task under it — otherwise child tuples are orphaned in FGA |
| Application / task deleted | `delete_access` for that object |
| Backfill (one-time) | bulk seed: `update_access` for every object, selecting **only currently effective** membership rows |

> **`member_remove` must name the relation.** An empty relations array deletes
> every direct relation the user holds on that object; a populated array deletes
> only the named ones. Because a user can be both `mentor` and `writer` on the
> same program, removing one role must not silently strip the other.

## Running the tests

Requires the [OpenFGA CLI](https://github.com/openfga/cli):

```bash
fga model test --tests tests.yaml
# Expected: Tests 15/15 passing, Checks 77/77 passing
```

## Trying it interactively

Spin up a local OpenFGA server with the Playground enabled:

```bash
docker run -d --name openfga \
  -p 8080:8080 -p 8081:8081 -p 3000:3000 \
  openfga/openfga run --playground-enabled --playground-addr 0.0.0.0:3000
```

Create a store and load the model and tuples:

```bash
FGA_STORE_ID=$(fga store create --name "mentorship-fga" --api-url http://localhost:8080 \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['store']['id'])")

fga model write --store-id="$FGA_STORE_ID" --api-url http://localhost:8080 --file model.fga
fga tuple import --store-id="$FGA_STORE_ID" --api-url http://localhost:8080 --file tuples.yaml

echo "Playground: http://localhost:3000/playground"
```

Teardown:

```bash
docker stop openfga && docker rm openfga
```

## Integration into LFX Platform v2

Production integration requires three PRs, independent of each other after PR 1:

| PR | Repo | Contents |
|---|---|---|
| 1 | `lfx-v2-helm` | Append `mentorship_*` types to `charts/lfx-platform/files/model.fga`; add test scenarios to the platform test suite |
| 2 | `lfx-v2-mentorship-service` | Add `fga_outbox` table + relay; publish `lfx.fga-sync.*` on lifecycle transitions; replace in-service permission checks with `lfx.access_check.request` |
| 3 | Heimdall config | Add RuleSets — one rule per route using the decision-7 table above |

PRs 2 and 3 depend on PR 1 being merged (a RuleSet referencing a relation that
does not exist fails closed).

**`tests.yaml` is the merge gate for PR 1.** Two categories are critical:
- **Negative cases**: mentor denied `admin` on an application; `program_admin`
  denied `program_approver` on their own program; `writer` on project A denied
  on project B's program.
- **Inheritance cases**: a project `writer` reaches a task two levels down;
  revoking a direct program `writer` does *not* revoke access for someone who
  still holds it via the project.

After deploying to staging, replay a representative set of NATS events and
verify the results against the assertions in `tests.yaml` via
`lfx.access_check.request`.

## Open architecture questions

| # | Question | Proposed default |
|---|---|---|
| AQ-1 | **One canonical route per object.** Confirm: `/me/*` exists only as collections; following a collection result to `GET /applications/{uid}` hits the same Heimdall-checked route an admin uses. | Route 1 (endorsed on the Architecture call) |
| AQ-2 | **Per-object child tuples vs. program-level only.** Keep the parent tuple and resolve `manager` through it, or stamp admins/mentors directly onto each child for read efficiency? | Keep inheritance; direct grants trade write amplification for read speed — at Mentorship volumes the parent hop is not a bottleneck |
| AQ-3 | **Storage deviation.** PostgreSQL instead of NATS-KV. | Keep Postgres per `02-target-architecture.md` |
| AQ-4 | **`mentorship_program_creator` / `mentorship_coordinator` on `project`.** These mirror `meetings_creator`/`meeting_coordinator`. The project-service must own and emit `mentorship_coordinator` (as it does for `meeting_coordinator`) — or the relation moves to `mentorship_program`. | Add both; confirm ownership before implementation |
| AQ-5 | **Drop HMAC email-approval links** in favour of LF staff approving via a logged-in Self Serve page gated on `program_approver`? | Drop them — one authorization model, no cross-team prerequisite |
| AQ-6 | **Program creation policy.** Gate on `mentorship_program_creator`, or keep legacy (any authenticated user creates; approval publishes)? | Launch with parity: authenticated-only creation, approval gated on `program_approver` |
| AQ-7 | **Invite-accept authorization.** Pending invitations have no FGA tuple, so Heimdall has nothing to check at accept time. Options: (a) emit `mentorship_invite` with `invitee: [user]` (mirrors `committee_invite`); (b) treat the signed invite token as the credential. | (a) — keeps the no-exceptions rule and has platform precedent |
