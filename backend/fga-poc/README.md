# Mentorship FGA — Authorization Model

Fine-grained authorization model for the LFX Mentorship service, built and
validated against the OpenFGA CLI before being integrated into the LFX
Platform v2 production stack (see [Integration](#integration-into-lfx-platform-v2)).

Reviewed by @Michal Lehotsky — corrections from that review are folded into
the model below (mentors cannot decide applications; withdraw/re-apply
require an ownership check).

## Contents

| File | Purpose |
|---|---|
| `model.fga` | The OpenFGA authorization model (types, relations, permissions) |
| `tuples.yaml` | Sample relationship tuples for local testing |
| `tests.yaml` | Regression test suite, run via the OpenFGA CLI |

## Model overview

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
> `Accepted` state. FGA has no concept of application lifecycle state, so
> this check belongs in the mentorship service's business logic, layered on
> top of `can_add_task` — it is intentionally not modeled as an FGA relation.

### Role summary

| Role | Scope | What they can do |
|---|---|---|
| `super_admin` | per-project | everything within that project |
| `program_approver` | per-project | view + approve programs in their project, view applications — deliberately excluded from `admin` so a program cannot approve itself |
| `program_admin` | per-project + per-program | create programs (project-level); full control of their specific program and its applications/tasks, including deciding applications |
| `mentor` | per-program | view program + applications, create terms, add prerequisite and non-prerequisite tasks, update task status — cannot invite mentors and **cannot** accept/reject applications |
| `mentee` | per-application | view, withdraw, and re-apply to their own application and tasks; withdraw/re-apply require ownership — the application ID alone is not a credential |

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

The production path requires three PRs across three repos:

1. **`linuxfoundation/lfx-v2-helm`** — append the `project`, `program`,
   `application`, and `task` types to the shared OpenFGA model
   (`charts/lfx-platform/files/model.fga`), and bump the model version.
2. **`linuxfoundation/lfx-v2-fga-sync`** — register the mentorship service in
   `docs/fga-protected-types.md`, listing the object types and the
   `update_access` / `delete_access` / `member_put` / `member_remove`
   operations each supports.
3. **`linuxfoundation/lfx-v2-mentorship-service`** —
   - add `docs/fga-contract.md` documenting lifecycle events → NATS subjects → tuple shapes,
   - publish to `lfx.fga-sync.*` on program, application, and task lifecycle events,
   - replace in-service permission checks with `lfx.access_check.request` calls
     against the relations in this model (`can_invite_mentor`, `can_approve`,
     `can_decide`, `can_update_status`, `can_withdraw`, `can_reapply`).

After deploying to staging, replay a representative set of NATS events and
verify the results against the assertions in `tests.yaml` using
`lfx.access_check.request`.
